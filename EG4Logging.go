package main

import (
	"database/sql"
	"fmt"
	"strings"

	"SystemController/EG4"
)

// col pairs a database column with the value written to it, so the column
// list, placeholders and values of an INSERT cannot get out of step.
type col struct {
	name  string
	value any
}

// insert builds and runs "INSERT INTO table (cols...) VALUES (?,...)".
func insert(tx *sql.Tx, table string, cols []col) (sql.Result, error) {
	names := make([]string, len(cols))
	values := make([]any, len(cols))
	for i, c := range cols {
		names[i] = c.name
		values[i] = c.value
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table,
		strings.Join(names, ", "), strings.TrimSuffix(strings.Repeat("?,", len(cols)), ","))
	return tx.Exec(q, values...)
}

// logEG4 writes one poll of the EG4 system to the database: a row in eg4_system
// for the combined values and the shared battery/BMS, and a row in eg4_inverter
// for each inverter that answered. Both are written in one transaction so a
// poll is never half logged. The tables are defined in EG4/eg4_tables.sql.
func logEG4(db *sql.DB, sr *EG4.SystemReading) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	// The BMS may not pass SOH to the inverters; log NULL rather than 0.
	var soh sql.NullInt64
	if sr.SOH > 0 {
		soh = sql.NullInt64{Int64: int64(sr.SOH), Valid: true}
	}

	res, err := insert(tx, "logging.eg4_system", []col{
		{"logged", sr.Time},
		{"complete", sr.Complete},
		{"units", len(sr.Units)},
		{"any_fault", sr.AnyFault},

		{"pv_power", sr.PVPower},
		{"load_power", sr.LoadPower},
		{"eps_power", sr.EPSPower},
		{"inverter_power", sr.InverterPower},
		{"charge_power", sr.ChargePower},
		{"discharge_power", sr.DischargePower},
		{"battery_power", sr.BatteryPower},
		{"ac_charge_power", sr.ACChargePower},
		{"ac_couple_power", sr.ACCouplePower},
		{"gen_power", sr.GenPower},

		{"battery_voltage", sr.BatteryVoltage},
		{"soc", sr.SOC},
		{"soh", soh},

		{"pv_energy_today", sr.PVEnergyToday},
		{"load_energy_today", sr.LoadEnergyToday},
		{"eps_energy_today", sr.EPSEnergyToday},
		{"charge_energy_today", sr.ChargeEnergyToday},
		{"discharge_energy_today", sr.DischargeEnergyToday},
		{"ac_charge_energy_today", sr.ACChargeEnergyToday},
		{"gen_energy_today", sr.GenEnergyToday},

		{"bms_source", sr.BMSSource},
		{"battery_current_bms", sr.BatteryCurrentBMS},
		{"bms_charge_current_limit", sr.BMSChargeCurrentLimit},
		{"bms_discharge_current_limit", sr.BMSDischargeCurrentLimit},
		{"bms_charge_voltage", sr.BMSChargeVoltage},
		{"bms_discharge_cutoff", sr.BMSDischargeCutoff},
		{"bms_max_cell_voltage", sr.BMSMaxCellVoltage},
		{"bms_min_cell_voltage", sr.BMSMinCellVoltage},
		{"bms_max_cell_temp", sr.BMSMaxCellTemp},
		{"bms_min_cell_temp", sr.BMSMinCellTemp},
		{"bms_cycle_count", sr.BMSCycleCount},
		{"battery_parallel_count", sr.BatteryParallelCount},
		{"battery_capacity_ah", sr.BatteryCapacityAh},
		{"bms_fault_code", sr.BMSFaultCode},
		{"bms_warning_code", sr.BMSWarningCode},
	})
	if err != nil {
		return fmt.Errorf("eg4_system: %w", err)
	}
	systemID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	for _, r := range sr.Units {
		_, err := insert(tx, "logging.eg4_inverter", []col{
			{"system_id", systemID},
			{"logged", r.Time},
			{"name", r.Name},
			{"status", r.Status},
			{"status_text", r.StatusText},

			{"pv1_voltage", r.PV1Voltage},
			{"pv1_current", r.PV1Current},
			{"pv1_power", r.PV1Power},
			{"pv2_voltage", r.PV2Voltage},
			{"pv2_current", r.PV2Current},
			{"pv2_power", r.PV2Power},
			{"pv_power", r.PVPower},

			{"charge_power", r.ChargePower},
			{"discharge_power", r.DischargePower},
			{"battery_power", r.BatteryPower},
			{"battery_current", r.BatteryCurrent},

			{"inverter_power", r.InverterPower},
			{"eps_voltage", r.EPSVoltage},
			{"eps_frequency", r.EPSFrequency},
			{"eps_power", r.EPSPower},
			{"eps_apparent_power", r.EPSApparentPower},
			{"eps_l1_voltage", r.EPSL1Voltage},
			{"eps_l2_voltage", r.EPSL2Voltage},
			{"eps_l1_power", r.EPSL1Power},
			{"eps_l2_power", r.EPSL2Power},
			{"load_power", r.LoadPower},

			{"ac_couple_power", r.ACCouplePower},
			{"ac_charge_power", r.ACChargePower},
			{"ac_input_is_gen", r.ACInputIsGen},
			{"gen_voltage", r.GenVoltage},
			{"gen_frequency", r.GenFrequency},
			{"gen_l1_voltage", r.GenL1Voltage},
			{"gen_l2_voltage", r.GenL2Voltage},
			{"gen_power", r.GenPower},

			{"pv_energy_today", r.PVEnergyToday},
			{"load_energy_today", r.LoadEnergyToday},
			{"eps_energy_today", r.EPSEnergyToday},
			{"charge_energy_today", r.ChargeEnergyToday},
			{"discharge_energy_today", r.DischargeEnergyToday},
			{"ac_charge_energy_today", r.ACChargeEnergyToday},
			{"gen_energy_today", r.GenEnergyToday},

			{"pv_energy_total", r.PVEnergyTotal},
			{"load_energy_total", r.LoadEnergyTotal},
			{"eps_energy_total", r.EPSEnergyTotal},
			{"charge_energy_total", r.ChargeEnergyTotal},
			{"discharge_energy_total", r.DischargeEnergyTotal},
			{"gen_energy_total", r.GenEnergyTotal},

			{"fault_code", r.FaultCode},
			{"warning_code", r.WarningCode},
			{"internal_temp", r.InternalTemp},
			{"radiator1_temp", r.Radiator1Temp},
			{"radiator2_temp", r.Radiator2Temp},
			{"running_time", int64(r.RunningTime.Seconds())},

			{"parallel_role", r.ParallelRole},
			{"parallel_phase", r.ParallelPhase},
			{"parallel_count", r.ParallelCount},
		})
		if err != nil {
			return fmt.Errorf("eg4_inverter %s: %w", r.Name, err)
		}
	}
	return tx.Commit()
}
