package main

import (
	"database/sql"
	"fmt"

	"SystemController/EG4"
)

// logEG4 writes one poll of the EG4 system to the database: a row in eg4_system
// for the combined values and a row in eg4_inverter for each inverter that
// answered. Both are written in one transaction so a poll is never half logged.
// The tables are defined in EG4/eg4_tables.sql.
func logEG4(db *sql.DB, sr *EG4.SystemReading) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	res, err := tx.Exec(`INSERT INTO logging.eg4_system (logged, complete, units, any_fault,
                                  pv_power, load_power, eps_power, inverter_power, charge_power, discharge_power,
                                  battery_power, ac_charge_power, ac_couple_power, gen_power,
                                  battery_voltage, soc,
                                  pv_energy_today, load_energy_today, eps_energy_today, charge_energy_today,
                                  discharge_energy_today, ac_charge_energy_today, gen_energy_today)
                         VALUES (?,?,?,?,
                                 ?,?,?,?,?,?,
                                 ?,?,?,?,
                                 ?,?,
                                 ?,?,?,?,
                                 ?,?,?)`,
		sr.Time, sr.Complete, len(sr.Units), sr.AnyFault,
		sr.PVPower, sr.LoadPower, sr.EPSPower, sr.InverterPower, sr.ChargePower, sr.DischargePower,
		sr.BatteryPower, sr.ACChargePower, sr.ACCouplePower, sr.GenPower,
		sr.BatteryVoltage, sr.SOC,
		sr.PVEnergyToday, sr.LoadEnergyToday, sr.EPSEnergyToday, sr.ChargeEnergyToday,
		sr.DischargeEnergyToday, sr.ACChargeEnergyToday, sr.GenEnergyToday)
	if err != nil {
		return fmt.Errorf("eg4_system: %w", err)
	}
	systemID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	for _, r := range sr.Units {
		if _, err := tx.Exec(`INSERT INTO logging.eg4_inverter (system_id, logged, name, status, status_text,
                                  pv1_voltage, pv1_current, pv1_power, pv2_voltage, pv2_current, pv2_power, pv_power,
                                  battery_voltage, soc, soh, charge_power, discharge_power, battery_power, battery_current_bms,
                                  inverter_power, eps_voltage, eps_frequency, eps_power, eps_apparent_power,
                                  eps_l1_voltage, eps_l2_voltage, eps_l1_power, eps_l2_power, load_power,
                                  ac_couple_power, ac_charge_power, ac_input_is_gen,
                                  gen_voltage, gen_frequency, gen_l1_voltage, gen_l2_voltage, gen_power,
                                  pv_energy_today, load_energy_today, eps_energy_today, charge_energy_today,
                                  discharge_energy_today, ac_charge_energy_today, gen_energy_today,
                                  pv_energy_total, load_energy_total, eps_energy_total, charge_energy_total,
                                  discharge_energy_total, gen_energy_total,
                                  fault_code, warning_code, internal_temp, radiator1_temp, radiator2_temp, running_time,
                                  bms_charge_current_limit, bms_discharge_current_limit, bms_charge_voltage, bms_discharge_cutoff,
                                  bms_max_cell_voltage, bms_min_cell_voltage, bms_max_cell_temp, bms_min_cell_temp,
                                  bms_cycle_count, battery_parallel_count, battery_capacity_ah, bms_fault_code, bms_warning_code,
                                  parallel_role, parallel_phase, parallel_count)
                         VALUES (?,?,?,?,?,
                                 ?,?,?,?,?,?,?,
                                 ?,?,?,?,?,?,?,
                                 ?,?,?,?,?,
                                 ?,?,?,?,?,
                                 ?,?,?,
                                 ?,?,?,?,?,
                                 ?,?,?,?,
                                 ?,?,?,
                                 ?,?,?,?,
                                 ?,?,
                                 ?,?,?,?,?,?,
                                 ?,?,?,?,
                                 ?,?,?,?,
                                 ?,?,?,?,?,
                                 ?,?,?)`,
			systemID, r.Time, r.Name, r.Status, r.StatusText,
			r.PV1Voltage, r.PV1Current, r.PV1Power, r.PV2Voltage, r.PV2Current, r.PV2Power, r.PVPower,
			r.BatteryVoltage, r.SOC, r.SOH, r.ChargePower, r.DischargePower, r.BatteryPower, r.BatteryCurrentBMS,
			r.InverterPower, r.EPSVoltage, r.EPSFrequency, r.EPSPower, r.EPSApparentPower,
			r.EPSL1Voltage, r.EPSL2Voltage, r.EPSL1Power, r.EPSL2Power, r.LoadPower,
			r.ACCouplePower, r.ACChargePower, r.ACInputIsGen,
			r.GenVoltage, r.GenFrequency, r.GenL1Voltage, r.GenL2Voltage, r.GenPower,
			r.PVEnergyToday, r.LoadEnergyToday, r.EPSEnergyToday, r.ChargeEnergyToday,
			r.DischargeEnergyToday, r.ACChargeEnergyToday, r.GenEnergyToday,
			r.PVEnergyTotal, r.LoadEnergyTotal, r.EPSEnergyTotal, r.ChargeEnergyTotal,
			r.DischargeEnergyTotal, r.GenEnergyTotal,
			r.FaultCode, r.WarningCode, r.InternalTemp, r.Radiator1Temp, r.Radiator2Temp, int64(r.RunningTime.Seconds()),
			r.BMSChargeCurrentLimit, r.BMSDischargeCurrentLimit, r.BMSChargeVoltage, r.BMSDischargeCutoff,
			r.BMSMaxCellVoltage, r.BMSMinCellVoltage, r.BMSMaxCellTemp, r.BMSMinCellTemp,
			r.BMSCycleCount, r.BatteryParallelCount, r.BatteryCapacityAh, r.BMSFaultCode, r.BMSWarningCode,
			r.ParallelRole, r.ParallelPhase, r.ParallelCount); err != nil {
			return fmt.Errorf("eg4_inverter %s: %w", r.Name, err)
		}
	}
	return tx.Commit()
}
