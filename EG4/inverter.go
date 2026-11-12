// Package eg4xp reads EG4 12000XP / 6000XP off-grid inverters over Modbus TCP,
// typically through an RS485-to-Ethernet gateway (e.g. Waveshare in
// "Modbus TCP to RTU" mode). Register layout follows the LuxPower protocol as
// mapped for the XP series; see the "EG4 12000XP Modbus Register Map" doc.
package EG4

import (
	"context"
	"fmt"
	"time"
)

// inputBlocks are the FC04 reads performed on every poll. Each stays inside one
// 40-register bank. Together they cover input registers 0-199.
var inputBlocks = []uint16{0, 40, 80, 120, 160}

const inputSpan = 200

// Config describes one inverter.
type Config struct {
	Name    string        // label used in results, e.g. "XP-1"
	Address string        // gateway host:port, e.g. "192.168.10.201:502"
	UnitID  byte          // Modbus slave address (holding register 15), default 1
	Timeout time.Duration // per-request timeout; keep it above the gateway's own timeout. Default 3s
}

// Inverter is a handle to one 12000XP.
type Inverter struct {
	cfg  Config
	conn *conn
}

// NewInverter creates a handle that owns its own TCP connection.
// Use NewSystem when several inverters share one gateway channel.
func NewInverter(cfg Config) *Inverter {
	cfg = withDefaults(cfg)
	return &Inverter{cfg: cfg, conn: newConn(cfg.Address, cfg.Timeout)}
}

func withDefaults(cfg Config) Config {
	if cfg.UnitID == 0 {
		cfg.UnitID = 1
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("%s#%d", cfg.Address, cfg.UnitID)
	}
	return cfg
}

// Name returns the configured label.
func (inv *Inverter) Name() string { return inv.cfg.Name }

// Close drops the connection.
func (inv *Inverter) Close() { inv.conn.Close() }

// ReadInputs reads qty input registers (FC04) starting at start.
func (inv *Inverter) ReadInputs(ctx context.Context, start, qty uint16) ([]uint16, error) {
	return inv.conn.readRegisters(ctx, inv.cfg.UnitID, fcReadInput, start, qty)
}

// ReadHolding reads qty holding registers (FC03) starting at start.
func (inv *Inverter) ReadHolding(ctx context.Context, start, qty uint16) ([]uint16, error) {
	return inv.conn.readRegisters(ctx, inv.cfg.UnitID, fcReadHolding, start, qty)
}

// Read polls input registers 0-199 and decodes them.
func (inv *Inverter) Read(ctx context.Context) (*Reading, error) {
	var raw [inputSpan]uint16
	for _, start := range inputBlocks {
		vals, err := inv.ReadInputs(ctx, start, maxRegsPerRead)
		if err != nil {
			return nil, fmt.Errorf("%s: input %d-%d: %w", inv.cfg.Name, start, start+maxRegsPerRead-1, err)
		}
		copy(raw[start:], vals)
	}
	r := Decode(raw)
	r.Name = inv.cfg.Name
	r.Time = time.Now()
	return r, nil
}

