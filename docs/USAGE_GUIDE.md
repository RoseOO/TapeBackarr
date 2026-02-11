# TapeBackarr Usage Guide

Complete guide to using TapeBackarr for tape backup and restore operations.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Web Interface Overview](#web-interface-overview)
3. [Managing Tapes](#managing-tapes)
4. [Configuring Backup Sources](#configuring-backup-sources)
5. [Creating and Running Backup Jobs](#creating-and-running-backup-jobs)
6. [Multi-Tape Spanning](#multi-tape-spanning)
7. [Restoring Data](#restoring-data)
8. [LTFS Management](#ltfs-management)
9. [Notifications](#notifications)
10. [Viewing Logs](#viewing-logs)
11. [Tape Inspection and Encryption](#tape-inspection-and-encryption)
12. [API Keys](#api-keys)
13. [User Management](#user-management)
14. [Database Backup](#database-backup)
15. [In-App Documentation](#in-app-documentation)
16. [Best Practices](#best-practices)
17. [Troubleshooting](#troubleshooting)

---

## Getting Started

### First Login

1. Open your browser and navigate to `http://your-server:8080`
2. Log in with the default credentials:
   - **Username**: `admin`
   - **Password**: `changeme`
3. **Immediately change the default password** via the Users page

### Initial Setup Checklist

1. [ ] Change admin password
2. [ ] Add tape drives to the system
3. [ ] Label and register your tapes
4. [ ] Configure backup sources (SMB/NFS/local paths)
5. [ ] Create backup jobs
6. [ ] Set up Telegram notifications (optional)

---

## Web Interface Overview

### Dashboard

The dashboard provides an at-a-glance view of your system:

- **System Status**: Drive status, active jobs, warnings
- **Quick Stats**: Total tapes, backup sets, data written
- **Recent Activity**: Latest backup and restore operations
- **Alerts**: Any pending tape changes or errors

### Navigation

| Section | Purpose |
|---------|---------|
| Dashboard | System overview and quick stats |
| Tapes | Manage tape inventory |
| Drives | Manage and select tape drives |
| Pools | Configure tape pools and retention policies |
| Sources | Configure backup source paths |
| Jobs | Create and manage backup jobs |
| Restore | Browse catalog and restore files |
| LTFS | Format, mount, and browse LTFS tapes |
| Encryption | Manage encryption keys and key sheets |
| API Keys | Manage API tokens for programmatic access |
| Proxmox | Proxmox VE integration for VM/LXC backups |
| Libraries | Tape library (autochanger) management |
| Inspect | View tape contents from the web UI |
| Logs | View operation logs and audit trail |
| Settings | System configuration and service control |
| Documentation | Access guides and references |
| Users | Manage user accounts (admin only) |

---

## Managing Tape Drives

TapeBackarr supports multiple tape drives. Use the **Drives** page to manage them.

### Adding a New Drive

1. Navigate to **Drives** in the sidebar
2. Click **Add Drive**
3. Enter the drive details:
   - **Device Path**: The Linux device path (e.g., `/dev/nst0`)
   - **Display Name**: A friendly name (e.g., "Primary LTO-8 Drive")
   - **Model**: Optional model information (e.g., "LTO-8")
   - **Serial Number**: Optional serial number
4. Click **Add Drive**

### Managing Drives

| Action | Description |
|--------|-------------|
| **Select** | Choose which drive to use for operations |
| **Rewind** | Rewind the tape to the beginning |
| **Eject** | Eject the tape from the drive |
| **Remove** | Remove the drive from TapeBackarr |

### Drive Status

| Status | Description |
|--------|-------------|
| Ready | Drive is online and ready |
| Busy | Drive is performing an operation |
| Offline | Drive is not responding |
| Error | Drive has encountered an error |

### Configuring Multiple Drives

You can also configure drives in the configuration file:

```json
{
  "tape": {
    "default_device": "/dev/nst0",
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
  }
}
```

---

## Managing Tapes

### Adding a New Tape

1. Navigate to **Tapes** in the sidebar
2. Click **Add Tape**
3. Enter the tape details:
   - **Barcode**: The physical barcode on the tape (required)
   - **Label**: A human-readable name (e.g., "WEEKLY-001")
   - **Pool**: Select the tape pool (DAILY, WEEKLY, MONTHLY, ARCHIVE)
   - **Capacity**: Tape capacity in bytes (default: LTO-8 = 12TB)
4. Click **Save**

### Labeling Tapes

TapeBackarr writes a label block at the beginning of each tape:

1. Insert the tape into the drive
2. Navigate to **Tapes** → Select the tape
3. Click **Write Label**
4. Confirm the operation (this will rewind and write to the tape)

The label format is: `TAPEBACKARR|label|uuid|pool|timestamp|encryption_fingerprint|compression_type`

### Tape Pools

Pools help organize tapes by retention policy:

| Pool | Typical Use | Default Retention |
|------|-------------|-------------------|
| DAILY | Daily backups | 7 days |
| WEEKLY | Weekly backups | 30 days |
| MONTHLY | Monthly backups | 365 days |
| ARCHIVE | Long-term archival | Indefinite |

### Tape Status Workflow

```
blank → active → full → expired → retired
                  ↓
               exported
```

- **blank**: New tape, never written
- **active**: In use, has space remaining
- **full**: Tape is full
- **expired**: Tape retention period has elapsed
- **retired**: No longer in use
- **exported**: Removed from the library (e.g., sent offsite)

### Marking Tapes

- **Export**: When tape is removed from the library (e.g., moved to offsite storage)
- **Import**: When tape returns to the library
- **Mark as Retired**: When tape is no longer usable

---

## Configuring Backup Sources

### Supported Source Types

| Type | Path Example | Notes |
|------|--------------|-------|
| Local | `/data/backups` | Direct filesystem path |
| NFS | `/mnt/nfs/share` | Mount NFS share first |
| SMB/CIFS | `/mnt/smb/share` | Mount SMB share first |

### Adding a Source

1. Navigate to **Sources**
2. Click **Add Source**
3. Configure:
   - **Name**: Descriptive name (e.g., "FileServer-Home")
   - **Type**: Select local, nfs, or smb
   - **Path**: Full path to the mounted directory
   - **Include Patterns**: Files to include (glob patterns)
   - **Exclude Patterns**: Files to exclude (glob patterns)

### Pattern Examples

**Include Patterns:**
```
*.doc
*.pdf
*.xlsx
important/*
```

**Exclude Patterns:**
```
*.tmp
*.log
cache/*
.git/*
node_modules/*
```

### Pre-mounting Network Shares

Before using NFS or SMB sources, mount them on the system:

```bash
# NFS mount
sudo mount -t nfs fileserver:/export/data /mnt/nfs/data

# SMB mount
sudo mount -t cifs //server/share /mnt/smb/share -o username=user,password=pass

# Add to /etc/fstab for persistent mounts
```

---

## Creating and Running Backup Jobs

### Creating a Backup Job

1. Navigate to **Jobs**
2. Click **Create Job**
3. Configure:
   - **Name**: Job name (e.g., "Daily-FileServer")
   - **Source**: Select the backup source
   - **Pool**: Target tape pool
   - **Backup Type**: Full or Incremental
   - **Schedule**: Cron expression (or leave empty for manual)

### Schedule Examples (Cron Format)

| Expression | Description |
|------------|-------------|
| `0 2 * * *` | Daily at 2:00 AM |
| `0 3 * * 0` | Weekly on Sunday at 3:00 AM |
| `0 4 1 * *` | Monthly on the 1st at 4:00 AM |
| `0 */6 * * *` | Every 6 hours |

### Backup Types

**Full Backup:**
- Backs up all files matching the include/exclude patterns
- Creates a complete snapshot
- Use for initial backups and periodic full copies

**Incremental Backup:**
- Only backs up files changed since the last backup
- Compares modification time and file size
- Faster and uses less tape space

### Running a Backup Manually

1. Navigate to **Jobs**
2. Find the job you want to run
3. Click **Run Now**
4. Monitor progress on the Dashboard

---

## Multi-Tape Spanning

TapeBackarr automatically handles backups that span multiple tapes.

### How Spanning Works

1. When a tape becomes full during a backup:
   - The current tape is marked as full
   - A **tape change notification** is sent (if configured)
   - The backup pauses and waits for a new tape

2. The operator:
   - Receives notification via Telegram (if configured)
   - Ejects the current tape
   - Inserts a new tape
   - Acknowledges the change in the web interface

3. The backup continues:
   - The new tape is labeled
   - A continuation marker links the tapes
   - The backup resumes from where it stopped

### Spanning Markers

Each tape in a spanning set contains:
- **Set ID**: Unique identifier for the backup set
- **Sequence Number**: Position in the spanning sequence
- **Previous Tape**: Reference to the previous tape
- **Next Tape**: Reference to the next tape (updated when known)

### Tape Insertion Guidance

During restore, the system guides you through tape changes:

```
Restore requires 3 tapes in this order:
1. WEEKLY-001 (insert first)
2. WEEKLY-002 (continue)
3. WEEKLY-003 (final tape)

Please insert tape WEEKLY-001 and click Continue.
```

---

## Restoring Data

### Browsing the Catalog

1. Navigate to **Restore**
2. Use the search box to find files:
   - Search by filename: `report.pdf`
   - Search with wildcards: `*.xlsx`
   - Search by path: `/data/documents/*`
3. Browse results to select files for restore

### Restore Options

| Option | Description |
|--------|-------------|
| Full Path | Restore to original location |
| Custom Path | Restore to a different location |
| Destination Type | Local, SMB, or NFS path |
| Overwrite | Replace existing files |
| Skip Existing | Don't overwrite existing files |
| Verify | Verify checksums after restore |

### Restore Destination Types

TapeBackarr supports restoring to different destination types:

| Type | Description | Example Path |
|------|-------------|--------------|
| **Local** | Direct filesystem path | `/restore/output` |
| **SMB/CIFS** | Pre-mounted SMB share | `/mnt/smb/restore` |
| **NFS** | Pre-mounted NFS share | `/mnt/nfs/restore` |

**Note:** For network destinations (SMB/NFS), mount the share first using standard Linux commands:

```bash
# Mount SMB share
sudo mount -t cifs //server/share /mnt/smb/restore -o username=user,password=pass

# Mount NFS share
sudo mount -t nfs server:/export/path /mnt/nfs/restore
```

### Restore Process

1. **Select files** from the catalog
2. **Review required tapes** - system shows which tapes are needed
3. **Choose destination** - local path or pre-mounted network share
4. **Insert first tape** as directed
5. **Start restore** - files are extracted to the destination
6. **Change tapes** if prompted (for multi-tape restores)
7. **Verify** - optionally verify restored file checksums

### Restore Single File

1. Search for the file in the catalog
2. Click on the file name
3. Click **Restore File**
4. Choose destination
5. Insert required tape when prompted
6. File is restored

---

## LTFS Management

TapeBackarr supports LTFS (Linear Tape File System), which allows tapes to be used like regular filesystems with drag-and-drop file access.

### Supported Drives

LTFS requires **LTO-5 or later** tape drives (LTO-5 introduced the dual-partition capability that LTFS depends on). The following drive vendors are supported:

| Vendor | LTFS Backend | Notes |
|--------|-------------|-------|
| IBM | `lin_tape` or `sg` | IBM drives can use either the proprietary lin_tape or generic sg backend |
| HP / HPE | `sg` | Uses the Linux SCSI Generic (sg) backend |
| Tandberg / Overland-Tandberg | `sg` | Fully supported including LTO-5 HH models |
| Quantum | `sg` | Uses the Linux SCSI Generic (sg) backend |
| Other LTO vendors | `sg` | Most LTO drives work with the sg backend |

> **Overland-Tandberg LTO-5 HH**: This drive is fully supported. LTFS uses the SCSI Generic (`sg`) backend which works with Tandberg/Overland-Tandberg drives out of the box. The open-source [LinearTapeFileSystem/ltfs](https://github.com/LinearTapeFileSystem/ltfs) project should be built with `--enable-sg` (default) for these drives.

TapeBackarr automatically detects your drive vendor and LTO generation and shows compatibility status on the **LTFS** page.

### LTFS vs Raw Format

| Feature | Raw (tar) | LTFS |
|---------|-----------|------|
| Format | tar archive with label/TOC | Filesystem on tape |
| Access | Sequential read/write | Random file access |
| Best for | Large streaming backups | Interoperability, file-level access |
| Requires | mt, tar | ltfs utilities |

### Formatting a Tape with LTFS

1. Navigate to **LTFS** in the sidebar
2. Select a drive with a loaded tape
3. Click **Format** to initialize the tape with LTFS
4. ⚠️ This erases all existing data on the tape

### Mounting and Unmounting

1. **Mount**: Click **Mount** to make the LTFS tape accessible as a filesystem
2. **Browse**: Once mounted, browse the tape contents directly in the web UI
3. **Unmount**: Always unmount before ejecting the tape

### Restoring from LTFS

1. Navigate to **LTFS**
2. Mount the tape if not already mounted
3. Browse to the files you need
4. Click **Restore** to copy files to a local destination

### LTFS Consistency Check

If an LTFS tape was not cleanly unmounted, run a consistency check:

1. Navigate to **LTFS**
2. Click **Check** to verify tape integrity
3. Review the results for any issues

---

## Notifications

TapeBackarr can send notifications via Telegram and Email when operator action is required.

### Telegram Notifications

#### Setting Up Telegram Bot

1. **Create a Telegram Bot:**
   - Message [@BotFather](https://t.me/botfather) on Telegram
   - Send `/newbot`
   - Follow the prompts to create your bot
   - Save the **API Token** provided

2. **Get Your Chat ID:**
   - Message your new bot (or add it to a group)
   - Visit: `https://api.telegram.org/bot{YOUR_TOKEN}/getUpdates`
   - Find the `chat.id` value in the response

3. **Configure TapeBackarr:**
   Edit `/etc/tapebackarr/config.json`:
   ```json
   {
     "notifications": {
       "telegram": {
         "enabled": true,
         "bot_token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
         "chat_id": "-1001234567890"
       }
     }
   }
   ```

4. **Restart TapeBackarr:**
   ```bash
   sudo systemctl restart tapebackarr
   ```

### Notification Types

| Event | Priority | When Sent |
|-------|----------|-----------|
| Tape Change Required | 🔴 High | Tape full, need new tape |
| Tape Full | 🔴 Urgent | Tape has reached capacity |
| Backup Started | 🟢 Normal | Job begins execution |
| Backup Completed | 🟢 Normal | Job finishes successfully |
| Backup Failed | 🔴 Urgent | Job encounters an error |
| Drive Error | 🔴 Urgent | Hardware issue detected |
| Wrong Tape | 🟡 High | Inserted tape doesn't match expected |

### Example Notification

```
📼 *Tape Change Required*

Job 'Daily-FileServer' requires a tape change.

Current tape: WEEKLY-001
Reason: Tape full

Please insert a new tape and acknowledge in the web interface.

*Details:*
• Job: Daily-FileServer
• CurrentTape: WEEKLY-001
• Reason: Tape full

_Sent at 2024-01-15 14:30:45_
```

### Email Notifications (SMTP)

TapeBackarr also supports email notifications for the same events.

#### Configuring Email

Edit `/etc/tapebackarr/config.json`:

```json
{
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
}
```

**Note:** For Gmail, use an [App Password](https://support.google.com/accounts/answer/185833) instead of your regular password.

#### Testing Notifications

Use the Settings page or API to test your notification configuration:

```bash
# Test Telegram
curl -X POST http://localhost:8080/api/v1/settings/telegram/test \
  -H "Authorization: Bearer <token>"
```

---

## Viewing Logs

### Log Types

| Log | Contents |
|-----|----------|
| Job Logs | Backup and restore job execution |
| Tape Logs | Tape operations (mount, eject, write) |
| Audit Logs | User actions and security events |
| System Logs | Application events and errors |

### Accessing Logs

1. Navigate to **Logs** in the sidebar
2. Select log type using tabs
3. Filter by:
   - Date range
   - Log level (info, warn, error)
   - Search text

### Exporting Logs

1. Apply desired filters
2. Click **Export**
3. Choose format:
   - **JSON**: Machine-readable, for analysis
   - **CSV**: For spreadsheets

### Log File Location

Logs are stored at: `/var/log/tapebackarr/tapebackarr.log`

View with standard tools:
```bash
# View recent logs
tail -f /var/log/tapebackarr/tapebackarr.log

# Search logs
grep "backup" /var/log/tapebackarr/tapebackarr.log

# Parse JSON logs
cat /var/log/tapebackarr/tapebackarr.log | jq '.message'
```

---

## Tape Inspection

### Viewing Tape Contents

You can inspect the contents of any tape loaded in a drive directly from the web UI:

1. Navigate to **Drives**
2. Click the **🔍 Inspect** button next to the drive containing the tape
3. The inspection modal shows:
   - **Label information**: Label, UUID, Pool, and timestamp from the TapeBackarr header
   - **Encryption status**: Whether the tape data is encrypted, and the key fingerprint used
   - **File contents**: A listing of files stored on the tape (up to 1000 entries)

If the tape contains encrypted data, the file listing will not be available — the modal will display the encryption key fingerprint needed for decryption.

### Encryption Management

TapeBackarr tracks encryption key fingerprints on tapes for visibility in the tape library:

- When a backup job uses encryption, the key fingerprint and name are stored on the tape record
- The **Tapes** page shows a 🔒 lock icon with the key name for encrypted tapes
- Hovering over the lock icon reveals the full key fingerprint
- The tape label on the physical tape also stores the encryption fingerprint (6th field in the label format)

### Decryption During Restore

When restoring from an encrypted tape:

1. TapeBackarr automatically detects the encryption key fingerprint from the tape label
2. Ensure the corresponding encryption key is present in the **Encryption Keys** section
3. The restore process will use the matching key to decrypt the data transparently

### Restarting TapeBackarr

You can restart the TapeBackarr service from the web UI:

1. Navigate to **Settings**
2. Click the **System** tab
3. Click **🔄 Restart TapeBackarr**
4. Confirm the restart when prompted
5. The page will automatically reload after 5 seconds

> **Warning:** Restarting will interrupt any active backup or restore operations. Use this after making configuration changes that require a service restart.

---

## API Keys

API keys provide programmatic access to TapeBackarr without using username/password authentication. They are useful for scripting and automation.

### Creating an API Key (Admin Only)

1. Navigate to **API Keys** in the sidebar
2. Click **Create API Key**
3. Enter a descriptive name (e.g., "Monitoring Script")
4. Click **Create**
5. **Copy the key immediately** — it will not be shown again

### Using an API Key

Include the key in the `Authorization` header:

```bash
curl -H "Authorization: Bearer <api-key>" http://localhost:8080/api/v1/dashboard
```

### Managing API Keys

- **View**: See all active API keys and their creation dates
- **Delete**: Revoke an API key when no longer needed
- Rotate keys periodically for security

---

## User Management

### User Roles

| Role | Capabilities |
|------|-------------|
| **Admin** | Full access: manage users, configuration, all operations |
| **Operator** | Run backups/restores, manage tapes, view logs |
| **Read-Only** | View dashboard, tapes, jobs, logs (no modifications) |

### Creating Users (Admin Only)

1. Navigate to **Users**
2. Click **Add User**
3. Enter:
   - **Username**: Login name
   - **Password**: Initial password (user should change)
   - **Role**: Select role
4. Click **Create**

### Password Requirements

- Minimum 8 characters
- Recommended: Mix of letters, numbers, symbols
- Users should change passwords regularly

### Changing Your Password

1. Click your username in the header
2. Select **Change Password**
3. Enter current and new password
4. Click **Update**

---

## Best Practices

### Tape Management

1. **Label everything**: Always label tapes before use
2. **Rotate pools**: Use DAILY/WEEKLY/MONTHLY rotation
3. **Store offsite**: Regularly move tapes offsite
4. **Track inventory**: Update status when tapes move
5. **Retire old tapes**: Replace tapes after recommended lifetime

### Backup Strategy

1. **3-2-1 Rule**: 3 copies, 2 media types, 1 offsite
2. **Test restores**: Regularly verify backups work
3. **Full + Incremental**: Monthly full, daily incremental
4. **Document sources**: Keep source configurations documented
5. **Backup the database**: Periodically backup TapeBackarr's database to tape

### Security

1. **Change default password** immediately
2. **Use strong passwords** for all accounts
3. **Limit admin access** to necessary users
4. **Review audit logs** regularly
5. **Secure the server** (firewall, updates)

### Monitoring

1. **Enable notifications**: Set up Telegram for alerts
2. **Check dashboard daily**: Look for warnings
3. **Monitor disk space**: Ensure database has space
4. **Review failed jobs**: Investigate and resolve issues

### Disaster Recovery

1. **Document recovery procedures** before you need them
2. **Keep a tape inventory** separate from the system
3. **Test full restores** periodically
4. **Have spare tapes** on hand
5. **Know your tape rotation** locations
6. **Backup the TapeBackarr database** to tape regularly

---

## Database Backup

TapeBackarr can backup its own database to tape for disaster recovery. This ensures you can recover your tape catalog and configuration even if the server is lost.

### Why Backup the Database?

The TapeBackarr database contains:
- File catalog with block offset information
- Tape inventory and pool assignments
- Backup job configurations
- User accounts
- Audit logs

Without this database, you would need to manually catalog each tape to know its contents.

### Backup Database to Tape

Via API:
```bash
curl -X POST http://localhost:8080/api/v1/database-backup/backup \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"tape_id": 1}'
```

### Restore Database from Tape

Via API:
```bash
curl -X POST http://localhost:8080/api/v1/database-backup/restore \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"backup_id": 1, "dest_path": "/tmp/restore"}'
```

### Best Practices for Database Backup

1. **Schedule regular backups**: Weekly or after major changes
2. **Use archive tapes**: Store database backups on long-term archive tapes
3. **Test restoration**: Periodically verify database backups can be restored
4. **Document the process**: Keep written instructions for database recovery

---

## In-App Documentation

TapeBackarr includes comprehensive documentation accessible directly from the web interface.

### Accessing Documentation

1. Click **Documentation** (📖) in the sidebar
2. Select a document from the list
3. Read the content directly in the browser

### Available Documents

| Document | Description |
|----------|-------------|
| Usage Guide | Complete guide to using TapeBackarr |
| API Reference | REST API documentation |
| Operator Guide | Quick reference for daily operations |
| Manual Recovery | Recover data without TapeBackarr |
| Architecture | System design and data flows |
| Database Schema | Database table definitions |
| Installation Guide | Installation instructions for all deployment methods |
| Proxmox Guide | Backup and restore Proxmox VMs and LXCs |

### Manual Recovery Guide

The **Manual Recovery** document is especially important for long-term archival. It contains step-by-step instructions for recovering data from tape using only standard Linux commands (mt, tar), without needing TapeBackarr.

This is useful when:
- The TapeBackarr server is unavailable
- Recovering data many years after archival
- Disaster recovery scenarios

---

## Troubleshooting

### Common Issues

**Tape not detected:**
```bash
# Check device exists
ls -la /dev/st* /dev/nst*

# Check permissions
groups $(whoami)  # Should include 'tape'

# Manual status check
mt -f /dev/nst0 status
```

**Backup fails to start:**
1. Check source path is accessible
2. Verify tape is loaded
3. Check tape is not write-protected
4. Review logs for specific error

**Restore not finding files:**
1. Verify backup completed successfully
2. Search with different patterns
3. Check if file was excluded

**Telegram notifications not working:**
1. Verify bot token is correct
2. Check chat ID is correct (include `-` for groups)
3. Ensure bot is added to the chat
4. Test with: `curl https://api.telegram.org/bot{YOUR_TOKEN}/getMe`

For additional help, check the system logs at `/var/log/tapebackarr/`.
