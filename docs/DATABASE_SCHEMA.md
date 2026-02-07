# TapeBackarr Database Schema

## Entity Relationship Diagram

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│     Users       │     │   TapePools     │     │   TapeDrives    │
├─────────────────┤     ├─────────────────┤     ├─────────────────┤
│ id (PK)         │     │ id (PK)         │     │ id (PK)         │
│ username        │     │ name            │     │ device_path     │
│ password_hash   │     │ description     │     │ serial_number   │
│ role            │     │ retention_days  │     │ model           │
│ created_at      │     │ created_at      │     │ status          │
│ updated_at      │     │ updated_at      │     │ current_tape_id │
└─────────────────┘     └─────────────────┘     │ created_at      │
                               │                │ updated_at      │
                               │                └─────────────────┘
                               │                        │
                               ▼                        │
┌─────────────────┐     ┌─────────────────┐            │
│ BackupSources   │     │     Tapes       │◀───────────┘
├─────────────────┤     ├─────────────────┤
│ id (PK)         │     │ id (PK)         │
│ name            │     │ barcode         │
│ source_type     │     │ label           │
│ path            │     │ pool_id (FK)    │──────┐
│ include_patterns│     │ status          │      │
│ exclude_patterns│     │ capacity_bytes  │      │
│ enabled         │     │ used_bytes      │      │
│ created_at      │     │ write_count     │      │
│ updated_at      │     │ last_written_at │      │
└─────────────────┘     │ offsite_location│      │
        │               │ created_at      │      │
        │               │ updated_at      │      │
        │               └─────────────────┘      │
        │                       │                │
        │                       │                │
        ▼                       ▼                │
┌─────────────────┐     ┌─────────────────┐      │
│   BackupJobs    │     │   BackupSets    │◀─────┘
├─────────────────┤     ├─────────────────┤
│ id (PK)         │     │ id (PK)         │
│ name            │     │ job_id (FK)     │───────────┐
│ source_id (FK)  │──┐  │ tape_id (FK)    │           │
│ pool_id (FK)    │──┼──│ backup_type     │           │
│ backup_type     │  │  │ start_time      │           │
│ schedule_cron   │  │  │ end_time        │           │
│ retention_days  │  │  │ status          │           │
│ enabled         │  │  │ file_count      │           │
│ last_run_at     │  │  │ total_bytes     │           │
│ next_run_at     │  │  │ start_block     │           │
│ created_at      │  │  │ end_block       │           │
│ updated_at      │  │  │ checksum        │           │
└─────────────────┘  │  │ created_at      │           │
        │            │  │ updated_at      │           │
        │            │  └─────────────────┘           │
        │            │          │                     │
        ▼            │          │                     │
┌─────────────────┐  │          ▼                     │
│  JobExecutions  │  │  ┌─────────────────┐           │
├─────────────────┤  │  │ CatalogEntries  │           │
│ id (PK)         │  │  ├─────────────────┤           │
│ job_id (FK)     │──┘  │ id (PK)         │           │
│ backup_set_id   │     │ backup_set_id   │───────────┘
│ status          │     │ file_path       │
│ start_time      │     │ file_size       │
│ end_time        │     │ file_mode       │
│ files_processed │     │ mod_time        │
│ bytes_processed │     │ checksum        │
│ error_message   │     │ block_offset    │
│ can_resume      │     │ created_at      │
│ resume_state    │     └─────────────────┘
│ created_at      │
│ updated_at      │
└─────────────────┘

┌─────────────────┐     ┌─────────────────┐
│   AuditLogs     │     │    Snapshots    │
├─────────────────┤     ├─────────────────┤
│ id (PK)         │     │ id (PK)         │
│ user_id (FK)    │     │ source_id (FK)  │
│ action          │     │ created_at      │
│ resource_type   │     │ file_count      │
│ resource_id     │     │ total_bytes     │
│ details (JSON)  │     │ snapshot_data   │
│ ip_address      │     └─────────────────┘
│ created_at      │
└─────────────────┘
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
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Tapes
Individual tape media tracking.

```sql
CREATE TABLE tapes (
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
```

### TapeDrives
Physical tape drive tracking.

```sql
CREATE TABLE tape_drives (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_path TEXT NOT NULL UNIQUE,
    serial_number TEXT,
    model TEXT,
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
Scheduled backup job definitions.

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

## Key Relationships

1. **Tapes ↔ TapePools**: Many-to-one (tapes belong to pools)
2. **BackupSets ↔ Tapes**: Many-to-one (backup sets stored on tapes)
3. **BackupSets ↔ BackupJobs**: Many-to-one (jobs produce backup sets)
4. **CatalogEntries ↔ BackupSets**: Many-to-one (files belong to backup sets)
5. **BackupSets ↔ BackupSets**: Self-reference for incremental chains

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
