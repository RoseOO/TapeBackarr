# TapeBackarr API Reference

Complete REST API documentation for TapeBackarr.

## Base URL

```
http://your-server:8080/api/v1
```

## Authentication

All API endpoints (except `/auth/login`) require JWT authentication.

### Login

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "changeme"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin"
  }
}
```

### Using the Token

Include the JWT token in the Authorization header:

```http
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

---

## Dashboard

### Get Dashboard Stats

```http
GET /api/v1/dashboard
Authorization: Bearer <token>
```

**Response:**
```json
{
  "drives": [
    {
      "device_path": "/dev/nst0",
      "ready": true,
      "online": true,
      "current_tape": "WEEKLY-001"
    }
  ],
  "stats": {
    "total_tapes": 12,
    "active_tapes": 8,
    "total_backup_sets": 156,
    "total_bytes_written": 2500000000000,
    "active_jobs": 0
  },
  "recent_activity": [
    {
      "type": "backup",
      "job_name": "Daily-FileServer",
      "status": "completed",
      "timestamp": "2024-01-15T02:30:00Z"
    }
  ],
  "alerts": [
    {
      "level": "warning",
      "message": "Tape WEEKLY-002 is 90% full"
    }
  ]
}
```

---

## Tapes

### List Tapes

```http
GET /api/v1/tapes
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `pool_id` | int | Filter by pool ID |
| `status` | string | Filter by status (blank, active, full, expired, retired, exported) |
| `limit` | int | Number of results (default: 100) |
| `offset` | int | Pagination offset |

**Response:**
```json
{
  "tapes": [
    {
      "id": 1,
      "barcode": "ABC123",
      "label": "WEEKLY-001",
      "pool_id": 2,
      "pool_name": "WEEKLY",
      "status": "active",
      "capacity_bytes": 12000000000000,
      "used_bytes": 5000000000000,
      "write_count": 15,
      "last_written_at": "2024-01-15T02:30:00Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "total": 12
}
```

### Get Single Tape

```http
GET /api/v1/tapes/{id}
Authorization: Bearer <token>
```

**Response:**
```json
{
  "id": 1,
  "barcode": "ABC123",
  "label": "WEEKLY-001",
  "pool_id": 2,
  "pool_name": "WEEKLY",
  "status": "active",
  "capacity_bytes": 12000000000000,
  "used_bytes": 5000000000000,
  "write_count": 15,
  "last_written_at": "2024-01-15T02:30:00Z",
  "offsite_location": null,
  "notes": "Primary weekly backup tape",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-15T02:30:00Z"
}
```

### Create Tape

```http
POST /api/v1/tapes
Authorization: Bearer <token>
Content-Type: application/json

{
  "barcode": "ABC123",
  "label": "WEEKLY-001",
  "pool_id": 2,
  "capacity_bytes": 12000000000000,
  "notes": "New tape for weekly backups"
}
```

**Response:** Returns the created tape object.

### Update Tape

```http
PUT /api/v1/tapes/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "label": "WEEKLY-001-RENAMED",
  "pool_id": 3,
  "notes": "Updated notes"
}
```

### Format Tape

```http
POST /api/v1/tapes/{id}/format
Authorization: Bearer <token>
```

Formats the physical tape. Tape must be loaded in a drive.

### Export Tape

```http
POST /api/v1/tapes/{id}/export
Authorization: Bearer <token>
```

Marks the tape as exported (e.g., for offsite storage).

### Import Tape

```http
POST /api/v1/tapes/{id}/import
Authorization: Bearer <token>
```

Imports a previously exported tape back into the system.

### Read Tape Label

```http
GET /api/v1/tapes/{id}/read-label
Authorization: Bearer <token>
```

Reads the label block from the physical tape.

**Response:**
```json
{
  "label": "WEEKLY-001",
  "barcode": "ABC123",
  "written_at": "2024-01-15T02:30:00Z"
}
```

### Get LTO Types

```http
GET /api/v1/tapes/lto-types
Authorization: Bearer <token>
```

Returns the list of supported LTO tape types and their capacities.

**Response:**
```json
{
  "lto_types": [
    {
      "name": "LTO-8",
      "native_capacity_bytes": 12000000000000,
      "compressed_capacity_bytes": 30000000000000
    }
  ]
}
```

**Valid Tape Status Values:**
- `blank`
- `active`
- `full`
- `expired`
- `retired`
- `exported`

### Write Tape Label

```http
POST /api/v1/tapes/{id}/label
Authorization: Bearer <token>
```

Writes the label block to the physical tape. Tape must be loaded in drive.

### Delete Tape

```http
DELETE /api/v1/tapes/{id}
Authorization: Bearer <token>
```

**Note:** Cannot delete tapes that have associated backup sets.

### Batch Label Tapes

```http
POST /api/v1/tapes/batch-label
Authorization: Bearer <token>
Content-Type: application/json

{
  "drive_id": 1,
  "prefix": "WEEKLY-",
  "start_number": 1,
  "count": 10,
  "digits": 3,
  "pool_id": 2
}
```

Starts a batch tape labelling operation. Tapes are labelled sequentially (e.g., WEEKLY-001 through WEEKLY-010). The operator is expected to insert and eject tapes one at a time.

**Response:**
```json
{
  "status": "started",
  "message": "Batch labelling started: WEEKLY-001 through WEEKLY-010"
}
```

### Batch Label Status

```http
GET /api/v1/tapes/batch-label/status
Authorization: Bearer <token>
```

Returns the current status of a batch label operation.

**Response:**
```json
{
  "running": true,
  "progress": 3,
  "total": 10,
  "current": "WEEKLY-004",
  "message": "Labelling tape WEEKLY-004...",
  "completed": 3,
  "failed": 0
}
```

### Cancel Batch Label

```http
POST /api/v1/tapes/batch-label/cancel
Authorization: Bearer <token>
```

Cancels a running batch label operation.

### Batch Update Tapes

```http
POST /api/v1/tapes/batch-update
Authorization: Bearer <token>
Content-Type: application/json

{
  "tape_ids": [1, 2, 3],
  "status": "retired",
  "pool_id": 5
}
```

Updates the status and/or pool for multiple tapes at once. At least one of `status` or `pool_id` must be provided. Lifecycle safeguards are applied (e.g., exported tapes cannot be changed, active tapes cannot be set to blank).

**Response:**
```json
{
  "updated": 3,
  "skipped": 0
}
```

---

## Tape Pools

### List Pools

```http
GET /api/v1/pools
Authorization: Bearer <token>
```

**Response:**
```json
{
  "pools": [
    {
      "id": 1,
      "name": "DAILY",
      "description": "Daily backup tapes",
      "retention_days": 7,
      "tape_count": 5
    }
  ]
}
```

### Create Pool

```http
POST /api/v1/pools
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "QUARTERLY",
  "description": "Quarterly backup tapes",
  "retention_days": 180
}
```

### Get Pool

```http
GET /api/v1/pools/{id}
Authorization: Bearer <token>
```

### Update Pool

```http
PUT /api/v1/pools/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "QUARTERLY-RENAMED",
  "retention_days": 365
}
```

### Delete Pool

```http
DELETE /api/v1/pools/{id}
Authorization: Bearer <token>
```

---

## Backup Sources

### List Sources

```http
GET /api/v1/sources
Authorization: Bearer <token>
```

**Response:**
```json
{
  "sources": [
    {
      "id": 1,
      "name": "FileServer-Home",
      "source_type": "nfs",
      "path": "/mnt/nfs/home",
      "include_patterns": ["*.doc", "*.pdf"],
      "exclude_patterns": ["*.tmp", "cache/*"],
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Create Source

```http
POST /api/v1/sources
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "FileServer-Home",
  "source_type": "nfs",
  "path": "/mnt/nfs/home",
  "include_patterns": ["*.doc", "*.pdf", "*.xlsx"],
  "exclude_patterns": ["*.tmp", "*.log", "cache/*"]
}
```

### Get Source

```http
GET /api/v1/sources/{id}
Authorization: Bearer <token>
```

### Update Source

```http
PUT /api/v1/sources/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "FileServer-Home-Updated",
  "exclude_patterns": ["*.tmp", "*.log", "cache/*", "node_modules/*"]
}
```

### Delete Source

```http
DELETE /api/v1/sources/{id}
Authorization: Bearer <token>
```

---

## Backup Jobs

### List Jobs

```http
GET /api/v1/jobs
Authorization: Bearer <token>
```

**Response:**
```json
{
  "jobs": [
    {
      "id": 1,
      "name": "Daily-FileServer",
      "source_id": 1,
      "source_name": "FileServer-Home",
      "pool_id": 1,
      "pool_name": "DAILY",
      "backup_type": "incremental",
      "schedule": "0 2 * * *",
      "enabled": true,
      "last_run_at": "2024-01-15T02:00:00Z",
      "next_run_at": "2024-01-16T02:00:00Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Create Job

```http
POST /api/v1/jobs
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Daily-FileServer",
  "source_id": 1,
  "pool_id": 1,
  "backup_type": "incremental",
  "schedule": "0 2 * * *",
  "enabled": true
}
```

### Get Job

```http
GET /api/v1/jobs/{id}
Authorization: Bearer <token>
```

### Update Job

```http
PUT /api/v1/jobs/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "schedule": "0 3 * * *",
  "enabled": true
}
```

### Run Job Manually

```http
POST /api/v1/jobs/{id}/run
Authorization: Bearer <token>
Content-Type: application/json

{
  "backup_type": "full"  // Optional, overrides job default
}
```

**Response:**
```json
{
  "backup_set_id": 157,
  "status": "running",
  "message": "Backup job started"
}
```

### Get Active Jobs

```http
GET /api/v1/jobs/active
Authorization: Bearer <token>
```

Returns a list of currently running or queued jobs.

### Get Resumable Jobs

```http
GET /api/v1/jobs/resumable
Authorization: Bearer <token>
```

Returns a list of paused or failed job executions that can be resumed.

**Response:**
```json
[
  {
    "id": 10,
    "job_id": 1,
    "job_name": "Daily-FileServer",
    "status": "paused",
    "files_processed": 500,
    "bytes_processed": 1000000000,
    "error_message": "",
    "can_resume": true,
    "created_at": "2024-01-15T02:00:00Z",
    "updated_at": "2024-01-15T02:15:00Z"
  }
]
```

### Cancel Job

```http
POST /api/v1/jobs/{id}/cancel
Authorization: Bearer <token>
```

Cancels a running or queued job.

### Pause Job

```http
POST /api/v1/jobs/{id}/pause
Authorization: Bearer <token>
```

Pauses a running job.

### Resume Job

```http
POST /api/v1/jobs/{id}/resume
Authorization: Bearer <token>
```

Resumes a paused job.

### Retry Job

```http
POST /api/v1/jobs/{id}/retry
Authorization: Bearer <token>
Content-Type: application/json

{
  "tape_id": 3,
  "use_pool": true,
  "from_scratch": false
}
```

Retries a failed or paused backup job. If `from_scratch` is false (default), the job will attempt to resume from where it left off using saved resume state. If `use_pool` is true or `tape_id` is 0, a tape will be selected from the job's pool.

**Response:**
```json
{
  "backup_set_id": 158,
  "status": "running",
  "message": "Backup job retried",
  "tape_label": "WEEKLY-003",
  "resume": true
}
```

### Recommend Tape for Job

```http
GET /api/v1/jobs/{id}/recommend-tape
Authorization: Bearer <token>
```

Recommends the best tape to use for the next run of this job.

**Response:**
```json
{
  "tape_id": 3,
  "tape_label": "WEEKLY-003",
  "reason": "Most available capacity in pool"
}
```

### Delete Job

```http
DELETE /api/v1/jobs/{id}
Authorization: Bearer <token>
```

---

## Backup Sets

### List Backup Sets

```http
GET /api/v1/backup-sets
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `job_id` | int | Filter by job ID |
| `tape_id` | int | Filter by tape ID |
| `status` | string | Filter by status |
| `limit` | int | Number of results |

**Response:**
```json
{
  "backup_sets": [
    {
      "id": 157,
      "job_id": 1,
      "job_name": "Daily-FileServer",
      "tape_id": 1,
      "tape_label": "WEEKLY-001",
      "backup_type": "incremental",
      "start_time": "2024-01-15T02:00:00Z",
      "end_time": "2024-01-15T02:30:00Z",
      "status": "completed",
      "file_count": 1500,
      "total_bytes": 5000000000
    }
  ]
}
```

### Get Backup Set Details

```http
GET /api/v1/backup-sets/{id}
Authorization: Bearer <token>
```

Returns detailed information including file list and spanning info.

### List Backup Set Files

```http
GET /api/v1/backup-sets/{id}/files
Authorization: Bearer <token>
```

Returns the list of files included in the backup set.

**Response:**
```json
{
  "files": [
    {
      "path": "/documents/report.pdf",
      "size": 1048576,
      "mod_time": "2024-01-14T10:30:00Z"
    }
  ],
  "total": 1500
}
```

### Delete Backup Set

```http
DELETE /api/v1/backup-sets/{id}
Authorization: Bearer <token>
```

Deletes a backup set and its associated catalog entries. Only backup sets with status `failed` or `completed` can be deleted.

**Response:**
```json
{
  "status": "deleted"
}
```

---

## Catalog

### Search Catalog

```http
GET /api/v1/catalog/search
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `q` | string | Search pattern (supports wildcards) |
| `limit` | int | Max results (default: 100) |

**Examples:**
- `/catalog/search?q=report.pdf`
- `/catalog/search?q=*.xlsx`
- `/catalog/search?q=/documents/*`

**Response:**
```json
{
  "entries": [
    {
      "id": 12345,
      "backup_set_id": 157,
      "file_path": "/documents/report.pdf",
      "file_size": 1048576,
      "mod_time": "2024-01-14T10:30:00Z",
      "checksum": "sha256:abc123...",
      "tape_id": 1,
      "tape_label": "WEEKLY-001",
      "block_offset": 50000
    }
  ],
  "total": 1
}
```

### Browse Catalog

```http
GET /api/v1/catalog/browse/{backupSetId}
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `prefix` | string | Directory path prefix to browse |

---

## Restore

### Plan Restore

```http
POST /api/v1/restore/plan
Authorization: Bearer <token>
Content-Type: application/json

{
  "file_paths": [
    "/documents/report.pdf",
    "/documents/spreadsheet.xlsx"
  ]
}
```

**Response:**
```json
{
  "restore_plan": {
    "files": [
      {
        "path": "/documents/report.pdf",
        "size": 1048576,
        "tape_id": 1,
        "tape_label": "WEEKLY-001"
      }
    ],
    "required_tapes": [
      {
        "id": 1,
        "label": "WEEKLY-001",
        "status": "active",
        "insertion_order": 1
      }
    ],
    "total_size": 2097152
  }
}
```

### Execute Restore

```http
POST /api/v1/restore/run
Authorization: Bearer <token>
Content-Type: application/json

{
  "backup_set_id": 157,
  "file_paths": [
    "/documents/report.pdf"
  ],
  "dest_path": "/restore/output",
  "destination_type": "local",
  "overwrite": false,
  "verify": true
}
```

**Destination Types:**
- `local` - Local filesystem path
- `smb` - SMB/CIFS network share (must be pre-mounted)
- `nfs` - NFS network share (must be pre-mounted)

**Response:**
```json
{
  "restore_id": 42,
  "status": "running",
  "message": "Restore started. Please insert tape WEEKLY-001."
}
```

---

## Tape Drives

### List Drives

```http
GET /api/v1/drives
Authorization: Bearer <token>
```

**Response:**
```json
{
  "drives": [
    {
      "id": 1,
      "device_path": "/dev/nst0",
      "display_name": "Primary LTO Drive",
      "serial_number": "ABC123",
      "model": "LTO-8",
      "status": "ready",
      "current_tape_id": 1,
      "enabled": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Create Drive

```http
POST /api/v1/drives
Authorization: Bearer <token>
Content-Type: application/json

{
  "device_path": "/dev/nst1",
  "display_name": "Secondary LTO Drive",
  "serial_number": "DEF456",
  "model": "LTO-6"
}
```

### Update Drive

```http
PUT /api/v1/drives/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "display_name": "Updated Drive Name",
  "enabled": true
}
```

### Delete Drive

```http
DELETE /api/v1/drives/{id}
Authorization: Bearer <token>
```

### Select Drive

Select which drive to use for operations.

```http
POST /api/v1/drives/{id}/select
Authorization: Bearer <token>
```

**Response:**
```json
{
  "status": "selected",
  "drive_id": 1,
  "device_path": "/dev/nst0"
}
```

### Get Drive Status

```http
GET /api/v1/drives/{id}/status
Authorization: Bearer <token>
```

**Response:**
```json
{
  "device_path": "/dev/nst0",
  "ready": true,
  "online": true,
  "write_protect": false,
  "bot": true,
  "eot": false,
  "file_number": 0,
  "block_number": 0,
  "block_size": 65536
}
```

### Eject Tape

```http
POST /api/v1/drives/{id}/eject
Authorization: Bearer <token>
```

### Rewind Tape

```http
POST /api/v1/drives/{id}/rewind
Authorization: Bearer <token>
```

### Scan for Drives

```http
GET /api/v1/drives/scan
Authorization: Bearer <token>
```

Scans the system for available tape drives.

**Response:**
```json
{
  "drives": [
    {
      "device_path": "/dev/nst0",
      "model": "LTO-8",
      "serial_number": "ABC123"
    }
  ]
}
```

### Detect Tape in Drive

```http
GET /api/v1/drives/{id}/detect-tape
Authorization: Bearer <token>
```

Detects whether a tape is loaded and reads its information.

### Format Tape in Drive

```http
POST /api/v1/drives/{id}/format-tape
Authorization: Bearer <token>
```

Formats the tape currently loaded in the drive.

### Inspect Tape in Drive

```http
GET /api/v1/drives/{id}/inspect-tape
Authorization: Bearer <token>
```

Reads and returns metadata about the tape currently loaded in the drive.

### Scan for Database Backup on Tape

```http
GET /api/v1/drives/{id}/scan-for-db-backup
Authorization: Bearer <token>
```

Scans the tape in the drive for TapeBackarr database backup records.

### Batch Label Tapes

```http
POST /api/v1/drives/{id}/batch-label
Authorization: Bearer <token>
Content-Type: application/json

{
  "tape_ids": [1, 2, 3]
}
```

Labels multiple tapes sequentially using the specified drive.

### Get Drive Statistics

```http
GET /api/v1/drives/{id}/statistics
Authorization: Bearer <token>
```

Returns usage statistics for the drive, including bytes read/written, error counts, load counts, power-on hours, and temperature.

**Response:**
```json
{
  "drive_id": 1,
  "total_bytes_read": 5000000000000,
  "total_bytes_written": 8000000000000,
  "read_errors": 0,
  "write_errors": 2,
  "total_load_count": 150,
  "cleaning_required": false,
  "power_on_hours": 12000,
  "tape_motion_hours": 8500.5,
  "temperature_c": 35,
  "read_compression_pct": 2.1,
  "write_compression_pct": 2.3
}
```

### Get Drive Alerts

```http
GET /api/v1/drives/{id}/alerts
Authorization: Bearer <token>
```

Returns recent alerts for the drive (up to 50, newest first).

**Response:**
```json
[
  {
    "id": 1,
    "drive_id": 1,
    "severity": "warning",
    "category": "cleaning",
    "message": "Drive cleaning recommended",
    "resolved": false,
    "resolved_at": null,
    "created_at": "2024-01-15T10:00:00Z"
  }
]
```

### Clean Drive

```http
POST /api/v1/drives/{id}/clean
Authorization: Bearer <token>
```

Initiates a drive cleaning cycle. A cleaning tape should be loaded in the drive. Resolves any pending cleaning-related alerts.

**Response:**
```json
{
  "status": "cleaned"
}
```

### Retension Tape

```http
POST /api/v1/drives/{id}/retension
Authorization: Bearer <token>
```

Runs a tape retension pass on the tape currently loaded in the drive. This can help with read errors caused by loose tape packing.

**Response:**
```json
{
  "status": "retensioned"
}
```

---

## Database Backup

Backup and restore the TapeBackarr database itself to tape.

### List Database Backups

```http
GET /api/v1/database-backup
Authorization: Bearer <token>
```

**Response:**
```json
{
  "backups": [
    {
      "id": 1,
      "tape_id": 1,
      "tape_label": "ARCHIVE-001",
      "backup_time": "2024-01-15T03:00:00Z",
      "file_size": 5242880,
      "checksum": "sha256:abc123...",
      "status": "completed",
      "created_at": "2024-01-15T03:00:00Z"
    }
  ]
}
```

### Backup Database to Tape

```http
POST /api/v1/database-backup/backup
Authorization: Bearer <token>
Content-Type: application/json

{
  "tape_id": 1
}
```

**Response:**
```json
{
  "id": 2,
  "status": "started",
  "message": "Database backup started"
}
```

### Restore Database from Tape

```http
POST /api/v1/database-backup/restore
Authorization: Bearer <token>
Content-Type: application/json

{
  "backup_id": 1,
  "dest_path": "/tmp/restore"
}
```

**Response:**
```json
{
  "status": "restored",
  "dest_path": "/tmp/restore/tapebackarr.db"
}
```

---

## Documentation

Access documentation from the API.

### List Available Documents

```http
GET /api/v1/docs
Authorization: Bearer <token>
```

**Response:**
```json
{
  "docs": [
    {
      "id": "usage",
      "title": "Usage Guide",
      "description": "Complete guide to using TapeBackarr"
    },
    {
      "id": "api",
      "title": "API Reference",
      "description": "REST API documentation"
    },
    {
      "id": "operator",
      "title": "Operator Guide",
      "description": "Quick reference for operators"
    },
    {
      "id": "recovery",
      "title": "Manual Recovery",
      "description": "Recover data without TapeBackarr"
    },
    {
      "id": "architecture",
      "title": "Architecture",
      "description": "System design and data flows"
    },
    {
      "id": "database",
      "title": "Database Schema",
      "description": "Database table definitions"
    },
    {
      "id": "installation",
      "title": "Installation Guide",
      "description": "Installation instructions for all deployment methods"
    },
    {
      "id": "proxmox",
      "title": "Proxmox Guide",
      "description": "Backup and restore Proxmox VMs and LXCs"
    }
  ]
}
```

### Get Document Content

```http
GET /api/v1/docs/{id}
Authorization: Bearer <token>
```

**Response:**
```json
{
  "id": "usage",
  "title": "USAGE_GUIDE.md",
  "content": "# TapeBackarr Usage Guide\n\n..."
}
```

---

## Logs

### Get Audit Logs

```http
GET /api/v1/logs/audit
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `start_date` | string | ISO 8601 date |
| `end_date` | string | ISO 8601 date |
| `user_id` | int | Filter by user |
| `resource_type` | string | Filter by resource (tape, job, etc.) |
| `limit` | int | Max results |

**Response:**
```json
{
  "logs": [
    {
      "id": 1000,
      "timestamp": "2024-01-15T10:30:00Z",
      "user_id": 1,
      "username": "admin",
      "action": "backup.run",
      "resource_type": "job",
      "resource_id": 1,
      "details": {"backup_type": "full"},
      "ip_address": "192.168.1.100"
    }
  ],
  "total": 1000
}
```

### Export Logs

```http
GET /api/v1/logs/export
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `format` | string | `json` or `csv` |
| `start_date` | string | ISO 8601 date |
| `end_date` | string | ISO 8601 date |

Returns downloadable file.

---

## Users (Admin Only)

### List Users

```http
GET /api/v1/users
Authorization: Bearer <token>
```

**Response:**
```json
{
  "users": [
    {
      "id": 1,
      "username": "admin",
      "role": "admin",
      "created_at": "2024-01-01T00:00:00Z",
      "last_login_at": "2024-01-15T08:00:00Z"
    }
  ]
}
```

### Create User

```http
POST /api/v1/users
Authorization: Bearer <token>
Content-Type: application/json

{
  "username": "operator1",
  "password": "securepassword",
  "role": "operator"
}
```

### Delete User

```http
DELETE /api/v1/users/{id}
Authorization: Bearer <token>
```

### Change Password

```http
POST /api/v1/auth/change-password
Authorization: Bearer <token>
Content-Type: application/json

{
  "current_password": "oldpassword",
  "new_password": "newpassword"
}
```

---

## Settings

### Get Settings

```http
GET /api/v1/settings
Authorization: Bearer <token>
```

**Response:**
```json
{
  "telegram_bot_token": "***",
  "telegram_chat_id": "123456",
  "notifications_enabled": true
}
```

### Update Settings (Admin Only)

```http
PUT /api/v1/settings
Authorization: Bearer <token>
Content-Type: application/json

{
  "telegram_bot_token": "bot123:ABC...",
  "telegram_chat_id": "123456",
  "notifications_enabled": true
}
```

### Test Telegram Notification (Admin Only)

```http
POST /api/v1/settings/telegram/test
Authorization: Bearer <token>
```

Sends a test notification to verify Telegram configuration.

**Response:**
```json
{
  "success": true,
  "message": "Test notification sent successfully"
}
```

### Restart Application (Admin Only)

```http
POST /api/v1/settings/restart
Authorization: Bearer <token>
```

Restarts the TapeBackarr application.

---

## Events

### Event Stream (SSE)

```http
GET /api/v1/events/stream
Authorization: Bearer <token>
```

Server-Sent Events stream for real-time updates (job progress, tape status changes, etc.).

### Get Notifications

```http
GET /api/v1/events
Authorization: Bearer <token>
```

Returns recent event notifications.

**Response:**
```json
{
  "events": [
    {
      "id": 1,
      "type": "job.completed",
      "message": "Backup job Daily-FileServer completed",
      "timestamp": "2024-01-15T02:30:00Z"
    }
  ]
}
```

---

## Encryption Keys

### List Encryption Keys

```http
GET /api/v1/encryption-keys
Authorization: Bearer <token>
```

**Response:**
```json
{
  "keys": [
    {
      "id": 1,
      "name": "default",
      "fingerprint": "SHA256:abc123...",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Get Key Sheet

```http
GET /api/v1/encryption-keys/keysheet
Authorization: Bearer <token>
```

Returns an HTML key sheet for printing and secure offline storage.

### Get Key Sheet (Text)

```http
GET /api/v1/encryption-keys/keysheet/text
Authorization: Bearer <token>
```

Returns a plain-text key sheet.

### Create Encryption Key (Admin Only)

```http
POST /api/v1/encryption-keys
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "my-key"
}
```

### Import Encryption Key (Admin Only)

```http
POST /api/v1/encryption-keys/import
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "imported-key",
  "key_data": "base64-encoded-key-material"
}
```

### Delete Encryption Key (Admin Only)

```http
DELETE /api/v1/encryption-keys/{id}
Authorization: Bearer <token>
```

---

## API Keys (Admin Only)

### List API Keys

```http
GET /api/v1/api-keys
Authorization: Bearer <token>
```

**Response:**
```json
{
  "api_keys": [
    {
      "id": 1,
      "name": "monitoring",
      "prefix": "tb_abc1...",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Create API Key

```http
POST /api/v1/api-keys
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "monitoring"
}
```

**Response:**
```json
{
  "id": 2,
  "name": "monitoring",
  "key": "tb_full-api-key-shown-only-once"
}
```

### Delete API Key

```http
DELETE /api/v1/api-keys/{id}
Authorization: Bearer <token>
```

---

## Tape Libraries (Autochangers)

Manage tape libraries (autochangers) for automated tape handling.

### List Libraries

```http
GET /api/v1/libraries
Authorization: Bearer <token>
```

**Response:**
```json
[
  {
    "id": 1,
    "name": "Primary Autochanger",
    "device_path": "/dev/sg3",
    "vendor": "IBM",
    "model": "TS3200",
    "serial_number": "ABC123",
    "num_slots": 24,
    "num_drives": 2,
    "num_import_export": 4,
    "barcode_reader": true,
    "enabled": true,
    "last_inventory_at": "2024-01-15T10:00:00Z",
    "created_at": "2024-01-01T00:00:00Z"
  }
]
```

### Create Library

```http
POST /api/v1/libraries
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Primary Autochanger",
  "device_path": "/dev/sg3",
  "vendor": "IBM",
  "model": "TS3200",
  "serial_number": "ABC123"
}
```

### Scan for Libraries

```http
GET /api/v1/libraries/scan
Authorization: Bearer <token>
```

Scans the system for SCSI medium changer devices using `lsscsi`.

**Response:**
```json
[
  {
    "device_path": "/dev/sg3",
    "type": "medium_changer",
    "vendor": "IBM",
    "model": "TS3200"
  }
]
```

### Get Library

```http
GET /api/v1/libraries/{id}
Authorization: Bearer <token>
```

### Update Library

```http
PUT /api/v1/libraries/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Updated Name",
  "enabled": true
}
```

### Delete Library

```http
DELETE /api/v1/libraries/{id}
Authorization: Bearer <token>
```

Also cleans up associated slot records and unlinks drives.

### Run Inventory

```http
POST /api/v1/libraries/{id}/inventory
Authorization: Bearer <token>
```

Runs `mtx status` to inventory the library. Discovers all slots, barcodes, and which drives are loaded. Updates the slot database accordingly.

### List Library Slots

```http
GET /api/v1/libraries/{id}/slots
Authorization: Bearer <token>
```

**Response:**
```json
{
  "slots": [
    {
      "id": 1,
      "library_id": 1,
      "slot_number": 1,
      "slot_type": "storage",
      "tape_id": 5,
      "barcode": "WEEKLY-001",
      "is_empty": false
    },
    {
      "id": 2,
      "library_id": 1,
      "slot_number": 2,
      "slot_type": "drive",
      "tape_id": null,
      "barcode": "",
      "is_empty": true,
      "drive_id": 1
    }
  ]
}
```

### Load Tape

```http
POST /api/v1/libraries/{id}/load
Authorization: Bearer <token>
Content-Type: application/json

{
  "slot_number": 5,
  "drive_number": 0
}
```

Loads a tape from the specified slot into the specified drive using `mtx load`.

### Unload Tape

```http
POST /api/v1/libraries/{id}/unload
Authorization: Bearer <token>
Content-Type: application/json

{
  "slot_number": 5,
  "drive_number": 0
}
```

Unloads a tape from the specified drive back to the specified slot using `mtx unload`.

### Transfer Tape

```http
POST /api/v1/libraries/{id}/transfer
Authorization: Bearer <token>
Content-Type: application/json

{
  "source_slot": 5,
  "dest_slot": 10
}
```

Transfers a tape between two slots using `mtx transfer`.

---

## Health Check

### Simple Health Check

```http
GET /health
```

No authentication required.

**Response:**
```json
{
  "status": "ok"
}
```

### Detailed Health Check

```http
GET /api/v1/health
```

No authentication required. Returns detailed component status.

**Response:**
```json
{
  "status": "ok",
  "version": "1.0.0",
  "database": "ok",
  "uptime": "72h15m"
}
```

---

## Proxmox

### List Nodes

```http
GET /api/v1/proxmox/nodes
Authorization: Bearer <token>
```

### List Guests

```http
GET /api/v1/proxmox/guests
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `node` | string | Filter by Proxmox node |
| `type` | string | Filter by guest type (qemu, lxc) |

### Get Guest

```http
GET /api/v1/proxmox/guests/{vmid}
Authorization: Bearer <token>
```

### Get Guest Config

```http
GET /api/v1/proxmox/guests/{vmid}/config
Authorization: Bearer <token>
```

### Get Cluster Status

```http
GET /api/v1/proxmox/cluster/status
Authorization: Bearer <token>
```

### List Proxmox Backups

```http
GET /api/v1/proxmox/backups
Authorization: Bearer <token>
```

### Get Proxmox Backup

```http
GET /api/v1/proxmox/backups/{id}
Authorization: Bearer <token>
```

### Create Proxmox Backup

```http
POST /api/v1/proxmox/backups
Authorization: Bearer <token>
Content-Type: application/json

{
  "vmid": 100,
  "node": "pve1"
}
```

### Backup All Guests

```http
POST /api/v1/proxmox/backups/all
Authorization: Bearer <token>
```

### List Proxmox Restores

```http
GET /api/v1/proxmox/restores
Authorization: Bearer <token>
```

### Create Proxmox Restore

```http
POST /api/v1/proxmox/restores
Authorization: Bearer <token>
Content-Type: application/json

{
  "backup_id": 1,
  "target_node": "pve1"
}
```

### Plan Proxmox Restore

```http
POST /api/v1/proxmox/restores/plan
Authorization: Bearer <token>
Content-Type: application/json

{
  "backup_id": 1
}
```

### List Proxmox Jobs

```http
GET /api/v1/proxmox/jobs
Authorization: Bearer <token>
```

### Create Proxmox Job

```http
POST /api/v1/proxmox/jobs
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "nightly-proxmox-backup",
  "schedule": "0 3 * * *",
  "vmids": [100, 101, 102]
}
```

### Get Proxmox Job

```http
GET /api/v1/proxmox/jobs/{id}
Authorization: Bearer <token>
```

### Update Proxmox Job

```http
PUT /api/v1/proxmox/jobs/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "schedule": "0 4 * * *",
  "enabled": true
}
```

### Delete Proxmox Job

```http
DELETE /api/v1/proxmox/jobs/{id}
Authorization: Bearer <token>
```

### Run Proxmox Job

```http
POST /api/v1/proxmox/jobs/{id}/run
Authorization: Bearer <token>
```

---

## Error Responses

All endpoints return errors in this format:

```json
{
  "error": {
    "code": "TAPE_NOT_FOUND",
    "message": "Tape with ID 999 not found"
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Invalid or missing token |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `VALIDATION_ERROR` | 400 | Invalid request data |
| `TAPE_IN_USE` | 409 | Tape is currently in use |
| `DRIVE_BUSY` | 409 | Drive is performing operation |
| `NO_TAPE_LOADED` | 400 | No tape in drive |
| `TAPE_WRITE_PROTECTED` | 400 | Tape is write-protected |
| `INTERNAL_ERROR` | 500 | Server error |

---

## Rate Limiting

The API has the following rate limits:

| Endpoint Type | Limit |
|--------------|-------|
| Authentication | 10 requests/minute |
| Read operations | 100 requests/minute |
| Write operations | 30 requests/minute |
| Backup/Restore | 5 concurrent operations |

Rate limit headers are included in responses:

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1705312800
```

---

## Webhooks (Coming Soon)

Future versions will support webhooks for external integrations.
