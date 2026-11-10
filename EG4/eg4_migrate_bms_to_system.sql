-- Migration for databases created from an earlier eg4_tables.sql.
-- Run after eg4_migrate_battery_to_system.sql, with the new SystemController
-- build deployed.
--
-- There is one battery bank, so the BMS values move from eg4_inverter to
-- eg4_system. eg4_inverter gets battery_current instead: that inverter's
-- share of the battery current, battery_power / battery voltage.

ALTER TABLE logging.eg4_system
    ADD COLUMN bms_source                  VARCHAR(16)       NOT NULL DEFAULT '',
    ADD COLUMN battery_current_bms         DECIMAL(6,1)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_charge_current_limit    DECIMAL(6,1)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_discharge_current_limit DECIMAL(6,1)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_charge_voltage          DECIMAL(5,1)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_discharge_cutoff        DECIMAL(5,1)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_max_cell_voltage        DECIMAL(5,3)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_min_cell_voltage        DECIMAL(5,3)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_max_cell_temp           DECIMAL(5,1)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_min_cell_temp           DECIMAL(5,1)      NOT NULL DEFAULT 0,
    ADD COLUMN bms_cycle_count             INT UNSIGNED      NOT NULL DEFAULT 0,
    ADD COLUMN battery_parallel_count      SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN battery_capacity_ah         INT UNSIGNED      NOT NULL DEFAULT 0,
    ADD COLUMN bms_fault_code              SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN bms_warning_code            SMALLINT UNSIGNED NOT NULL DEFAULT 0;

ALTER TABLE logging.eg4_inverter
    ADD COLUMN battery_current DECIMAL(6,1) NOT NULL DEFAULT 0 AFTER battery_power;

-- Copy the BMS values into each system row from one of its inverter rows,
-- the parallel master if there is one, as the program now does.
UPDATE logging.eg4_system s
    JOIN logging.eg4_inverter i
      ON i.id = (SELECT i2.id
                   FROM logging.eg4_inverter i2
                  WHERE i2.system_id = s.id
                  ORDER BY (i2.parallel_role = 1) DESC, i2.id
                  LIMIT 1)
   SET s.bms_source                  = i.name,
       s.battery_current_bms         = i.battery_current_bms,
       s.bms_charge_current_limit    = i.bms_charge_current_limit,
       s.bms_discharge_current_limit = i.bms_discharge_current_limit,
       s.bms_charge_voltage          = i.bms_charge_voltage,
       s.bms_discharge_cutoff        = i.bms_discharge_cutoff,
       s.bms_max_cell_voltage        = i.bms_max_cell_voltage,
       s.bms_min_cell_voltage        = i.bms_min_cell_voltage,
       s.bms_max_cell_temp           = i.bms_max_cell_temp,
       s.bms_min_cell_temp           = i.bms_min_cell_temp,
       s.bms_cycle_count             = i.bms_cycle_count,
       s.battery_parallel_count      = i.battery_parallel_count,
       s.battery_capacity_ah         = i.battery_capacity_ah,
       s.bms_fault_code              = i.bms_fault_code,
       s.bms_warning_code            = i.bms_warning_code;

-- Per-inverter battery current, using the bank voltage now held in eg4_system.
UPDATE logging.eg4_inverter i
    JOIN logging.eg4_system s ON s.id = i.system_id
   SET i.battery_current = i.battery_power / s.battery_voltage
 WHERE s.battery_voltage > 0;

-- The defaults were only needed to add the columns to existing rows.
ALTER TABLE logging.eg4_system
    ALTER COLUMN bms_source                  DROP DEFAULT,
    ALTER COLUMN battery_current_bms         DROP DEFAULT,
    ALTER COLUMN bms_charge_current_limit    DROP DEFAULT,
    ALTER COLUMN bms_discharge_current_limit DROP DEFAULT,
    ALTER COLUMN bms_charge_voltage          DROP DEFAULT,
    ALTER COLUMN bms_discharge_cutoff        DROP DEFAULT,
    ALTER COLUMN bms_max_cell_voltage        DROP DEFAULT,
    ALTER COLUMN bms_min_cell_voltage        DROP DEFAULT,
    ALTER COLUMN bms_max_cell_temp           DROP DEFAULT,
    ALTER COLUMN bms_min_cell_temp           DROP DEFAULT,
    ALTER COLUMN bms_cycle_count             DROP DEFAULT,
    ALTER COLUMN battery_parallel_count      DROP DEFAULT,
    ALTER COLUMN battery_capacity_ah         DROP DEFAULT,
    ALTER COLUMN bms_fault_code              DROP DEFAULT,
    ALTER COLUMN bms_warning_code            DROP DEFAULT;

ALTER TABLE logging.eg4_inverter
    ALTER COLUMN battery_current DROP DEFAULT,
    DROP COLUMN battery_current_bms,
    DROP COLUMN bms_charge_current_limit,
    DROP COLUMN bms_discharge_current_limit,
    DROP COLUMN bms_charge_voltage,
    DROP COLUMN bms_discharge_cutoff,
    DROP COLUMN bms_max_cell_voltage,
    DROP COLUMN bms_min_cell_voltage,
    DROP COLUMN bms_max_cell_temp,
    DROP COLUMN bms_min_cell_temp,
    DROP COLUMN bms_cycle_count,
    DROP COLUMN battery_parallel_count,
    DROP COLUMN battery_capacity_ah,
    DROP COLUMN bms_fault_code,
    DROP COLUMN bms_warning_code;
