# TapeBackarr

📼 **Production-grade tape library management system with modern web interface**

TapeBackarr is a disk-light, tape-first backup system designed to run on Debian Linux and manage LTO tape drives. It supports direct streaming from network shares to tape without requiring large intermediate disk storage.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

### Core Capabilities
- **Direct Streaming**: Stream data from SMB/NFS/local filesystem directly to tape
- **Full Cataloging**: Complete file-level catalog with block offset tracking
- **Incremental Backups**: Track file changes via timestamps and size
- **Multi-tape Spanning**: Automatic handling of tape-full conditions with continuation markers
- **Guided Restore**: Operator-friendly restore workflow with tape insertion guidance
- **Notifications**: Real-time alerts via Telegram and Email (SMTP)
- **Encryption**: AES-256 encryption for sensitive backups
- **Database Backup**: Backup the TapeBackarr database itself to tape for disaster recovery
- **Multi-Drive Support**: Manage and select from multiple tape drives
- **Tape Library Support**: Autochanger (tape library) control via SCSI medium changers — automated load, unload, transfer, and inventory
- **Drive Health Monitoring**: Drive statistics, alerts, cleaning, and retension support
- **LTFS Support**: Format, mount, browse, and restore tapes using Linear Tape File System
- **Proxmox VE Integration**: Backup and restore VMs and LXC containers directly to tape

### Tape Management
- Tape labeling and pool assignment (DAILY, WEEKLY, MONTHLY, ARCHIVE)
- Status tracking (blank, active, full, expired, retired, exported)
- Capacity and usage monitoring
- Write count tracking
- Offsite location tracking

### Backup Operations
- Scheduled backups with cron expressions
- Manual backup execution
- Glob-based include/exclude patterns
- Full and incremental backup types
- Job state persistence for resume after crash
- Database backup to tape for disaster recovery

### Restore Operations
- Restore to local filesystem
- Restore to network destinations (SMB/NFS)
- File verification with checksums
- Guided multi-tape restore workflow

### Web Interface
- Modern, responsive dashboard
- Tape management with status updates
- Multi-drive management and selection
- Backup job configuration and scheduling
- Catalog browsing and file search
- Guided restore wizard
- Audit log viewer with export
- Role-based access control (admin/operator/read-only)
- **In-app documentation** - Access all guides from the web UI

### Deployment Options
- **Native Installation**: Debian/Ubuntu with systemd
- **Docker**: Container deployment with docker-compose
- **Proxmox LXC**: Automated installation script for Proxmox community scripts

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Web UI (SvelteKit)                       │
├─────────────────────────────────────────────────────────────────┤
│                      REST API (Go/Chi)                           │
├─────────────────────────────────────────────────────────────────┤
│  Backup Service │ Restore Service │ Tape Service │ Scheduler     │
├─────────────────────────────────────────────────────────────────┤
│                    Tape I/O Layer (mt, tar)                      │
├─────────────────────────────────────────────────────────────────┤
│                     SQLite Database                              │
└─────────────────────────────────────────────────────────────────┘
```

## Requirements

- **OS**: Debian 12+ or Ubuntu 22.04+ (systemd-native)
- **Hardware**: LTO tape drive (/dev/st0, /dev/nst0)
- **Software**: 
  - Go 1.24+
  - Node.js 18+ (for frontend build)
  - mt-st package
  - tar

## Installation

For complete installation instructions, see the [Installation Guide](docs/INSTALLATION.md).

### Quick Install (Native)

```bash
# Clone repository
git clone https://github.com/RoseOO/TapeBackarr.git
cd TapeBackarr

# Build
make build

# Run installer
sudo ./deploy/install.sh
```

### Docker Install

```bash
# Clone repository
git clone https://github.com/RoseOO/TapeBackarr.git
cd TapeBackarr

# Configure
cp deploy/config.example.json config.json
nano config.json

# Start
docker compose up -d
```

### Proxmox LXC Install

Run this command on your **Proxmox host**:

```bash
bash -c "$(wget -qLO - https://github.com/RoseOO/TapeBackarr/raw/main/deploy/proxmox-lxc-install.sh)"
```

This creates an LXC container with TapeBackarr installed and tape device passthrough configured.

### Manual Installation

```bash
# Clone repository
git clone https://github.com/RoseOO/TapeBackarr.git
cd TapeBackarr

# Build backend
go build -o tapebackarr ./cmd/tapebackarr

