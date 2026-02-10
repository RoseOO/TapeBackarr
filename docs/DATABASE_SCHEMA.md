# TapeBackarr Database Schema

## Entity Relationship Diagram

```
┌─────────────────────┐   ┌─────────────────────┐   ┌─────────────────────┐
│       Users         │   │     TapePools       │   │    TapeDrives       │
├─────────────────────┤   ├─────────────────────┤   ├─────────────────────┤
│ id (PK)             │   │ id (PK)             │   │ id (PK)             │
│ username            │   │ name                │   │ device_path         │
│ password_hash       │   │ description         │   │ serial_number       │
│ role                │   │ retention_days      │   │ model               │
│ created_at          │   │ allow_reuse         │   │ vendor              │
│ updated_at          │   │ allocation_policy   │   │ display_name        │
└─────────────────────┘   │ created_at          │   │ enabled             │
                          │ updated_at          │   │ status              │
                          └─────────────────────┘   │ current_tape_id(FK) │
                                  │                 │ created_at          │
                                  │                 │ updated_at          │
                                  │                 └─────────────────────┘
                                  │                         │
                                  ▼                         │
┌─────────────────────┐   ┌─────────────────────┐          │
│   BackupSources     │   │       Tapes         │◀─────────┘
├─────────────────────┤   ├─────────────────────┤
│ id (PK)             │   │ id (PK)             │
│ name                │   │ uuid                │
│ source_type         │   │ barcode             │
│ path                │   │ label               │
│ include_patterns    │   │ pool_id (FK)        │──────┐
│ exclude_patterns    │   │ status              │      │
│ enabled             │   │ lto_type            │      │
│ created_at          │   │ capacity_bytes      │      │
│ updated_at          │   │ used_bytes          │      │
└─────────────────────┘   │ write_count         │      │
        │                 │ last_written_at     │      │
        │                 │ offsite_location    │      │
        │                 │ export_time         │      │
        │                 │ import_time         │      │
        │                 │ labeled_at          │      │
        │                 │ encryption_key_*    │      │
        │                 │ created_at          │      │
        │                 │ updated_at          │      │
        │                 └─────────────────────┘      │
        │                         │                    │
        ▼                         ▼                    │
┌─────────────────────┐   ┌─────────────────────┐     │
│    BackupJobs       │   │    BackupSets       │◀────┘
├─────────────────────┤   ├─────────────────────┤
│ id (PK)             │   │ id (PK)             │
│ name                │   │ job_id (FK)         │──────────┐
│ source_id (FK)      │─┐ │ tape_id (FK)        │          │
│ pool_id (FK)        │─┤ │ backup_type         │          │
│ backup_type         │ │ │ start_time          │          │
│ schedule_cron       │ │ │ end_time            │          │
│ retention_days      │ │ │ status              │          │
│ enabled             │ │ │ file_count          │          │
│ encryption_enabled  │ │ │ total_bytes         │          │
│ encryption_key_id   │ │ │ start_block         │          │
│ compression         │ │ │ end_block           │          │
│ last_run_at         │ │ │ checksum            │          │
│ next_run_at         │ │ │ encrypted           │          │
│ created_at          │ │ │ encryption_key_id   │          │
│ updated_at          │ │ │ compressed          │          │
└─────────────────────┘ │ │ compression_type    │          │
        │               │ │ parent_set_id (FK)  │          │
        │               │ │ created_at          │          │
        ▼               │ │ updated_at          │          │
┌─────────────────────┐ │ └─────────────────────┘          │
│   JobExecutions     │ │         │                        │
├─────────────────────┤ │         │                        │
│ id (PK)             │ │         ▼                        │
│ job_id (FK)         │─┘ ┌─────────────────────┐          │
│ backup_set_id (FK)  │   │  CatalogEntries     │          │
│ status              │   ├─────────────────────┤          │
│ start_time          │   │ id (PK)             │          │
│ end_time            │   │ backup_set_id (FK)  │──────────┘
│ files_processed     │   │ file_path           │
│ bytes_processed     │   │ file_size           │
│ error_message       │   │ file_mode           │
│ can_resume          │   │ mod_time            │
│ resume_state        │   │ checksum            │
│ created_at          │   │ block_offset        │
│ updated_at          │   │ created_at          │
└─────────────────────┘   └─────────────────────┘

┌─────────────────────┐   ┌─────────────────────┐   ┌─────────────────────┐
│    AuditLogs        │   │     Snapshots       │   │  EncryptionKeys     │
├─────────────────────┤   ├─────────────────────┤   ├─────────────────────┤
│ id (PK)             │   │ id (PK)             │   │ id (PK)             │
│ user_id (FK)        │   │ source_id (FK)      │   │ name                │
│ action              │   │ backup_set_id (FK)  │   │ algorithm           │
│ resource_type       │   │ created_at          │   │ key_data            │
│ resource_id         │   │ file_count          │   │ key_fingerprint     │
│ details (JSON)      │   │ total_bytes         │   │ description         │
│ ip_address          │   │ snapshot_data       │   │ created_at          │
│ created_at          │   └─────────────────────┘   │ updated_at          │
└─────────────────────┘                             └─────────────────────┘

Additional tables not shown: database_backups, restore_operations, api_keys,
proxmox_nodes, proxmox_guests, proxmox_backups, proxmox_restores,
proxmox_backup_jobs, proxmox_job_executions, tape_spanning_sets,
tape_spanning_members, tape_change_requests, tape_libraries,
tape_library_slots, drive_statistics, drive_alerts (see definitions below).
```

