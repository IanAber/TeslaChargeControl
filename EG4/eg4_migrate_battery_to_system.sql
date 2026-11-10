-- Migration for databases created from an earlier eg4_tables.sql:
-- battery voltage, SOC and SOH belong to the shared battery bank, so they live
-- in eg4_system only. SOH is NULL when the BMS does not report it.
-- Run once, with the new SystemController build deployed.

ALTER TABLE logging.eg4_system
    ADD COLUMN soh TINYINT UNSIGNED NULL AFTER soc;

-- Copy SOH from the existing inverter rows, treating 0 as "not reported".
UPDATE logging.eg4_system s
    JOIN (SELECT system_id, ROUND(AVG(NULLIF(soh, 0))) AS soh
            FROM logging.eg4_inverter
           GROUP BY system_id) i ON i.system_id = s.id
   SET s.soh = i.soh;

ALTER TABLE logging.eg4_inverter
    DROP COLUMN battery_voltage,
    DROP COLUMN soc,
    DROP COLUMN soh;
