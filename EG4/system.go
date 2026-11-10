package EG4

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// System is a group of paralleled inverters polled together.
//
// Each inverter can sit on its own gateway channel (different Address) or
// share one RS485 bus (same Address, different UnitID). Inverters that share
// an Address share one TCP connection and their requests are serialized;
// inverters on different addresses are read concurrently.
type System struct {
	Inverters []*Inverter
	conns     []*conn
}

// NewSystem builds a System. It rejects two inverters with the same address and
// unit ID, because both would answer the same requests on a shared bus.
func NewSystem(cfgs ...Config) (*System, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("no inverters configured")
	}
	byAddr := map[string]*conn{}
	seen := map[string]string{}
	sys := &System{}
	for _, cfg := range cfgs {
		cfg = withDefaults(cfg)
		key := fmt.Sprintf("%s#%d", cfg.Address, cfg.UnitID)
		if other, dup := seen[key]; dup {
			return nil, fmt.Errorf("%s and %s both use %s unit %d; give each a unique Modbus address (holding register 15)",
				other, cfg.Name, cfg.Address, cfg.UnitID)
		}
		seen[key] = cfg.Name
		c, ok := byAddr[cfg.Address]
		if !ok {
			c = newConn(cfg.Address, cfg.Timeout)
			byAddr[cfg.Address] = c
			sys.conns = append(sys.conns, c)
		}
		sys.Inverters = append(sys.Inverters, &Inverter{cfg: cfg, conn: c})
	}
	return sys, nil
}

// Close drops all connections.
func (s *System) Close() {
	for _, c := range s.conns {
		c.Close()
	}
}

// SystemReading holds each inverter's reading plus combined totals.
// Units that failed to read are listed in Errors and left out of the totals.
type SystemReading struct {
	Time     time.Time
	Units    []*Reading       // successful readings, in configured order
	Errors   map[string]error // inverter name -> error
	Complete bool             // true when every inverter was read

	// Summed across units
	PVPower         int // W
	LoadPower       int // W
	EPSPower        int // W
	InverterPower   int // W
	ChargePower     int // W
	DischargePower  int // W
	BatteryPower    int // W, + charging / - discharging
	GridImportPower int // W
	ACChargePower   int // W
	ACCouplePower   int // W
	GenPower        int // W

	PVEnergyToday         float64 // kWh
	LoadEnergyToday       float64 // kWh
	ChargeEnergyToday     float64 // kWh
	DischargeEnergyToday  float64 // kWh
	GridImportEnergyToday float64 // kWh
	ACChargeEnergyToday   float64 // kWh
	EPSEnergyToday        float64 // kWh
	GenEnergyToday        float64 // kWh

	// Shared battery bank: averaged across units
	BatteryVoltage float64 // V
	SOC            float64 // %

	AnyFault bool
}

// Read polls every inverter and returns per-unit readings and totals.
// It returns an error only when no inverter could be read.
func (s *System) Read(ctx context.Context) (*SystemReading, error) {
	results := make([]*Reading, len(s.Inverters))
	errs := make([]error, len(s.Inverters))

	var wg sync.WaitGroup
	for i, inv := range s.Inverters {
		wg.Add(1)
		go func(i int, inv *Inverter) {
			defer wg.Done()
			results[i], errs[i] = inv.Read(ctx)
		}(i, inv)
	}
	wg.Wait()

	sr := &SystemReading{Time: time.Now(), Errors: map[string]error{}}
	for i, r := range results {
		if errs[i] != nil {
			sr.Errors[s.Inverters[i].Name()] = errs[i]
			continue
		}
		sr.Units = append(sr.Units, r)
	}
	sr.Complete = len(sr.Errors) == 0
	if len(sr.Units) == 0 {
		return sr, fmt.Errorf("no inverter responded: %v", sr.Errors)
	}

	for _, r := range sr.Units {
		sr.PVPower += r.PVPower
		sr.LoadPower += r.LoadPower
		sr.EPSPower += r.EPSPower
		sr.InverterPower += r.InverterPower
		sr.ChargePower += r.ChargePower
		sr.DischargePower += r.DischargePower
		sr.GridImportPower += r.GridImportPower
		sr.ACChargePower += r.ACChargePower
		sr.ACCouplePower += r.ACCouplePower
		sr.GenPower += r.GenPower
		sr.PVEnergyToday += r.PVEnergyToday
		sr.LoadEnergyToday += r.LoadEnergyToday
		sr.ChargeEnergyToday += r.ChargeEnergyToday
		sr.DischargeEnergyToday += r.DischargeEnergyToday
		sr.GridImportEnergyToday += r.GridImportEnergyToday
		sr.ACChargeEnergyToday += r.ACChargeEnergyToday
		sr.EPSEnergyToday += r.EPSEnergyToday
		sr.GenEnergyToday += r.GenEnergyToday
		sr.BatteryVoltage += r.BatteryVoltage
		sr.SOC += float64(r.SOC)
		sr.AnyFault = sr.AnyFault || r.HasFault()
	}
	n := float64(len(sr.Units))
	sr.BatteryVoltage /= n
	sr.SOC /= n
	sr.BatteryPower = sr.ChargePower - sr.DischargePower
	return sr, nil
}

// summary drops the raw register arrays so each poll prints on one readable line.
func (sr *SystemReading) Summary() map[string]any {
	units := map[string]any{}
	for _, r := range sr.Units {
		units[r.Name] = map[string]any{
			"status":   r.StatusText,
			"pv_w":     r.PVPower,
			"load_w":   r.LoadPower,
			"batt_w":   r.BatteryPower,
			"batt_v":   r.BatteryVoltage,
			"soc":      r.SOC,
			"grid_w":   r.GridImportPower,
			"fault":    r.FaultCode,
			"warning":  r.WarningCode,
			"parallel": map[string]int{"role": r.ParallelRole, "count": r.ParallelCount},
		}
	}
	return map[string]any{
		"time":           sr.Time.Format(time.RFC3339),
		"complete":       sr.Complete,
		"pv_w":           sr.PVPower,
		"load_w":         sr.LoadPower,
		"batt_w":         sr.BatteryPower,
		"batt_v":         sr.BatteryVoltage,
		"soc":            sr.SOC,
		"grid_import_w":  sr.GridImportPower,
		"pv_kwh_today":   sr.PVEnergyToday,
		"load_kwh_today": sr.LoadEnergyToday,
		"any_fault":      sr.AnyFault,
		"units":          units,
	}
}