## Table Definitions

### Users
Stores user accounts for web UI authentication.

```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'readonly')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### TapePools
Groups tapes by purpose/policy (e.g., WEEKLY, MONTHLY, ARCHIVE).

```sql
CREATE TABLE tape_pools (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    retention_days INTEGER DEFAULT 0,
    allow_reuse INTEGER DEFAULT 1,
    allocation_policy TEXT DEFAULT 'continue',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Tapes
Individual tape media tracking with UUID labels and PBS-style lifecycle states.

```sql
CREATE TABLE tapes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT,
    barcode TEXT UNIQUE,
    label TEXT NOT NULL,
    pool_id INTEGER REFERENCES tape_pools(id),
    status TEXT NOT NULL CHECK (status IN ('blank', 'active', 'full', 'expired', 'retired', 'exported')),
    lto_type TEXT DEFAULT '',
    capacity_bytes INTEGER DEFAULT 0,
    used_bytes INTEGER DEFAULT 0,
    write_count INTEGER DEFAULT 0,
    last_written_at DATETIME,
    offsite_location TEXT,
    export_time DATETIME,
    import_time DATETIME,
    labeled_at DATETIME,
    encryption_key_fingerprint TEXT DEFAULT '',
    encryption_key_name TEXT DEFAULT '',
    format_type TEXT NOT NULL DEFAULT 'raw' CHECK (format_type IN ('raw', 'ltfs')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### TapeDrives
Physical tape drive tracking with multi-drive support.

```sql
CREATE TABLE tape_drives (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_path TEXT NOT NULL UNIQUE,
    serial_number TEXT,
    model TEXT,
    vendor TEXT DEFAULT '',
    display_name TEXT,
    enabled INTEGER DEFAULT 1,
    status TEXT NOT NULL CHECK (status IN ('ready', 'busy', 'offline', 'error')),
    current_tape_id INTEGER REFERENCES tapes(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### BackupSources
Configured backup source paths.

```sql
CREATE TABLE backup_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('local', 'smb', 'nfs')),
    path TEXT NOT NULL,
    include_patterns TEXT,  -- JSON array of glob patterns
    exclude_patterns TEXT,  -- JSON array of glob patterns
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### BackupJobs
Scheduled backup job definitions with encryption and compression support.

```sql
CREATE TABLE backup_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    source_id INTEGER NOT NULL REFERENCES backup_sources(id),
    pool_id INTEGER NOT NULL REFERENCES tape_pools(id),
    backup_type TEXT NOT NULL CHECK (backup_type IN ('full', 'incremental')),
    schedule_cron TEXT,
    retention_days INTEGER DEFAULT 30,
    enabled BOOLEAN DEFAULT 1,
    encryption_enabled BOOLEAN DEFAULT 0,
    encryption_key_id INTEGER REFERENCES encryption_keys(id),
    compression TEXT DEFAULT 'none',
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### BackupSets
Individual backup operations (a single run of a job).

```sql
CREATE TABLE backup_sets (
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
    encrypted BOOLEAN DEFAULT 0,
    encryption_key_id INTEGER REFERENCES encryption_keys(id),
    compressed BOOLEAN DEFAULT 0,
    compression_type TEXT DEFAULT 'none',
    format_type TEXT NOT NULL DEFAULT 'raw' CHECK (format_type IN ('raw', 'ltfs')),
    parent_set_id INTEGER REFERENCES backup_sets(id),  -- For incremental reference
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### CatalogEntries
File-level catalog for restore operations.

```sql
CREATE TABLE catalog_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_set_id INTEGER NOT NULL REFERENCES backup_sets(id),
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_mode INTEGER,
    mod_time DATETIME,
    checksum TEXT,
    block_offset INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

    -- Index for efficient file lookup
    UNIQUE(backup_set_id, file_path)
);

CREATE INDEX idx_catalog_path ON catalog_entries(file_path);
CREATE INDEX idx_catalog_backup_set ON catalog_entries(backup_set_id);
```

### JobExecutions
Tracks individual job execution instances for resume capability.

```sql
CREATE TABLE job_executions (
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
    resume_state TEXT,  -- JSON with position info for resume
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### AuditLogs
Audit trail for all operations.

```sql
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER REFERENCES users(id),
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id INTEGER,
    details TEXT,  -- JSON with operation details
    ip_address TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_created ON audit_logs(created_at);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, resource_id);
```

### Snapshots
Stores filesystem snapshots for incremental backup comparison.

```sql
CREATE TABLE snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL REFERENCES backup_sources(id),
    backup_set_id INTEGER REFERENCES backup_sets(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    file_count INTEGER DEFAULT 0,
    total_bytes INTEGER DEFAULT 0,
    snapshot_data BLOB  -- Compressed JSON or msgpack of file metadata
);

CREATE INDEX idx_snapshot_source ON snapshots(source_id, created_at DESC);
```

### EncryptionKeys
Stores encryption keys for backup encryption.

```sql
CREATE TABLE encryption_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    algorithm TEXT NOT NULL DEFAULT 'aes-256-gcm',
    key_data TEXT NOT NULL,  -- Base64 encoded encrypted key
    key_fingerprint TEXT NOT NULL,  -- SHA256 fingerprint for identification
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_encryption_keys_name ON encryption_keys(name);
CREATE INDEX idx_encryption_keys_fingerprint ON encryption_keys(key_fingerprint);
```

### DatabaseBackups
Tracks backups of the TapeBackarr database itself to tape.

```sql
CREATE TABLE database_backups (
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

CREATE INDEX idx_database_backups_tape ON database_backups(tape_id);
CREATE INDEX idx_database_backups_time ON database_backups(backup_time DESC);
```

### RestoreOperations
Tracks file restore operations from tape to destination.

```sql
CREATE TABLE restore_operations (
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

CREATE INDEX idx_restore_operations_status ON restore_operations(status);
CREATE INDEX idx_restore_operations_time ON restore_operations(created_at DESC);
```

### ApiKeys
API key authentication for programmatic access.

```sql
CREATE TABLE api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'readonly',
    last_used_at DATETIME,
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);
```

### ProxmoxNodes
Tracks Proxmox VE cluster and standalone nodes.

```sql
CREATE TABLE proxmox_nodes (
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

CREATE INDEX idx_proxmox_nodes_name ON proxmox_nodes(node_name);
CREATE INDEX idx_proxmox_nodes_status ON proxmox_nodes(status);
```

### ProxmoxGuests
Tracks Proxmox VMs and LXC containers.

```sql
CREATE TABLE proxmox_guests (
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

CREATE INDEX idx_proxmox_guests_node ON proxmox_guests(node_name);
CREATE INDEX idx_proxmox_guests_vmid ON proxmox_guests(vmid);
CREATE INDEX idx_proxmox_guests_type ON proxmox_guests(guest_type);
```

### ProxmoxBackups
Records of Proxmox guest backups to tape.

```sql
CREATE TABLE proxmox_backups (
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

CREATE INDEX idx_proxmox_backups_node ON proxmox_backups(node);
CREATE INDEX idx_proxmox_backups_vmid ON proxmox_backups(vmid);
CREATE INDEX idx_proxmox_backups_tape ON proxmox_backups(tape_id);
CREATE INDEX idx_proxmox_backups_status ON proxmox_backups(status);
CREATE INDEX idx_proxmox_backups_time ON proxmox_backups(start_time DESC);
CREATE INDEX idx_proxmox_backups_guest_type ON proxmox_backups(guest_type);
```

### ProxmoxRestores
Tracks Proxmox guest restore operations.

```sql
CREATE TABLE proxmox_restores (
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

CREATE INDEX idx_proxmox_restores_backup ON proxmox_restores(backup_id);
CREATE INDEX idx_proxmox_restores_status ON proxmox_restores(status);
CREATE INDEX idx_proxmox_restores_time ON proxmox_restores(start_time DESC);
```

### ProxmoxBackupJobs
Scheduled Proxmox backup job definitions.

```sql
CREATE TABLE proxmox_backup_jobs (
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
    schedule_cron TEXT,
    retention_days INTEGER DEFAULT 30,
    enabled BOOLEAN DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_proxmox_jobs_enabled ON proxmox_backup_jobs(enabled);
CREATE INDEX idx_proxmox_jobs_next_run ON proxmox_backup_jobs(next_run_at);
```

### ProxmoxJobExecutions
Tracks execution instances of Proxmox backup jobs.

```sql
CREATE TABLE proxmox_job_executions (
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

CREATE INDEX idx_proxmox_executions_job ON proxmox_job_executions(job_id);
CREATE INDEX idx_proxmox_executions_status ON proxmox_job_executions(status);
CREATE INDEX idx_proxmox_executions_time ON proxmox_job_executions(start_time DESC);
```

### TapeSpanningSets
Represents a backup that spans multiple tapes (defined in Go models).

```sql
CREATE TABLE tape_spanning_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL REFERENCES backup_jobs(id),
    total_tapes INTEGER DEFAULT 0,
    total_bytes INTEGER DEFAULT 0,
    total_files INTEGER DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed', 'failed')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### TapeSpanningMembers
Represents a single tape in a spanning set (defined in Go models).

```sql
CREATE TABLE tape_spanning_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    spanning_set_id INTEGER NOT NULL REFERENCES tape_spanning_sets(id),
    tape_id INTEGER NOT NULL REFERENCES tapes(id),
    backup_set_id INTEGER NOT NULL REFERENCES backup_sets(id),
    sequence_number INTEGER NOT NULL,
    bytes_written INTEGER DEFAULT 0,
    files_start_index INTEGER DEFAULT 0,
    files_end_index INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_spanning_members_set ON tape_spanning_members(spanning_set_id);
CREATE INDEX idx_spanning_members_tape ON tape_spanning_members(tape_id);
```

### TapeChangeRequests
Represents a pending tape change request during spanning operations (defined in Go models).

```sql
CREATE TABLE tape_change_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_execution_id INTEGER NOT NULL REFERENCES job_executions(id),
    spanning_set_id INTEGER REFERENCES tape_spanning_sets(id),
    current_tape_id INTEGER NOT NULL REFERENCES tapes(id),
    reason TEXT NOT NULL CHECK (reason IN ('tape_full', 'tape_error')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'acknowledged', 'completed', 'cancelled')),
    requested_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    acknowledged_at DATETIME,
    new_tape_id INTEGER REFERENCES tapes(id)
);
```

### TapeLibraries
Tape library (autochanger) support for automated tape handling via SCSI medium changers.

```sql
CREATE TABLE tape_libraries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    device_path TEXT NOT NULL,  -- SCSI generic device e.g. /dev/sg3
    vendor TEXT DEFAULT '',
    model TEXT DEFAULT '',
    serial_number TEXT DEFAULT '',
    num_slots INTEGER DEFAULT 0,
    num_drives INTEGER DEFAULT 0,
    num_import_export INTEGER DEFAULT 0,  -- mail slots / import-export elements
    barcode_reader BOOLEAN DEFAULT 0,
    enabled BOOLEAN DEFAULT 1,
    last_inventory_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### TapeLibrarySlots
Tracks the contents of each slot in a tape library.

```sql
CREATE TABLE tape_library_slots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES tape_libraries(id) ON DELETE CASCADE,
    slot_number INTEGER NOT NULL,
    slot_type TEXT NOT NULL DEFAULT 'storage' CHECK (slot_type IN ('storage', 'import_export', 'drive')),
    tape_id INTEGER REFERENCES tapes(id),
    barcode TEXT DEFAULT '',
    is_empty BOOLEAN DEFAULT 1,
    drive_id INTEGER REFERENCES tape_drives(id),  -- only for drive slots
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(library_id, slot_number)
);

CREATE INDEX idx_library_slots_library ON tape_library_slots(library_id);
CREATE INDEX idx_library_slots_tape ON tape_library_slots(tape_id);
```

**Note:** The `tape_drives` table also includes `library_id` and `library_drive_number` columns to link drives to their parent library.

### DriveStatistics
Tracks usage metrics and health indicators for tape drives.

```sql
CREATE TABLE drive_statistics (
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

CREATE INDEX idx_drive_statistics_drive ON drive_statistics(drive_id);
```

### DriveAlerts
Monitoring alerts for drive health and maintenance.

```sql
CREATE TABLE drive_alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    drive_id INTEGER NOT NULL REFERENCES tape_drives(id) ON DELETE CASCADE,
    severity TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'warning', 'critical')),
    category TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    resolved BOOLEAN DEFAULT 0,
    resolved_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_drive_alerts_drive ON drive_alerts(drive_id);
CREATE INDEX idx_drive_alerts_unresolved ON drive_alerts(drive_id, resolved);
```

## Key Relationships

1. **Tapes ↔ TapePools**: Many-to-one (tapes belong to pools)
2. **Tapes ↔ TapeDrives**: One-to-one optional (a drive may have a tape loaded)
3. **BackupSets ↔ Tapes**: Many-to-one (backup sets stored on tapes)
4. **BackupSets ↔ BackupJobs**: Many-to-one (jobs produce backup sets)
5. **BackupSets ↔ BackupSets**: Self-reference for incremental chains (parent_set_id)
6. **CatalogEntries ↔ BackupSets**: Many-to-one (files belong to backup sets)
7. **BackupJobs ↔ EncryptionKeys**: Many-to-one optional (jobs may use an encryption key)
8. **BackupSets ↔ EncryptionKeys**: Many-to-one optional (sets may reference an encryption key)
9. **DatabaseBackups ↔ Tapes**: Many-to-one (database backups stored on tapes)
10. **RestoreOperations ↔ BackupSets**: Many-to-one (restores reference a backup set)
11. **ProxmoxBackups ↔ Tapes**: Many-to-one (Proxmox backups stored on tapes)
12. **ProxmoxRestores ↔ ProxmoxBackups**: Many-to-one (restores reference a backup)
13. **ProxmoxJobExecutions ↔ ProxmoxBackupJobs**: Many-to-one (jobs produce executions)
14. **TapeSpanningSets ↔ BackupJobs**: Many-to-one (spanning sets reference a backup job)
15. **TapeSpanningMembers ↔ TapeSpanningSets**: Many-to-one (members belong to a spanning set)
16. **TapeSpanningMembers ↔ Tapes**: Many-to-one (each member references a tape)
17. **TapeSpanningMembers ↔ BackupSets**: Many-to-one (each member references a backup set)
18. **TapeChangeRequests ↔ JobExecutions**: Many-to-one (requests arise during executions)
19. **TapeLibraries ↔ TapeLibrarySlots**: One-to-many (libraries have slots)
20. **TapeLibrarySlots ↔ Tapes**: Many-to-one optional (slots may contain tapes)
21. **TapeDrives ↔ TapeLibraries**: Many-to-one optional (drives may belong to a library)
22. **DriveStatistics ↔ TapeDrives**: One-to-one (each drive has statistics)
23. **DriveAlerts ↔ TapeDrives**: One-to-many (drives can have alerts)

## Query Patterns

### Find all files on a specific tape
```sql
SELECT ce.file_path, ce.file_size, ce.mod_time, bs.start_time
FROM catalog_entries ce
JOIN backup_sets bs ON ce.backup_set_id = bs.id
WHERE bs.tape_id = ?
ORDER BY ce.block_offset;
```

### Find tapes needed to restore a file
```sql
SELECT DISTINCT t.barcode, t.label, t.status, bs.start_time
FROM catalog_entries ce
JOIN backup_sets bs ON ce.backup_set_id = bs.id
JOIN tapes t ON bs.tape_id = t.id
WHERE ce.file_path LIKE ?
ORDER BY bs.start_time DESC;
```

### Get tape utilization
```sql
SELECT 
    tp.name as pool,
    COUNT(t.id) as tape_count,
    SUM(t.used_bytes) as total_used,
    SUM(t.capacity_bytes) as total_capacity
FROM tapes t
JOIN tape_pools tp ON t.pool_id = tp.id
GROUP BY tp.id;
```
