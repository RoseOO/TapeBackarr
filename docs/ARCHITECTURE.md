# TapeBackarr System Architecture

## High-Level Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              TapeBackarr System                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                           Web UI (SvelteKit)                             │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │ │
│  │  │Dashboard │ │  Tapes   │ │  Jobs    │ │ Restore  │ │   Logs   │       │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘       │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                          │
│                                    │ REST API                                 │
│                                    ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                          API Gateway (Go/Chi)                            │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐                     │ │
│  │  │   Auth   │ │   RBAC   │ │ Rate     │ │  CORS    │                     │ │
│  │  │ Middleware│ │Middleware│ │ Limiting │ │ Handler  │                     │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘                     │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                          │
│              ┌─────────────────────┼─────────────────────┐                   │
│              ▼                     ▼                     ▼                   │
│  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐          │
│  │  Backup Service   │ │  Restore Service  │ │   Tape Service    │          │
│  │                   │ │                   │ │                   │          │
│  │ - Job Management  │ │ - Catalog Browse  │ │ - Device Control  │          │
│  │ - Streaming       │ │ - File Selection  │ │ - Status Monitor  │          │
│  │ - Incremental     │ │ - Multi-tape      │ │ - Pool Management │          │
│  │ - Catalog Update  │ │ - Verification    │ │ - Labeling        │          │
│  └───────────────────┘ └───────────────────┘ └───────────────────┘          │
│              │                     │                     │                   │
│              └─────────────────────┼─────────────────────┘                   │
│                                    ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                        Tape I/O Layer (Go)                               │ │
│  │                                                                           │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                    │ │
│  │  │ mt commands  │  │ sg_* utils   │  │ tar streamer │                    │ │
│  │  │ (tape ctl)   │  │ (SCSI)       │  │ (data write) │                    │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘                    │ │
│  │                           │                                               │ │
│  │                           ▼                                               │ │
│  │                    /dev/st0, /dev/nst0                                    │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                          │
│                                    │                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                      Data Layer (SQLite)                                 │ │
│  │                                                                           │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │ │
│  │  │  Tapes   │ │ Backups  │ │ Catalog  │ │   Jobs   │ │   Logs   │       │ │
│  │  │          │ │  Sets    │ │ (Files)  │ │          │ │  Audit   │       │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘       │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                     External Data Sources                                 │ │
│  │                                                                           │ │
│  │   ┌──────────────┐    ┌──────────────┐    ┌──────────────┐               │ │
│  │   │  SMB Shares  │    │  NFS Mounts  │    │  Local FS    │               │ │
│  │   │  /mnt/smb/*  │    │  /mnt/nfs/*  │    │  /backup/*   │               │ │
│  │   └──────────────┘    └──────────────┘    └──────────────┘               │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow

### Backup Flow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Source Path  │     │ File Scanner │     │ tar Pipeline │     │ Tape Device  │
│ (SMB/NFS/FS) │────▶│ + Filter     │────▶│ + mbuffer    │────▶│ /dev/st0     │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
                            │                     │                     │
                            ▼                     ▼                     ▼
                     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
                     │ Catalog DB   │     │ Block Offset │     │ Tape Header  │
                     │ (file list)  │     │ Tracking     │     │ + Label      │
                     └──────────────┘     └──────────────┘     └──────────────┘
```

### Restore Flow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Catalog      │     │ Tape         │     │ tar Extract  │     │ Destination  │
│ Selection    │────▶│ Positioning  │────▶│ Pipeline     │────▶│ Path         │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
       │                     │                     │
       ▼                     ▼                     ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Required     │     │ Operator     │     │ Verification │
│ Tape List    │     │ Guidance     │     │ (checksum)   │
└──────────────┘     └──────────────┘     └──────────────┘
```

## Component Details

### 1. Web UI Layer
- **Technology**: SvelteKit (server-rendered for reliability)
- **Features**: Dashboard, Tape Management, Backup Jobs, Restore Wizard, Logs
- **Authentication**: Session-based with JWT tokens

### 2. API Gateway
- **Technology**: Go with Chi router
- **Features**: RESTful endpoints, JWT auth, RBAC middleware
- **Endpoints**: /api/v1/* for all operations

### 3. Service Layer
- **Backup Service**: Manages backup jobs, handles streaming, catalog updates
- **Restore Service**: Catalog browsing, file selection, guided restore
- **Tape Service**: Device control, status monitoring, pool management

### 4. Tape I/O Layer
- **mt commands**: Tape positioning, rewinding, ejecting
- **sg_* utilities**: SCSI generic access for advanced operations
- **tar streaming**: Direct streaming writes/reads

### 5. Data Layer
- **Database**: SQLite for portability and simplicity
- **Tables**: Tapes, BackupSets, CatalogEntries, Jobs, Users, AuditLogs

## Technology Choices

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Backend | Go | Excellent for long-running I/O, process management, single binary deployment |
| Database | SQLite | No external dependencies, suitable for metadata-only workload |
| Web UI | SvelteKit | Fast, server-rendered option with minimal complexity |
| Process Control | systemd | Native Debian integration, reliable service management |
| Tape Tools | mt, tar, sg_* | Standard Linux utilities, well-tested |

## Security Model

1. **Authentication**: Local user database with bcrypt password hashing
2. **Authorization**: Role-based (admin, operator, read-only)
3. **Audit Trail**: All operations logged with timestamp and user

## Failure Handling

1. **Job Persistence**: Job state saved to database, resumable after crash
2. **Tape Full**: Detected via I/O error, graceful continuation to new tape
3. **Drive Errors**: Logged, operator notification, job pause
4. **Wrong Tape**: Label verification before write/read operations