// Reading is one decoded snapshot of an inverter, in engineering units.
type Reading struct {
	Name string
	Time time.Time

	Status     uint16 // input 0
	StatusText string

	// PV (2 MPPTs)
	PV1Voltage, PV2Voltage float64 // V
	PV1Power, PV2Power     int     // W
	PV1Current, PV2Current float64 // A
	PVPower                int     // W, PV1+PV2

	// Battery
	BatteryVoltage    float64 // V
	SOC, SOH          int     // %
	ChargePower       int     // W into battery
	DischargePower    int     // W out of battery
	BatteryPower      int     // W, + charging / - discharging
	BatteryCurrent    float64 // A, this inverter's share: BatteryPower / BatteryVoltage
	BatteryCurrentBMS float64 // A, + charging / - discharging (BMS-reported, whole bank)

	// Grid / AC input
	GridVoltage     float64 // V, L1-L2
	GridFrequency   float64 // Hz
	GridImportPower int     // W (input 27)
	ACChargePower   int     // W, rectifier (input 17)
	ACInputIsGen    bool    // input 77 bit 0

	// Inverter / backup output
	InverterPower    int     // W
	EPSVoltage       float64 // V, L1-L2
	EPSFrequency     float64 // Hz
	EPSPower         int     // W
	EPSApparentPower int     // VA
	EPSL1Voltage     float64 // V
	EPSL2Voltage     float64 // V
	EPSL1Power       int     // W (includes smart-load output)
	EPSL2Power       int     // W (includes smart-load output)
	LoadPower        int     // W (input 170)
	ACCouplePower    int     // W (input 153)

	// Generator. The 12000XP takes the generator on its AC input, so when
	// ACInputIsGen is set the power and energy come from the AC input
	// registers (27, 37, 58-59); otherwise they are 0. Inputs 123-126, the
	// LuxPower generator power/energy registers, hold unrelated values on the
	// 12000XP and are not used. Voltage and frequency use the generator
	// registers (121, 122) and fall back to the AC input's when those read 0.
	GenVoltage   float64 // V
	GenFrequency float64 // Hz
	GenL1Voltage float64 // V (input 195)
	GenL2Voltage float64 // V (input 196)
	GenPower     int     // W

	// Energy today, kWh
	PVEnergyToday         float64
	ACChargeEnergyToday   float64
	ChargeEnergyToday     float64
	DischargeEnergyToday  float64
	EPSEnergyToday        float64
	GridImportEnergyToday float64
	LoadEnergyToday       float64
	GenEnergyToday        float64 // AC input energy today when ACInputIsGen

	// Lifetime energy, kWh
	PVEnergyTotal         float64
	ChargeEnergyTotal     float64
	DischargeEnergyTotal  float64
	EPSEnergyTotal        float64
	GridImportEnergyTotal float64
	LoadEnergyTotal       float64
	GenEnergyTotal        float64 // AC input lifetime energy when ACInputIsGen

	// Health
	FaultCode      uint32
	WarningCode    uint32
	InternalTemp   int           // °C; some 12000XPs always report 0
	Radiator1Temp  int           // °C
	Radiator2Temp  int           // °C
	RunningTime    time.Duration `json:"-"`
	RunningSeconds int64         // RunningTime in seconds, for JSON

	// BMS
	BMSChargeCurrentLimit    float64 // A
	BMSDischargeCurrentLimit float64 // A
	BMSChargeVoltage         float64 // V
	BMSDischargeCutoff       float64 // V
	BMSMaxCellVoltage        float64 // V
	BMSMinCellVoltage        float64 // V
	BMSMaxCellTemp           float64 // °C
	BMSMinCellTemp           float64 // °C
	BMSCycleCount            int
	BatteryParallelCount     int
	BatteryCapacityAh        int
	BMSFaultCode             uint16
	BMSWarningCode           uint16

	// Parallel system (input 113)
	ParallelRole  int // 0 = standalone, 1 = master, 2 = slave (LuxPower convention)
	ParallelPhase int
	ParallelCount int

	// Raw input registers 0-199, for anything not decoded above.
	Raw [inputSpan]uint16 `json:"-"`
}

// HasFault reports whether the inverter or BMS reports a fault code.
func (r *Reading) HasFault() bool { return r.FaultCode != 0 || r.BMSFaultCode != 0 }

