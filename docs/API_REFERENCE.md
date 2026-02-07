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
| `status` | string | Filter by status (blank, active, full, retired, offsite) |
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

### Update Tape Status

```http
PUT /api/v1/tapes/{id}/status
Authorization: Bearer <token>
Content-Type: application/json

{
  "status": "offsite",
  "offsite_location": "Iron Mountain Box 42"
}
```

**Valid Status Values:**
- `blank`
- `active`
- `full`
- `retired`
- `offsite`

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
GET /api/v1/catalog/browse
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Directory path to browse |
| `backup_set_id` | int | Specific backup set |

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
  "file_paths": [
    "/documents/report.pdf"
  ],
  "destination": "/restore/output",
  "overwrite": false,
  "verify": true
}
```

**Response:**
```json
{
  "restore_id": 42,
  "status": "running",
  "message": "Restore started. Please insert tape WEEKLY-001."
}
```

### Acknowledge Tape Change

```http
POST /api/v1/restore/{id}/acknowledge-tape
Authorization: Bearer <token>
Content-Type: application/json

{
  "tape_label": "WEEKLY-002"
}
```

### Get Restore Status

```http
GET /api/v1/restore/{id}/status
Authorization: Bearer <token>
```

**Response:**
```json
{
  "restore_id": 42,
  "status": "waiting_for_tape",
  "current_tape": "WEEKLY-001",
  "next_tape": "WEEKLY-002",
  "files_restored": 5,
  "files_total": 10,
  "bytes_restored": 5000000
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
      "name": "LTO-8 Drive",
      "device_path": "/dev/nst0",
      "status": "online",
      "current_tape_id": 1,
      "current_tape_label": "WEEKLY-001"
    }
  ]
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
GET /api/v1/logs/audit/export
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

### Update User Role

```http
PUT /api/v1/users/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "role": "admin"
}
```

### Delete User

```http
DELETE /api/v1/users/{id}
Authorization: Bearer <token>
```

### Change Password

```http
PUT /api/v1/users/{id}/password
Authorization: Bearer <token>
Content-Type: application/json

{
  "current_password": "oldpassword",
  "new_password": "newpassword"
}
```

---

## Notifications

### Test Telegram

```http
POST /api/v1/notifications/telegram/test
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
