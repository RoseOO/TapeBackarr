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
│  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐          │
│  │ Encryption Service│ │Notification Service│ │  Proxmox Client  │          │
│  │                   │ │                   │ │                   │          │
│  │ - AES-256 encrypt │ │ - Telegram alerts │ │ - VM/LXC backup  │          │
│  │ - Key management  │ │ - Email (SMTP)    │ │ - Guest discovery │          │
│  │ - Key sheets      │ │ - Event routing   │ │ - vzdump stream   │          │
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
                     │ (file list)  │     │ Tracking     │     │ + Label + TOC│
                     └──────────────┘     └──────────────┘     └──────────────┘
```

### Tape Layout

TapeBackarr writes data to tape in a self-describing format with three sections
separated by file marks (FM):

```
[Label (512B)] [FM] [Backup Data (tar)] [FM] [TOC (JSON)] [FM] [EOD]
  File #0               File #1               File #2
```

- **File #0 — Label Block** (512 bytes): Contains tape identity in the format
  `TAPEBACKARR|label|uuid|pool|timestamp|encryption_fingerprint|compression_type`.
  Written once when the tape is first labeled. Read with
  `dd if=/dev/nst0 bs=512 count=1`.

- **File #1 — Backup Data**: Standard tar archive streamed directly from the backup
  source. May be encrypted (AES-256-CBC) and/or compressed (gzip/zstd). Uses a
  configurable block size (default 64KB / 128×512-byte blocks).

- **File #2 — Table of Contents (TOC)**: A JSON document written after the backup
  data completes. Contains the full file catalog (paths, sizes, checksums, timestamps)
  so the tape is self-describing even without access to the database. The TOC is padded
  to 64KB block boundaries. Its size depends on the number of backed-up files (typically
  a few KB to several MB). Read with `mt -f /dev/nst0 fsf 2 && dd if=/dev/nst0 bs=64k`.

The TOC is written **after** the backup data and its trailing file mark, once all file
checksums have been calculated. It does **not** require a rewind — it is appended
sequentially. The same catalog data is also stored in the SQLite database for fast
searching, but the on-tape TOC ensures disaster recovery is possible without the database.


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
- **Encryption Service**: AES-256 encryption, key management, key sheet generation
- **Notification Service**: Telegram bot alerts, Email (SMTP) notifications
- **Proxmox Client**: VM/LXC discovery, vzdump streaming, guest restore

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
| Backend | Go 1.24+ | Excellent for long-running I/O, process management, single binary deployment |
| Database | SQLite | No external dependencies, suitable for metadata-only workload |
| Web UI | SvelteKit | Fast, server-rendered option with minimal complexity |
| Process Control | systemd | Native Debian integration, reliable service management |
| Tape Tools | mt, tar, sg_* | Standard Linux utilities, well-tested |
| Encryption | AES-256-GCM | Industry-standard authenticated encryption |
| Notifications | Telegram API, SMTP | Real-time operator alerts via multiple channels |

## Security Model

1. **Authentication**: Local user database with bcrypt password hashing, API key support
2. **Authorization**: Role-based (admin, operator, read-only)
3. **Encryption**: AES-256-GCM backup encryption with key management
4. **Audit Trail**: All operations logged with timestamp and user

## Failure Handling

1. **Job Persistence**: Job state saved to database, resumable after crash
2. **Tape Full**: Detected via I/O error, graceful continuation to new tape
3. **Drive Errors**: Logged, operator notification, job pause
4. **Wrong Tape**: Label verification before write/read operations