# Build frontend
cd web/frontend
npm install
npm run build
cd ../..

# Install
sudo mkdir -p /opt/tapebackarr /etc/tapebackarr /var/lib/tapebackarr /var/log/tapebackarr
sudo cp tapebackarr /opt/tapebackarr/
sudo cp deploy/config.example.json /etc/tapebackarr/config.json
sudo cp deploy/tapebackarr.service /etc/systemd/system/

# Edit configuration
sudo nano /etc/tapebackarr/config.json

# Start service
sudo systemctl daemon-reload
sudo systemctl enable tapebackarr
sudo systemctl start tapebackarr
```

### Configuration

Edit `/etc/tapebackarr/config.json`:

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080
  },
  "database": {
    "path": "/var/lib/tapebackarr/tapebackarr.db"
  },
  "tape": {
    "default_device": "/dev/nst0",
    "drives": [
      {
        "device_path": "/dev/nst0",
        "display_name": "Primary LTO Drive",
        "enabled": true
      },
      {
        "device_path": "/dev/nst1",
        "display_name": "Secondary LTO Drive",
        "enabled": false
      }
    ],
    "buffer_size_mb": 512,
    "block_size": 262144,
    "write_retries": 3,
    "verify_after_write": true
  },
  "logging": {
    "level": "info",
    "format": "json",
    "output_path": "/var/log/tapebackarr/tapebackarr.log"
  },
  "auth": {
    "jwt_secret": "YOUR_SECURE_SECRET_HERE",
    "token_expiration": 24,
    "session_timeout": 60
  },
  "notifications": {
    "telegram": {
      "enabled": false,
      "bot_token": "YOUR_BOT_TOKEN",
      "chat_id": "YOUR_CHAT_ID"
    },
    "email": {
      "enabled": false,
      "smtp_host": "smtp.example.com",
      "smtp_port": 587,
      "username": "your-username",
      "password": "your-password",
      "from_email": "tapebackarr@example.com",
      "from_name": "TapeBackarr",
      "to_emails": "admin@example.com",
      "use_tls": true,
      "skip_verify": false
    }
  }
}
```

### Multi-Drive Configuration

TapeBackarr supports multiple tape drives. Configure them in the `tape.drives` array:

```json
"drives": [
  {
    "device_path": "/dev/nst0",
    "display_name": "Primary LTO-8 Drive",
    "enabled": true
  },
  {
    "device_path": "/dev/nst1",
    "display_name": "Secondary LTO-6 Drive",
    "enabled": true
  }
]
```

You can also add and manage drives through the web UI under the **Drives** section.

### Notification Setup

TapeBackarr supports both Telegram and Email notifications for critical events like tape changes, backup completion, and errors.

#### Telegram Notifications

