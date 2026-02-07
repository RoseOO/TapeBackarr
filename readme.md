# TapeBackarr

📼 **Production-grade tape library management system with modern web interface**

TapeBackarr is a disk-light, tape-first backup system designed to run on Debian Linux and manage LTO tape drives. It supports direct streaming from network shares to tape without requiring large intermediate disk storage.

## Features

### Core Capabilities
- **Direct Streaming**: Stream data from SMB/NFS/local filesystem directly to tape
- **Full Cataloging**: Complete file-level catalog with block offset tracking
- **Incremental Backups**: Track file changes via timestamps and size
- **Multi-tape Spanning**: Automatic handling of tape-full conditions with continuation markers
- **Guided Restore**: Operator-friendly restore workflow with tape insertion guidance
- **Telegram Notifications**: Real-time alerts when tapes need to be changed

### Tape Management
- Tape labeling and pool assignment (DAILY, WEEKLY, MONTHLY, ARCHIVE)
- Status tracking (blank, active, full, retired, offsite)
- Capacity and usage monitoring
- Write count tracking
- Offsite location tracking

### Backup Operations
- Scheduled backups with cron expressions
- Manual backup execution
- Glob-based include/exclude patterns
- Full and incremental backup types
- Job state persistence for resume after crash

### Web Interface
- Modern, responsive dashboard
- Tape management with status updates
- Backup job configuration and scheduling
- Catalog browsing and file search
- Guided restore wizard
- Audit log viewer with export
- Role-based access control (admin/operator/read-only)

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

- **OS**: Debian 12+ (systemd-native)
- **Hardware**: LTO tape drive (/dev/st0, /dev/nst0)
- **Software**: 
  - Go 1.21+
  - Node.js 18+ (for frontend build)
  - mt-st package
  - tar

## Installation

### Build from Source

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
    "buffer_size_mb": 256,
    "block_size": 65536,
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
    }
  }
}
```

### Telegram Notifications Setup

To receive notifications when tapes need to be changed:

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
| `/api/v1/dashboard` | GET | Dashboard statistics |
| `/api/v1/tapes` | GET/POST | List/create tapes |
| `/api/v1/tapes/{id}` | GET/PUT/DELETE | Manage tape |
| `/api/v1/pools` | GET/POST | List/create pools |
| `/api/v1/sources` | GET/POST | List/create sources |
| `/api/v1/jobs` | GET/POST | List/create jobs |
| `/api/v1/jobs/{id}/run` | POST | Run backup job |
| `/api/v1/backup-sets` | GET | List backup sets |
| `/api/v1/catalog/search` | GET | Search catalog |
| `/api/v1/restore/plan` | POST | Plan restore |
| `/api/v1/restore/run` | POST | Execute restore |
| `/api/v1/logs/audit` | GET | Audit logs |

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
- **tapes**: Individual tape media
- **backup_sources**: Configured backup paths
- **backup_jobs**: Scheduled backup jobs
- **backup_sets**: Individual backup runs
- **catalog_entries**: File-level catalog
- **audit_logs**: Operation audit trail

### Tape Status Flow
```
blank → active → full → retired
                  ↓
               offsite
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

For detailed documentation, see:

- [**Usage Guide**](docs/USAGE_GUIDE.md) - Complete guide for using TapeBackarr
- [**API Reference**](docs/API_REFERENCE.md) - REST API documentation
- [**Operator Guide**](docs/OPERATOR_GUIDE.md) - Quick reference for daily operations
- [**Architecture**](docs/ARCHITECTURE.md) - System design and data flows
- [**Database Schema**](docs/DATABASE_SCHEMA.md) - Database table definitions

## License

MIT License - see LICENSE file for details.

## Contributing

Contributions welcome! Please read CONTRIBUTING.md before submitting PRs.
