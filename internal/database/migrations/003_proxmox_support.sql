-- Proxmox VE backup and restore support

-- Proxmox nodes table (for tracking cluster/standalone nodes)
CREATE TABLE IF NOT EXISTS proxmox_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_name TEXT NOT NULL UNIQUE,
    host TEXT,
    status TEXT DEFAULT 'unknown',
    cpu_count INTEGER,
    memory_total INTEGER,
    disk_total INTEGER,
    is_cluster_member BOOLEAN DEFAULT 0,
    last_seen_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxmox_nodes_name ON proxmox_nodes(node_name);
CREATE INDEX IF NOT EXISTS idx_proxmox_nodes_status ON proxmox_nodes(status);

-- Proxmox guests table (VMs and LXCs)
CREATE TABLE IF NOT EXISTS proxmox_guests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_name TEXT NOT NULL,
    vmid INTEGER NOT NULL,
    guest_type TEXT NOT NULL CHECK (guest_type IN ('qemu', 'lxc')),
    name TEXT,
    status TEXT,
    cpu_count INTEGER,
    memory_max INTEGER,
    disk_total INTEGER,
    is_template BOOLEAN DEFAULT 0,
    tags TEXT,
    last_backup_id INTEGER REFERENCES proxmox_backups(id),
    last_backup_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(node_name, vmid)
);

CREATE INDEX IF NOT EXISTS idx_proxmox_guests_node ON proxmox_guests(node_name);
CREATE INDEX IF NOT EXISTS idx_proxmox_guests_vmid ON proxmox_guests(vmid);
CREATE INDEX IF NOT EXISTS idx_proxmox_guests_type ON proxmox_guests(guest_type);

-- Proxmox backups table
CREATE TABLE IF NOT EXISTS proxmox_backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node TEXT NOT NULL,
    vmid INTEGER NOT NULL,
    guest_type TEXT NOT NULL CHECK (guest_type IN ('qemu', 'lxc')),
    guest_name TEXT,
    tape_id INTEGER NOT NULL REFERENCES tapes(id),
    backup_mode TEXT NOT NULL CHECK (backup_mode IN ('snapshot', 'suspend', 'stop')),
    compress TEXT CHECK (compress IN ('zstd', 'lzo', 'gzip', '')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    total_bytes INTEGER DEFAULT 0,
    config_data TEXT,  -- JSON: full VM/LXC configuration at backup time
    tape_block_start INTEGER,
    tape_block_end INTEGER,
    tape_file_number INTEGER,
    error_message TEXT,
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxmox_backups_node ON proxmox_backups(node);
CREATE INDEX IF NOT EXISTS idx_proxmox_backups_vmid ON proxmox_backups(vmid);
CREATE INDEX IF NOT EXISTS idx_proxmox_backups_tape ON proxmox_backups(tape_id);
CREATE INDEX IF NOT EXISTS idx_proxmox_backups_status ON proxmox_backups(status);
CREATE INDEX IF NOT EXISTS idx_proxmox_backups_time ON proxmox_backups(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_proxmox_backups_guest_type ON proxmox_backups(guest_type);

-- Proxmox restores table
CREATE TABLE IF NOT EXISTS proxmox_restores (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_id INTEGER NOT NULL REFERENCES proxmox_backups(id),
    source_node TEXT NOT NULL,
    target_node TEXT NOT NULL,
    source_vmid INTEGER NOT NULL,
    target_vmid INTEGER NOT NULL,
    guest_type TEXT NOT NULL CHECK (guest_type IN ('qemu', 'lxc')),
    guest_name TEXT,
    target_storage TEXT,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    error_message TEXT,
    config_applied BOOLEAN DEFAULT 0,
    started_after_restore BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxmox_restores_backup ON proxmox_restores(backup_id);
CREATE INDEX IF NOT EXISTS idx_proxmox_restores_status ON proxmox_restores(status);
CREATE INDEX IF NOT EXISTS idx_proxmox_restores_time ON proxmox_restores(start_time DESC);

-- Proxmox backup jobs table (scheduled backups)
CREATE TABLE IF NOT EXISTS proxmox_backup_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    node TEXT,  -- NULL means all nodes
    vmid_filter TEXT,  -- JSON array of VMIDs to include, NULL means all
    guest_type_filter TEXT CHECK (guest_type_filter IN ('qemu', 'lxc', 'all')),
    tag_filter TEXT,  -- Comma-separated tags to match
    pool_id INTEGER REFERENCES tape_pools(id),
    backup_mode TEXT NOT NULL DEFAULT 'snapshot' CHECK (backup_mode IN ('snapshot', 'suspend', 'stop')),
    compress TEXT DEFAULT 'zstd' CHECK (compress IN ('zstd', 'lzo', 'gzip', '')),
    schedule_cron TEXT,  -- Cron expression (with seconds)
    retention_days INTEGER DEFAULT 30,
    enabled BOOLEAN DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxmox_jobs_enabled ON proxmox_backup_jobs(enabled);
CREATE INDEX IF NOT EXISTS idx_proxmox_jobs_next_run ON proxmox_backup_jobs(next_run_at);

-- Proxmox job executions table
CREATE TABLE IF NOT EXISTS proxmox_job_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL REFERENCES proxmox_backup_jobs(id),
    tape_id INTEGER REFERENCES tapes(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'partial')),
    start_time DATETIME,
    end_time DATETIME,
    guests_total INTEGER DEFAULT 0,
    guests_completed INTEGER DEFAULT 0,
    guests_failed INTEGER DEFAULT 0,
    total_bytes INTEGER DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxmox_executions_job ON proxmox_job_executions(job_id);
CREATE INDEX IF NOT EXISTS idx_proxmox_executions_status ON proxmox_job_executions(status);
CREATE INDEX IF NOT EXISTS idx_proxmox_executions_time ON proxmox_job_executions(start_time DESC);

-- Trigger to update updated_at timestamps
CREATE TRIGGER IF NOT EXISTS update_proxmox_nodes_timestamp 
    AFTER UPDATE ON proxmox_nodes
BEGIN
    UPDATE proxmox_nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_proxmox_guests_timestamp 
    AFTER UPDATE ON proxmox_guests
BEGIN
    UPDATE proxmox_guests SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_proxmox_backups_timestamp 
    AFTER UPDATE ON proxmox_backups
BEGIN
    UPDATE proxmox_backups SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_proxmox_restores_timestamp 
    AFTER UPDATE ON proxmox_restores
BEGIN
    UPDATE proxmox_restores SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_proxmox_jobs_timestamp 
    AFTER UPDATE ON proxmox_backup_jobs
BEGIN
    UPDATE proxmox_backup_jobs SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_proxmox_executions_timestamp 
    AFTER UPDATE ON proxmox_job_executions
BEGIN
    UPDATE proxmox_job_executions SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
