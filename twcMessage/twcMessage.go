package twcMessage

import (
	"encoding/hex"
	"fmt"
	"github.com/goburrow/serial"
	"log"
	"time"
)

// Message structure
//
// Byte	Function
//  00	C0
//  01	C?
//  02	E?
//  03	Source Address
//  04	Source Address
//  05	Destination Address
//  06	Destination Address
//  07	Status
//  08	Allowed/Requested Current
//  09	Allowed/Requested Current
//  10	Charging Current
//  11	Charging Current
//  12	00
//  13	00
//  14	Checksum
//  15	C0
//  16	Fe

type TwcMessage struct {
	bytes       []byte
	currentByte int
	isEscaped   bool
	port        serial.Port
	listenMode  bool
	isC0        bool
}

func New(p serial.Port, listenMode bool) TwcMessage {
	m := TwcMessage{make([]byte, 17), 0, false, p, listenMode, false}
	return m
}

// Add a new byte to the message
// Add a new byte to the message
func (m *TwcMessage) AddByte(b byte) {
	// First byte is always 0xc0
	if m.currentByte == 0 {
		if b == 0xc0 {
			m.bytes[0] = b
			m.currentByte = 1
		}
		return
	}
	// If the last byte was an escape character process this byte accordingly
	if m.isEscaped {
		if m.bytes[m.currentByte-1] == 0xdb {
			if b == 0xdc {
				m.bytes[m.currentByte-1] = 0xc0
			} else if b == 0xdd {
				m.bytes[m.currentByte-1] = 0xdb
			}
		}
		m.isEscaped = false
	} else {
		if m.currentByte >= len(m.bytes) {
			log.Panicf("Tried to add a byte at position %d - Length = %d - Capacity = %d\n", m.currentByte, len(m.bytes), cap(m.bytes))
			return
		}
		m.bytes[m.currentByte] = b
		m.currentByte++
		if (m.currentByte >= len(m.bytes)) && !m.IsComplete() {
			log.Print("Buffer Overflow!")
			m.currentByte = 0
			m.isEscaped = false
			for i := 0; i < len(m.bytes); i++ {
				m.bytes[i] = 0
			}
		} else {
			m.isEscaped = b == 0xdb
			if m.isC0 && (b != 0xfe) {
				m.Reset()
				m.bytes[0] = 0xc0
				m.bytes[1] = b
				m.currentByte = 2
			}
			m.isC0 = b == 0xc0
		}
	}
}

func (m *TwcMessage) IsComplete() bool {
	return (m.bytes[16] == 0xfe) && (m.currentByte == cap(m.bytes))
}

func (m *TwcMessage) IsValid() (valid bool) {
	chksum := m.bytes[2]
	for _, b := range m.bytes[3:13] {
		chksum += b
	}
	return (chksum & 0xff) == m.bytes[14]
}

func (m *TwcMessage) Reset() {
	m.isEscaped = false
	m.isC0 = false
	m.currentByte = 0
	for i := 0; i < cap(m.bytes); i++ {
		m.bytes[i] = 0
	}
}

func (m *TwcMessage) Print() {
	fmt.Println(hex.EncodeToString(m.bytes))
}

func (m *TwcMessage) GetCode() int {
	return (int(m.bytes[1]) << 8) + int(m.bytes[2])
}

func (m *TwcMessage) PutCode(c int) {
	m.bytes[1] = byte((c >> 8) & 0xff)
	m.bytes[2] = byte(c & 0xff)
}

func (m *TwcMessage) GetFromAddress() uint {
	return (uint(m.bytes[3]) << 8) + uint(m.bytes[4])
}

func (m *TwcMessage) PutFromAddress(a uint) {
	m.bytes[3] = byte((a >> 8) & 0xff)
	m.bytes[4] = byte(a & 0xff)
}

func (m *TwcMessage) GetToAddress() uint {
	return (uint(m.bytes[5]) << 8) + uint(m.bytes[6])
}

func (m *TwcMessage) PutToAddress(a uint) {
	m.bytes[5] = byte((a >> 8) & 0xff)
	m.bytes[6] = byte(a & 0xff)
}

func (m *TwcMessage) GetStatus() byte {
	return m.bytes[7] & 0xff
}

func (m *TwcMessage) PutStatus(s byte) {
	m.bytes[7] = s & 0xff
}

func (m *TwcMessage) GetSetPoint() int {
	return (int(m.bytes[8]) << 8) + int(m.bytes[9])
}

func (m *TwcMessage) PutSetPoint(i int) {
	m.bytes[8] = byte((i >> 8) & 0xff)
	m.bytes[9] = byte(i & 0xff)
}

func (m *TwcMessage) GetCurrent() int {
	return (int(m.bytes[10]) << 8) + int(m.bytes[11])
}

func (m *TwcMessage) PutCurrent(i int) {
	m.bytes[10] = byte((i >> 8) & 0xff)
	m.bytes[11] = byte(i & 0xff)
}

func (m *TwcMessage) writeByte(b byte) {
	if !m.listenMode {
		bytes := make([]byte, 0)
		bytes = append(bytes, b)
		_, _ = (m.port).Write(bytes)
	} else {
		fmt.Print("[%02x] ", b)
	}
}

func (m *TwcMessage) SendMessage() {
	//Calculate a new checksum
	chksum := m.bytes[2]
	for _, b := range m.bytes[3:13] {
		chksum += b
	}
	m.bytes[14] = chksum & 0xff

	if m.listenMode {
		fmt.Print("Sending : ")
	}
	for idx, b := range m.bytes {
		//Escape 0xc0 and 0xdb codes if in the data area of the message (bytes 1..14)
		if idx == 0 || idx > 14 {
			m.writeByte(b)
		} else {
			switch b {
			case 0xc0:
				m.writeByte(0xdb)
				m.writeByte(0xdc)
			case 0xdb:
				m.writeByte(0xdb)
				m.writeByte(0xdd)
			default:
				m.writeByte(b)
			}
		}
	}
	if m.listenMode {
		fmt.Println("...Sent")
	}
	time.Sleep(100 * time.Millisecond)
}

func (m *TwcMessage) SendMasterLinkReady1(fromAddress uint) {
	copy(m.bytes, []byte{0xc0, 0xfc, 0xe1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfc, 0xfe})
	m.PutFromAddress(fromAddress)
	m.PutToAddress(fromAddress)
	if m.listenMode {
		fmt.Print("LinkReady-1 ")
	}
	m.SendMessage()
}

func (m *TwcMessage) SendMasterLinkReady2(fromAddress uint) {
	copy(m.bytes, []byte{0xc0, 0xfb, 0xe2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfc, 0xfe})
	m.PutFromAddress(fromAddress)
	m.PutToAddress(fromAddress)
	if m.listenMode {
		fmt.Print("LinkReady-1 ")
	}
	m.SendMessage()
}

func (m *TwcMessage) SendMasterHeartbeat(fromAddress uint, toAddress uint, status byte, current int, setPoint int) {
	copy(m.bytes, []byte{0xc0, 0xfb, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0, 0xfe})
	m.PutFromAddress(fromAddress)
	m.PutToAddress(toAddress)
	m.PutStatus(status)
	m.PutCurrent(current)
	m.PutSetPoint(setPoint)
	if m.listenMode {
		fmt.Print("Heartbeat ")
	}
	m.SendMessage()
}
