-- Drive alerts table for monitoring drive health
CREATE TABLE IF NOT EXISTS drive_alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    drive_id INTEGER NOT NULL REFERENCES tape_drives(id) ON DELETE CASCADE,
    severity TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'warning', 'critical')),
    category TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    resolved BOOLEAN DEFAULT 0,
    resolved_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_drive_alerts_drive ON drive_alerts(drive_id);
CREATE INDEX IF NOT EXISTS idx_drive_alerts_unresolved ON drive_alerts(drive_id, resolved);

-- Drive statistics table for tracking usage metrics
CREATE TABLE IF NOT EXISTS drive_statistics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    drive_id INTEGER NOT NULL UNIQUE REFERENCES tape_drives(id) ON DELETE CASCADE,
    total_bytes_read INTEGER DEFAULT 0,
    total_bytes_written INTEGER DEFAULT 0,
    read_errors INTEGER DEFAULT 0,
    write_errors INTEGER DEFAULT 0,
    total_load_count INTEGER DEFAULT 0,
    cleaning_required BOOLEAN DEFAULT 0,
    last_cleaned_at DATETIME,
    power_on_hours INTEGER DEFAULT 0,
    tape_motion_hours REAL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_drive_statistics_drive ON drive_statistics(drive_id);
