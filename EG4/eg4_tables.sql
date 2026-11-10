-- Tables for logging the paralleled EG4 12000XP inverters (off grid).
-- One eg4_system row per poll holds the combined values; one eg4_inverter row
-- per inverter per poll holds that unit's values and points back to its poll.

CREATE TABLE IF NOT EXISTS logging.eg4_system (
    id                       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    logged                   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    complete                 TINYINT(1)      NOT NULL,           -- every inverter answered
    units                    TINYINT UNSIGNED NOT NULL,          -- inverters included in the totals
    any_fault                TINYINT(1)      NOT NULL,

    -- Power, W (summed across inverters)
    pv_power                 INT             NOT NULL,
    load_power               INT             NOT NULL,
    eps_power                INT             NOT NULL,
    inverter_power           INT             NOT NULL,
    charge_power             INT             NOT NULL,
    discharge_power          INT             NOT NULL,
    battery_power            INT             NOT NULL,           -- + charging / - discharging
    ac_charge_power          INT             NOT NULL,           -- rectifier, charging from the generator
    ac_couple_power          INT             NOT NULL,
    gen_power                INT             NOT NULL,

    -- Shared battery bank (averaged across inverters)
    battery_voltage          DECIMAL(5,2)    NOT NULL,
    soc                      DECIMAL(5,2)    NOT NULL,
    soh                      TINYINT UNSIGNED NULL,              -- NULL when the BMS does not report it

    -- Energy today, kWh (summed across inverters)
    pv_energy_today          DECIMAL(8,1)    NOT NULL,
    load_energy_today        DECIMAL(8,1)    NOT NULL,
    eps_energy_today         DECIMAL(8,1)    NOT NULL,
    charge_energy_today      DECIMAL(8,1)    NOT NULL,
    discharge_energy_today   DECIMAL(8,1)    NOT NULL,
    ac_charge_energy_today   DECIMAL(8,1)    NOT NULL,
    gen_energy_today         DECIMAL(8,1)    NOT NULL,

    PRIMARY KEY (id),
    KEY idx_eg4_system_logged (logged)
) ENGINE = InnoDB;

CREATE TABLE IF NOT EXISTS logging.eg4_inverter (
    id                       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    system_id                BIGINT UNSIGNED NOT NULL,
    logged                   DATETIME(3)     NOT NULL,
    name                     VARCHAR(16)     NOT NULL,           -- XP-1, XP-2
    status                   SMALLINT UNSIGNED NOT NULL,
    status_text              VARCHAR(40)     NOT NULL,

    -- PV
    pv1_voltage              DECIMAL(5,1)    NOT NULL,
    pv1_current              DECIMAL(5,2)    NOT NULL,
    pv1_power                INT             NOT NULL,
    pv2_voltage              DECIMAL(5,1)    NOT NULL,
    pv2_current              DECIMAL(5,2)    NOT NULL,
    pv2_power                INT             NOT NULL,
    pv_power                 INT             NOT NULL,

    -- Battery (voltage, SOC and SOH are in eg4_system)
    charge_power             INT             NOT NULL,
    discharge_power          INT             NOT NULL,
    battery_power            INT             NOT NULL,           -- + charging / - discharging
    battery_current_bms      DECIMAL(6,1)    NOT NULL,           -- + charging / - discharging

    -- Inverter / EPS output
    inverter_power           INT             NOT NULL,
    eps_voltage              DECIMAL(5,1)    NOT NULL,
    eps_frequency            DECIMAL(5,2)    NOT NULL,
    eps_power                INT             NOT NULL,
    eps_apparent_power       INT             NOT NULL,
    eps_l1_voltage           DECIMAL(5,1)    NOT NULL,
    eps_l2_voltage           DECIMAL(5,1)    NOT NULL,
    eps_l1_power             INT             NOT NULL,
    eps_l2_power             INT             NOT NULL,
    load_power               INT             NOT NULL,

    -- AC coupling and generator
    ac_couple_power          INT             NOT NULL,
    ac_charge_power          INT             NOT NULL,
    ac_input_is_gen          TINYINT(1)      NOT NULL,
    gen_voltage              DECIMAL(5,1)    NOT NULL,
    gen_frequency            DECIMAL(5,2)    NOT NULL,
    gen_l1_voltage           DECIMAL(5,1)    NOT NULL,
    gen_l2_voltage           DECIMAL(5,1)    NOT NULL,
    gen_power                INT             NOT NULL,

    -- Energy today, kWh
    pv_energy_today          DECIMAL(8,1)    NOT NULL,
    load_energy_today        DECIMAL(8,1)    NOT NULL,
    eps_energy_today         DECIMAL(8,1)    NOT NULL,
    charge_energy_today      DECIMAL(8,1)    NOT NULL,
    discharge_energy_today   DECIMAL(8,1)    NOT NULL,
    ac_charge_energy_today   DECIMAL(8,1)    NOT NULL,
    gen_energy_today         DECIMAL(8,1)    NOT NULL,

    -- Lifetime energy, kWh
    pv_energy_total          DECIMAL(12,1)   NOT NULL,
    load_energy_total        DECIMAL(12,1)   NOT NULL,
    eps_energy_total         DECIMAL(12,1)   NOT NULL,
    charge_energy_total      DECIMAL(12,1)   NOT NULL,
    discharge_energy_total   DECIMAL(12,1)   NOT NULL,
    gen_energy_total         DECIMAL(12,1)   NOT NULL,

    -- Health
    fault_code               INT UNSIGNED    NOT NULL,
    warning_code             INT UNSIGNED    NOT NULL,
    internal_temp            SMALLINT        NOT NULL,
    radiator1_temp           SMALLINT        NOT NULL,
    radiator2_temp           SMALLINT        NOT NULL,
    running_time             INT UNSIGNED    NOT NULL,           -- seconds

    -- BMS
    bms_charge_current_limit    DECIMAL(6,1) NOT NULL,
    bms_discharge_current_limit DECIMAL(6,1) NOT NULL,
    bms_charge_voltage          DECIMAL(5,1) NOT NULL,
    bms_discharge_cutoff        DECIMAL(5,1) NOT NULL,
    bms_max_cell_voltage        DECIMAL(5,3) NOT NULL,
    bms_min_cell_voltage        DECIMAL(5,3) NOT NULL,
    bms_max_cell_temp           DECIMAL(5,1) NOT NULL,
    bms_min_cell_temp           DECIMAL(5,1) NOT NULL,
    bms_cycle_count             INT UNSIGNED NOT NULL,
    battery_parallel_count      SMALLINT UNSIGNED NOT NULL,
    battery_capacity_ah         INT UNSIGNED NOT NULL,
    bms_fault_code              SMALLINT UNSIGNED NOT NULL,
    bms_warning_code            SMALLINT UNSIGNED NOT NULL,

    -- Parallel
    parallel_role            TINYINT UNSIGNED NOT NULL,          -- 0 standalone, 1 master, 2 slave
    parallel_phase           TINYINT UNSIGNED NOT NULL,
    parallel_count           TINYINT UNSIGNED NOT NULL,

    PRIMARY KEY (id),
    KEY idx_eg4_inverter_name_logged (name, logged),
    CONSTRAINT fk_eg4_inverter_system FOREIGN KEY (system_id)
        REFERENCES logging.eg4_system (id) ON DELETE CASCADE
) ENGINE = InnoDB;
