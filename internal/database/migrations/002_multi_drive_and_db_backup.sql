-- Multi-drive support and database backup capability

-- Add enabled flag and display name to drives
ALTER TABLE tape_drives ADD COLUMN enabled BOOLEAN DEFAULT 1;
ALTER TABLE tape_drives ADD COLUMN display_name TEXT;

-- Database backup tracking table
CREATE TABLE IF NOT EXISTS database_backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tape_id INTEGER NOT NULL REFERENCES tapes(id),
    backup_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    file_size INTEGER NOT NULL DEFAULT 0,
    checksum TEXT,
    block_offset INTEGER,
    status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_database_backups_tape ON database_backups(tape_id);
CREATE INDEX IF NOT EXISTS idx_database_backups_time ON database_backups(backup_time DESC);

-- Add restore destination type to track network restores
CREATE TABLE IF NOT EXISTS restore_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_set_id INTEGER REFERENCES backup_sets(id),
    destination_type TEXT NOT NULL CHECK (destination_type IN ('local', 'smb', 'nfs')),
    destination_path TEXT NOT NULL,
    files_requested INTEGER DEFAULT 0,
    files_restored INTEGER DEFAULT 0,
    bytes_restored INTEGER DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    error_message TEXT,
    verify_enabled BOOLEAN DEFAULT 0,
    verify_passed BOOLEAN,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_restore_operations_status ON restore_operations(status);
CREATE INDEX IF NOT EXISTS idx_restore_operations_time ON restore_operations(created_at DESC);