1. Create a bot with [@BotFather](https://t.me/botfather) on Telegram
2. Get your chat ID by messaging the bot and visiting `https://api.telegram.org/bot{YOUR_TOKEN}/getUpdates`
3. Enable in config:
   ```json
   "notifications": {
     "telegram": {
       "enabled": true,
       "bot_token": "123456789:ABCdefGHIjklMNO...",
       "chat_id": "-1001234567890"
     }
   }
   ```
4. Restart TapeBackarr

#### Email Notifications (SMTP)

Configure SMTP settings to receive email notifications:

```json
"notifications": {
  "email": {
    "enabled": true,
    "smtp_host": "smtp.gmail.com",
    "smtp_port": 587,
    "username": "your-email@gmail.com",
    "password": "your-app-password",
    "from_email": "tapebackarr@yourdomain.com",
    "from_name": "TapeBackarr",
    "to_emails": "admin@yourdomain.com, operator@yourdomain.com",
    "use_tls": true,
    "skip_verify": false
  }
}
```

**Note:** For Gmail, use an [App Password](https://support.google.com/accounts/answer/185833) instead of your regular password.

**Notification Events:**
- 📼 Tape change required
- 📀 Tape full
- ✅ Backup completed
- ❌ Backup failed
- 🚨 Drive error

## Usage

### Access Web UI

Open `http://your-server:8080` in a browser.

Default credentials:
- **Username**: admin
- **Password**: changeme

⚠️ **Change the default password immediately!**

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/auth/login` | POST | Authenticate user |
| `/api/v1/auth/change-password` | POST | Change password |
| `/api/v1/dashboard` | GET | Dashboard statistics |
| `/api/v1/tapes` | GET/POST | List/create tapes |
| `/api/v1/tapes/{id}` | GET/PUT/DELETE | Manage tape |
| `/api/v1/tapes/{id}/label` | POST | Write tape label |
| `/api/v1/tapes/{id}/format` | POST | Format tape |
| `/api/v1/tapes/{id}/export` | POST | Export tape |
| `/api/v1/tapes/{id}/import` | POST | Import tape |
| `/api/v1/tapes/{id}/read-label` | GET | Read tape label |
| `/api/v1/tapes/lto-types` | GET | List LTO types |
| `/api/v1/tapes/batch-label` | POST | Batch label tapes |
| `/api/v1/tapes/batch-label/status` | GET | Batch label status |
| `/api/v1/tapes/batch-label/cancel` | POST | Cancel batch label |
| `/api/v1/tapes/batch-update` | POST | Batch update tapes |
| `/api/v1/pools` | GET/POST | List/create pools |
| `/api/v1/pools/{id}` | GET/PUT/DELETE | Manage pool |
| `/api/v1/drives` | GET/POST | List/create drives |
| `/api/v1/drives/scan` | GET | Scan for drives |
| `/api/v1/drives/{id}` | PUT/DELETE | Manage drive |
| `/api/v1/drives/{id}/status` | GET | Drive status |
| `/api/v1/drives/{id}/select` | POST | Select active drive |
| `/api/v1/drives/{id}/eject` | POST | Eject tape |
| `/api/v1/drives/{id}/rewind` | POST | Rewind tape |
| `/api/v1/drives/{id}/inspect-tape` | GET | Inspect tape contents |
| `/api/v1/drives/{id}/detect-tape` | GET | Detect tape in drive |
| `/api/v1/drives/{id}/scan-for-db-backup` | GET | Scan tape for database backup |
| `/api/v1/drives/{id}/batch-label` | POST | Batch label tapes |
| `/api/v1/drives/{id}/statistics` | GET | Drive statistics |
| `/api/v1/drives/{id}/alerts` | GET | Drive alerts |
| `/api/v1/drives/{id}/clean` | POST | Clean drive |
| `/api/v1/drives/{id}/retension` | POST | Retension tape |
| `/api/v1/sources` | GET/POST | List/create sources |
| `/api/v1/sources/{id}` | GET/PUT/DELETE | Manage source |
| `/api/v1/jobs` | GET/POST | List/create jobs |
| `/api/v1/jobs/active` | GET | List active jobs |
| `/api/v1/jobs/resumable` | GET | List resumable jobs |
| `/api/v1/jobs/{id}` | GET/PUT/DELETE | Manage job |
| `/api/v1/jobs/{id}/run` | POST | Run backup job |
| `/api/v1/jobs/{id}/cancel` | POST | Cancel running job |
| `/api/v1/jobs/{id}/pause` | POST | Pause running job |
| `/api/v1/jobs/{id}/resume` | POST | Resume paused job |
| `/api/v1/jobs/{id}/retry` | POST | Retry failed job |
| `/api/v1/jobs/{id}/recommend-tape` | GET | Get tape recommendation |
| `/api/v1/backup-sets` | GET | List backup sets |
| `/api/v1/backup-sets/{id}` | GET | Get backup set details |
| `/api/v1/backup-sets/{id}` | DELETE | Delete backup set |
| `/api/v1/backup-sets/{id}/files` | GET | List backup set files |
| `/api/v1/backup-sets/{id}/cancel` | POST | Cancel backup set |
| `/api/v1/catalog/search` | GET | Search catalog |
| `/api/v1/catalog/browse/{backupSetId}` | GET | Browse catalog |
| `/api/v1/restore/plan` | POST | Plan restore |
| `/api/v1/restore/run` | POST | Execute restore |
| `/api/v1/restore/raw-read` | POST | Raw read data from tape |
| `/api/v1/logs/audit` | GET | Audit logs |
| `/api/v1/logs/export` | GET | Export logs |
| `/api/v1/users` | GET/POST | List/create users (admin) |
| `/api/v1/users/{id}` | DELETE | Delete user (admin) |
| `/api/v1/settings` | GET/PUT | Get/update settings |
| `/api/v1/settings/telegram/test` | POST | Test Telegram notification |
| `/api/v1/settings/restart` | POST | Restart service |
| `/api/v1/encryption-keys` | GET/POST | List/create encryption keys |
| `/api/v1/encryption-keys/keysheet` | GET | Get encryption key sheet |
| `/api/v1/encryption-keys/keysheet/text` | GET | Get key sheet as text |
| `/api/v1/encryption-keys/{id}` | DELETE | Delete encryption key |
| `/api/v1/api-keys` | GET/POST | List/create API keys (admin) |
| `/api/v1/api-keys/{id}` | DELETE | Delete API key (admin) |
| `/api/v1/database-backup` | GET | List database backups |
| `/api/v1/database-backup/backup` | POST | Backup database to tape |
| `/api/v1/database-backup/restore` | POST | Restore database from tape |
| `/api/v1/events/stream` | GET | Server-sent events stream |
| `/api/v1/events` | GET | Get notifications |
| `/api/v1/docs` | GET | List documentation |
| `/api/v1/docs/{id}` | GET | Get document content |
| `/api/v1/health` | GET | Health check |
| `/api/v1/proxmox/nodes` | GET | List Proxmox nodes |
| `/api/v1/proxmox/guests` | GET | List VMs and LXCs |
| `/api/v1/proxmox/guests/{vmid}` | GET | Get guest details |
| `/api/v1/proxmox/guests/{vmid}/config` | GET | Get guest configuration |
| `/api/v1/proxmox/cluster/status` | GET | Cluster status |
| `/api/v1/proxmox/backups` | GET/POST | List/create Proxmox backups |
| `/api/v1/proxmox/backups/{id}` | GET | Get Proxmox backup details |
| `/api/v1/proxmox/backups/all` | POST | Backup all guests |
| `/api/v1/proxmox/restores` | GET/POST | List/create Proxmox restores |
| `/api/v1/proxmox/restores/plan` | POST | Plan Proxmox restore |
| `/api/v1/proxmox/jobs` | GET/POST | List/create Proxmox backup jobs |
| `/api/v1/proxmox/jobs/{id}` | GET/PUT/DELETE | Manage Proxmox job |
| `/api/v1/proxmox/jobs/{id}/run` | POST | Run Proxmox job manually |
| `/api/v1/libraries` | GET/POST | List/create tape libraries |
| `/api/v1/libraries/scan` | GET | Scan for autochangers |
| `/api/v1/libraries/{id}` | GET/PUT/DELETE | Manage tape library |
| `/api/v1/libraries/{id}/inventory` | POST | Run library inventory |
| `/api/v1/libraries/{id}/slots` | GET | List library slots |
| `/api/v1/libraries/{id}/load` | POST | Load tape into drive |
| `/api/v1/libraries/{id}/unload` | POST | Unload tape from drive |
| `/api/v1/libraries/{id}/transfer` | POST | Transfer tape between slots |
| `/api/v1/ltfs/status` | GET | LTFS status |
| `/api/v1/ltfs/format` | POST | Format tape with LTFS |
| `/api/v1/ltfs/mount` | POST | Mount LTFS tape |
| `/api/v1/ltfs/unmount` | POST | Unmount LTFS tape |
| `/api/v1/ltfs/browse` | GET | Browse LTFS tape contents |
| `/api/v1/ltfs/restore` | POST | Restore files from LTFS tape |
| `/api/v1/ltfs/check` | POST | LTFS consistency check |

### CLI Commands Used Internally

```bash
# Tape status
mt -f /dev/nst0 status

# Rewind tape
mt -f /dev/nst0 rewind

# Eject tape
mt -f /dev/nst0 eject

# Write file mark
mt -f /dev/nst0 weof 1

# Seek to file number
mt -f /dev/nst0 fsf 5

# Create backup (streaming)
tar -cv -b 128 -C /source/path -T filelist.txt -f /dev/nst0

# Restore from tape
tar -xv -b 128 -f /dev/nst0 -C /restore/path
```

## Data Model

### Main Tables
- **users**: User accounts with roles
- **tape_pools**: Tape groupings (DAILY, WEEKLY, etc.)
- **tapes**: Individual tape media with UUID tracking
- **tape_drives**: Physical tape drive tracking
- **backup_sources**: Configured backup paths
- **backup_jobs**: Scheduled backup jobs
- **backup_sets**: Individual backup runs
- **catalog_entries**: File-level catalog
- **job_executions**: Job execution tracking with resume support
- **snapshots**: Filesystem snapshots for incremental backups
- **audit_logs**: Operation audit trail
- **encryption_keys**: Encryption key management
- **api_keys**: API key authentication
- **database_backups**: Database backup tracking
- **restore_operations**: Restore operation tracking
- **tape_spanning_sets**: Multi-tape backup sets
- **tape_spanning_members**: Individual tapes in spanning sets
- **tape_change_requests**: Pending tape change requests
- **tape_libraries**: Tape library (autochanger) tracking
- **tape_library_slots**: Tape library slot contents
- **drive_statistics**: Drive usage metrics and health data
- **drive_alerts**: Drive health monitoring alerts
- **proxmox_nodes**: Proxmox VE node tracking
- **proxmox_guests**: VM and LXC container tracking
- **proxmox_backups**: Proxmox backup records
- **proxmox_restores**: Proxmox restore records
- **proxmox_backup_jobs**: Scheduled Proxmox backup jobs
- **proxmox_job_executions**: Proxmox job execution tracking

### Tape Status Flow
```
blank → active → full → expired → retired
                  ↓
               exported
```

## Incremental Backup Algorithm

1. Scan source directory for all files
2. Apply include/exclude filters
3. Load previous snapshot from database
4. Compare files:
   - New files → include
   - Modified (mtime or size changed) → include
   - Unchanged → skip
5. Stream changed files to tape
6. Save new snapshot to database

## Recovery Strategy

### Single Tape Recovery
1. Search catalog for file(s)
2. Identify required tape
3. Load tape, position to backup set
4. Extract requested files

### Multi-Tape Recovery
1. System identifies all required tapes
2. Displays insertion order to operator
3. Guides through tape changes
4. Verifies checksums after restore

### Crash Recovery
- Job state persisted to database
- Resume capability for interrupted backups
- Automatic cleanup of partial backup sets

## Logging

All operations are logged in JSON format:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "message": "Backup completed",
  "fields": {
    "job_id": 1,
    "file_count": 1500,
    "total_bytes": 52428800000
  }
}
```

Audit logs capture:
- User authentication
- Tape operations (mount, unmount, label)
- Backup job execution
- Restore operations
- Configuration changes

## Security

- JWT-based authentication
- Role-based access control
- bcrypt password hashing
- Audit logging for compliance
- No secrets in logs

## Troubleshooting

### Tape Not Detected
```bash
# Check device
ls -la /dev/st* /dev/nst*

# Check permissions
groups $(whoami)  # Should include 'tape'

# Manual status check
mt -f /dev/nst0 status
```

### Backup Fails
1. Check tape is loaded and not write-protected
2. Verify source path is accessible
3. Check disk space for temporary files
4. Review logs in `/var/log/tapebackarr/`

### Database Issues
```bash
# Backup database
sqlite3 /var/lib/tapebackarr/tapebackarr.db ".backup backup.db"

# Check database integrity
sqlite3 /var/lib/tapebackarr/tapebackarr.db "PRAGMA integrity_check"
```

## Documentation

Documentation is available in two ways:

### In-App Documentation

Access documentation directly from the web interface by clicking **Documentation** in the sidebar. This provides access to all guides without leaving the application.

### Document Files

- [**Installation Guide**](docs/INSTALLATION.md) - Complete installation instructions (Native, Docker, Proxmox LXC)
- [**Usage Guide**](docs/USAGE_GUIDE.md) - Complete guide for using TapeBackarr
- [**API Reference**](docs/API_REFERENCE.md) - REST API documentation
- [**Operator Guide**](docs/OPERATOR_GUIDE.md) - Quick reference for daily operations
- [**Manual Recovery**](docs/MANUAL_RECOVERY.md) - Recover data without TapeBackarr
- [**Architecture**](docs/ARCHITECTURE.md) - System design and data flows
- [**Database Schema**](docs/DATABASE_SCHEMA.md) - Database table definitions
- [**Proxmox Guide**](docs/PROXMOX_GUIDE.md) - Backup and restore Proxmox VMs and LXCs

### Disaster Recovery

The [Manual Recovery Guide](docs/MANUAL_RECOVERY.md) provides detailed instructions for recovering tape data using only standard Linux commands (mt, tar), without requiring TapeBackarr. This is essential for long-term archival scenarios where the application may not be available.

Key recovery capabilities:
- Read tape labels and catalog contents
- Restore files using raw mt/tar commands
- Handle multi-tape spanning sets
- Recover the TapeBackarr database from tape
- Restore encrypted backups with your key sheet

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting PRs.

## Security

For security-related information, see [SECURITY.md](SECURITY.md).

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version history.

## License

MIT License - see [LICENSE](LICENSE) file for details.
