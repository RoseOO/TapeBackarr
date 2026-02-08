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
8. [Telegram Notifications](#telegram-notifications)
9. [Viewing Logs](#viewing-logs)
10. [User Management](#user-management)
11. [Best Practices](#best-practices)

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
| Sources | Configure backup source paths |
| Jobs | Create and manage backup jobs |
| Restore | Browse catalog and restore files |
| Logs | View operation logs and audit trail |
| Users | Manage user accounts (admin only) |

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

The label format is: `TAPEBACKARR|label|timestamp`

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
blank → active → full → retired
                   ↓
                offsite
```

- **blank**: New tape, never written
- **active**: In use, has space remaining
- **full**: Tape is full
- **retired**: No longer in use
- **offsite**: Stored at an offsite location

### Marking Tapes

- **Mark as Offsite**: When tape is moved to offsite storage
- **Mark as Returned**: When tape returns from offsite
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
| Overwrite | Replace existing files |
| Skip Existing | Don't overwrite existing files |
| Verify | Verify checksums after restore |

### Restore Process

1. **Select files** from the catalog
2. **Review required tapes** - system shows which tapes are needed
3. **Insert first tape** as directed
4. **Start restore** - files are extracted to the destination
5. **Change tapes** if prompted (for multi-tape restores)
6. **Verify** - optionally verify restored file checksums

### Restore Single File

1. Search for the file in the catalog
2. Click on the file name
3. Click **Restore File**
4. Choose destination
5. Insert required tape when prompted
6. File is restored

---

## Telegram Notifications

TapeBackarr can send notifications to Telegram when operator action is required.

### Setting Up Telegram Bot

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
