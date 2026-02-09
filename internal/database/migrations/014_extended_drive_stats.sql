-- Add extended drive statistics columns from sg_logs
ALTER TABLE drive_statistics ADD COLUMN temperature_c INTEGER DEFAULT 0;
ALTER TABLE drive_statistics ADD COLUMN lifetime_power_cycles INTEGER DEFAULT 0;
ALTER TABLE drive_statistics ADD COLUMN read_compression_pct INTEGER DEFAULT 0;
ALTER TABLE drive_statistics ADD COLUMN write_compression_pct INTEGER DEFAULT 0;
ALTER TABLE drive_statistics ADD COLUMN tape_alert_flags TEXT DEFAULT '';
