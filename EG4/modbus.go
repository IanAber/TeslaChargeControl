package EG4

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Modbus function codes used by the 12000XP.
const (
	fcReadHolding = 0x03
	fcReadInput   = 0x04
)

// maxRegsPerRead is the LuxPower limit: reads must stay inside one 40-register bank.
const maxRegsPerRead = 40

// ModbusError is an exception response returned by the inverter (via the gateway).
type ModbusError struct {
	Function byte
	Code     byte
}

func (e *ModbusError) Error() string {
	names := map[byte]string{
		1: "illegal function", 2: "illegal data address", 3: "illegal data value",
		4: "server device failure", 0x0A: "gateway path unavailable",
		0x0B: "gateway target device failed to respond",
	}
	name := names[e.Code]
	if name == "" {
		name = "unknown"
	}
	return fmt.Sprintf("modbus exception 0x%02X (%s) on function 0x%02X", e.Code, name, e.Function)
}

// conn is a Modbus TCP connection to one gateway endpoint (e.g. one Waveshare channel).
// Requests are serialized: RS485 is half-duplex, so only one request may be in flight.
type conn struct {
	addr    string
	timeout time.Duration

	mu    sync.Mutex
	c     net.Conn
	txnID uint16
}

func newConn(addr string, timeout time.Duration) *conn {
	return &conn{addr: addr, timeout: timeout}
}

func (m *conn) close() {
	if m.c != nil {
		_ = m.c.Close()
		m.c = nil
	}
}

// Close drops the TCP connection; the next request reconnects.
func (m *conn) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.close()
}

// readRegisters performs one FC03/FC04 read and returns qty 16-bit values.
func (m *conn) readRegisters(ctx context.Context, unit, fc byte, start, qty uint16) ([]uint16, error) {
	if qty == 0 || qty > maxRegsPerRead {
		return nil, fmt.Errorf("quantity %d out of range 1-%d", qty, maxRegsPerRead)
	}
	if start/maxRegsPerRead != (start+qty-1)/maxRegsPerRead {
		return nil, fmt.Errorf("read %d+%d crosses a 40-register bank boundary", start, qty)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.c == nil {
		d := net.Dialer{Timeout: m.timeout}
		c, err := d.DialContext(ctx, "tcp", m.addr)
		if err != nil {
			return nil, fmt.Errorf("connect %s: %w", m.addr, err)
		}
		m.c = c
	}

	deadline := time.Now().Add(m.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = m.c.SetDeadline(deadline)

	m.txnID++
	txn := m.txnID
	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:], txn)
	binary.BigEndian.PutUint16(req[2:], 0) // protocol ID
	binary.BigEndian.PutUint16(req[4:], 6) // bytes that follow
	req[6] = unit
	req[7] = fc
	binary.BigEndian.PutUint16(req[8:], start)
	binary.BigEndian.PutUint16(req[10:], qty)

	if _, err := m.c.Write(req); err != nil {
		m.close()
		return nil, fmt.Errorf("write: %w", err)
	}

	for {
		vals, gotTxn, err := m.readResponse(unit, fc, qty)
		if err != nil {
			var me *ModbusError
			if !errors.As(err, &me) {
				m.close() // framing/timeout errors leave the stream in an unknown state
			}
			return nil, err
		}
		if gotTxn == txn {
			return vals, nil
		}
		// A late reply to an earlier, timed-out request: discard it and keep reading.
	}
}

func (m *conn) readResponse(unit, fc byte, qty uint16) ([]uint16, uint16, error) {
	hdr := make([]byte, 7)
	if _, err := io.ReadFull(m.c, hdr); err != nil {
		return nil, 0, fmt.Errorf("read header: %w", err)
	}
	txn := binary.BigEndian.Uint16(hdr[0:])
	length := binary.BigEndian.Uint16(hdr[4:])
	if length < 2 || length > 256 {
		return nil, txn, fmt.Errorf("bad MBAP length %d", length)
	}
	pdu := make([]byte, length-1)
	if _, err := io.ReadFull(m.c, pdu); err != nil {
		return nil, txn, fmt.Errorf("read body: %w", err)
	}
	if hdr[6] != unit {
		return nil, txn, fmt.Errorf("reply from unit %d, expected %d", hdr[6], unit)
	}
	if pdu[0] == fc|0x80 {
		if len(pdu) < 2 {
			return nil, txn, errors.New("short exception response")
		}
		return nil, txn, &ModbusError{Function: fc, Code: pdu[1]}
	}
	if pdu[0] != fc {
		return nil, txn, fmt.Errorf("reply function 0x%02X, expected 0x%02X", pdu[0], fc)
	}
	if len(pdu) < 2 || int(pdu[1]) != int(qty)*2 || len(pdu) != 2+int(qty)*2 {
		return nil, txn, fmt.Errorf("reply has %d data bytes, expected %d", len(pdu)-2, qty*2)
	}
	vals := make([]uint16, qty)
	for i := range vals {
		vals[i] = binary.BigEndian.Uint16(pdu[2+i*2:])
	}
	return vals, txn, nil
}