// Decode converts raw input registers 0-199 into a Reading. It is exported so
// readings captured elsewhere (logs, tests) can be decoded the same way.
func Decode(raw [inputSpan]uint16) *Reading {
	u := func(a int) int { return int(raw[a]) }
	s := func(a int) int { return int(int16(raw[a])) }
	d := func(a int, div float64) float64 { return float64(raw[a]) / div }
	ds := func(a int, div float64) float64 { return float64(int16(raw[a])) / div }
	u32 := func(a int) uint32 { return uint32(raw[a]) | uint32(raw[a+1])<<16 } // low word first
	e32 := func(a int) float64 { return float64(u32(a)) / 10 }

	r := &Reading{Raw: raw}
	r.Status = raw[0]
	r.StatusText = StatusText(raw[0])

	r.PV1Voltage, r.PV2Voltage = d(1, 10), d(2, 10)
	r.PV1Power, r.PV2Power = u(7), u(8)
	r.PV1Current, r.PV2Current = d(72, 100), d(73, 100)
	r.PVPower = r.PV1Power + r.PV2Power

	r.BatteryVoltage = d(4, 10)
	r.SOC = int(raw[5] & 0xFF)
	r.SOH = int(raw[5] >> 8)
	r.ChargePower, r.DischargePower = u(10), u(11)
	r.BatteryPower = r.ChargePower - r.DischargePower
	if r.BatteryVoltage > 0 {
		// No register gives the inverter's own battery current, so derive it.
		r.BatteryCurrent = float64(r.BatteryPower) / r.BatteryVoltage
	}
	r.BatteryCurrentBMS = ds(98, 10)

	r.GridVoltage = d(12, 10)
	r.GridFrequency = d(15, 100)
	r.GridImportPower = u(27)
	r.ACChargePower = u(17)
	r.ACInputIsGen = raw[77]&0x01 != 0

	r.InverterPower = u(16)
	r.EPSVoltage = d(20, 10)
	r.EPSFrequency = d(23, 100)
	r.EPSPower = u(24)
	r.EPSApparentPower = u(25)
	r.EPSL1Voltage, r.EPSL2Voltage = d(127, 10), d(128, 10)
	r.EPSL1Power, r.EPSL2Power = u(129), u(130)
	r.LoadPower = s(170)
	r.ACCouplePower = u(153)

	r.GenVoltage = d(121, 10)
	r.GenFrequency = d(122, 100)
	r.GenL1Voltage, r.GenL2Voltage = d(195, 10), d(196, 10)

	r.PVEnergyToday = d(28, 10) + d(29, 10)
	r.ACChargeEnergyToday = d(32, 10)
	r.ChargeEnergyToday = d(33, 10)
	r.DischargeEnergyToday = d(34, 10)
	r.EPSEnergyToday = d(35, 10)
	r.GridImportEnergyToday = d(37, 10)
	r.LoadEnergyToday = d(171, 10)

	r.PVEnergyTotal = e32(40) + e32(42)
	r.ChargeEnergyTotal = e32(50)
	r.DischargeEnergyTotal = e32(52)
	r.EPSEnergyTotal = e32(54)
	r.GridImportEnergyTotal = e32(58)
	r.LoadEnergyTotal = e32(172)

	// See the Generator fields in Reading for why these come from the AC input.
	if r.ACInputIsGen {
		r.GenPower = r.GridImportPower
		r.GenEnergyToday = r.GridImportEnergyToday
		r.GenEnergyTotal = r.GridImportEnergyTotal
		if r.GenVoltage == 0 {
			r.GenVoltage = r.GridVoltage
		}
		if r.GenFrequency == 0 {
			r.GenFrequency = r.GridFrequency
		}
	}

	r.FaultCode = u32(60)
	r.WarningCode = u32(62)
	r.InternalTemp = s(64)
	r.Radiator1Temp, r.Radiator2Temp = s(65), s(66)
	r.RunningTime = time.Duration(u32(69)) * time.Second
	r.RunningSeconds = int64(u32(69))

	r.BMSChargeCurrentLimit = d(81, 10)
	r.BMSDischargeCurrentLimit = d(82, 10)
	r.BMSChargeVoltage = d(83, 10)
	r.BMSDischargeCutoff = d(84, 10)
	r.BatteryParallelCount = u(96)
	r.BatteryCapacityAh = u(97)
	r.BMSFaultCode = raw[99]
	r.BMSWarningCode = raw[100]
	r.BMSMaxCellVoltage = d(101, 1000)
	r.BMSMinCellVoltage = d(102, 1000)
	r.BMSMaxCellTemp = ds(103, 10)
	r.BMSMinCellTemp = ds(104, 10)
	r.BMSCycleCount = u(106)

	r.ParallelRole = int(raw[113] & 0x03)
	r.ParallelPhase = int(raw[113]>>2) & 0x03
	r.ParallelCount = int(raw[113] >> 8)
	return r
}

var statusNames = map[uint16]string{
	0x00: "Standby",
	0x01: "Fault",
	0x02: "Firmware updating",
	0x04: "PV supplying load",
	0x08: "PV charging battery",
	0x10: "Battery supplying load",
	0x11: "Bypass",
	0x14: "PV + battery supplying load",
	0x19: "PV charging + bypass",
	0x20: "AC charging battery",
	0x28: "PV + AC charging battery",
	0x40: "Battery off-grid",
	0x80: "PV off-grid",
	0x88: "PV off-grid, charging battery",
	0xC0: "PV + battery off-grid",
}

// StatusText names an input-register-0 status code.
func StatusText(code uint16) string {
	if s, ok := statusNames[code]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (0x%02X)", code)
}
