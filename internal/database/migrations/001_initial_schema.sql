-- Initial schema

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'readonly')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tape pools table
CREATE TABLE IF NOT EXISTS tape_pools (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    retention_days INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tapes table
CREATE TABLE IF NOT EXISTS tapes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    barcode TEXT UNIQUE,
    label TEXT NOT NULL,
    pool_id INTEGER REFERENCES tape_pools(id),
    status TEXT NOT NULL CHECK (status IN ('blank', 'active', 'full', 'retired', 'offsite')),
    capacity_bytes INTEGER DEFAULT 0,
    used_bytes INTEGER DEFAULT 0,
    write_count INTEGER DEFAULT 0,
    last_written_at DATETIME,
    offsite_location TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tape drives table
CREATE TABLE IF NOT EXISTS tape_drives (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_path TEXT NOT NULL UNIQUE,
    serial_number TEXT,
    model TEXT,
    status TEXT NOT NULL CHECK (status IN ('ready', 'busy', 'offline', 'error')),
    current_tape_id INTEGER REFERENCES tapes(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Backup sources table
CREATE TABLE IF NOT EXISTS backup_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('local', 'smb', 'nfs')),
    path TEXT NOT NULL,
    include_patterns TEXT,
    exclude_patterns TEXT,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Backup jobs table
CREATE TABLE IF NOT EXISTS backup_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    source_id INTEGER NOT NULL REFERENCES backup_sources(id),
    pool_id INTEGER NOT NULL REFERENCES tape_pools(id),
    backup_type TEXT NOT NULL CHECK (backup_type IN ('full', 'incremental')),
    schedule_cron TEXT,
    retention_days INTEGER DEFAULT 30,
    enabled BOOLEAN DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Backup sets table
CREATE TABLE IF NOT EXISTS backup_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL REFERENCES backup_jobs(id),
    tape_id INTEGER NOT NULL REFERENCES tapes(id),
    backup_type TEXT NOT NULL CHECK (backup_type IN ('full', 'incremental')),
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    file_count INTEGER DEFAULT 0,
    total_bytes INTEGER DEFAULT 0,
    start_block INTEGER,
    end_block INTEGER,
    checksum TEXT,
    parent_set_id INTEGER REFERENCES backup_sets(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Catalog entries table
CREATE TABLE IF NOT EXISTS catalog_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_set_id INTEGER NOT NULL REFERENCES backup_sets(id),
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_mode INTEGER,
    mod_time DATETIME,
    checksum TEXT,
    block_offset INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(backup_set_id, file_path)
);

CREATE INDEX IF NOT EXISTS idx_catalog_path ON catalog_entries(file_path);
CREATE INDEX IF NOT EXISTS idx_catalog_backup_set ON catalog_entries(backup_set_id);

-- Job executions table
CREATE TABLE IF NOT EXISTS job_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL REFERENCES backup_jobs(id),
    backup_set_id INTEGER REFERENCES backup_sets(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'paused')),
    start_time DATETIME,
    end_time DATETIME,
    files_processed INTEGER DEFAULT 0,
    bytes_processed INTEGER DEFAULT 0,
    error_message TEXT,
    can_resume BOOLEAN DEFAULT 0,
    resume_state TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Audit logs table
CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER REFERENCES users(id),
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id INTEGER,
    details TEXT,
    ip_address TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_logs(resource_type, resource_id);

-- Snapshots table
CREATE TABLE IF NOT EXISTS snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL REFERENCES backup_sources(id),
    backup_set_id INTEGER REFERENCES backup_sets(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    file_count INTEGER DEFAULT 0,
    total_bytes INTEGER DEFAULT 0,
    snapshot_data BLOB
);

CREATE INDEX IF NOT EXISTS idx_snapshot_source ON snapshots(source_id, created_at DESC);

-- Insert default admin user (password: changeme)
INSERT INTO users (username, password_hash, role) 
VALUES ('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZRGdjGj/n3.0rl94xCzPfL5MWzKDy', 'admin');

-- Insert default tape pools
INSERT INTO tape_pools (name, description, retention_days) VALUES 
    ('DAILY', 'Daily backup tapes', 7),
    ('WEEKLY', 'Weekly backup tapes', 30),
    ('MONTHLY', 'Monthly backup tapes', 365),
    ('ARCHIVE', 'Archive tapes for long-term retention', 0);
