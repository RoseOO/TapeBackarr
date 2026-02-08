# Proxmox VE Integration Guide

TapeBackarr supports direct backup and restore of Proxmox VE virtual machines (QEMU/KVM) and LXC containers to tape storage. This guide covers setup, configuration, and usage of the Proxmox integration.

## Table of Contents

1. [Overview](#overview)
2. [Requirements](#requirements)
3. [Configuration](#configuration)
4. [Authentication](#authentication)
5. [Backup Operations](#backup-operations)
6. [Restore Operations](#restore-operations)
7. [Scheduled Jobs](#scheduled-jobs)
8. [API Reference](#api-reference)
9. [Best Practices](#best-practices)
10. [Troubleshooting](#troubleshooting)

## Overview

The Proxmox integration allows you to:

- **Discover** all VMs and LXC containers across standalone nodes or clusters
- **Backup** individual guests or all guests to tape with full configuration
- **Restore** guests from tape to the same or different nodes
- **Schedule** automated backups with retention policies
- **Preserve** complete guest configuration for disaster recovery

### Architecture

```
┌─────────────────────┐     ┌─────────────────────┐
│   Proxmox VE        │     │    TapeBackarr      │
│   ┌─────────────┐   │     │   ┌─────────────┐   │
│   │   VM 100    │   │◄────┤   │   Proxmox   │   │
│   │   VM 101    │   │     │   │   Client    │   │
│   │   LXC 200   │   │     │   └─────────────┘   │
│   │   LXC 201   │   │           │               │
│   └─────────────┘   │           ▼               │
│                     │     ┌─────────────────┐   │
│   vzdump / qmrestore│     │ Backup Service  │   │
└─────────────────────┘     │ Restore Service │   │
                            └────────┬────────┘   │
                                     │            │
                            ┌────────▼────────┐   │
                            │  Tape Service   │   │
                            │  (/dev/nst0)    │   │
                            └─────────────────┘   │
                            └─────────────────────┘
```

### Supported Configurations

| Configuration | Support |
|--------------|---------|
| Standalone Proxmox node | ✅ Full support |
| Proxmox cluster | ✅ Full support |
| QEMU/KVM VMs | ✅ Full support |
| LXC containers | ✅ Full support |
| VM templates | ⚠️ Skipped by default |
| Mounted disks | ✅ Included in backup |
| VM configuration | ✅ Saved to database |
| High Availability | ✅ HA state preserved |

## Requirements

### On TapeBackarr Server

1. **Network access** to Proxmox API (port 8006 by default)
2. **vzdump** and **qmrestore** utilities installed (if running on Proxmox host)
3. **Tape drive** configured and accessible

### On Proxmox VE

1. Proxmox VE 7.0 or newer
2. API access enabled (default)
3. Valid credentials (user/password or API token)

### Permissions Required

The Proxmox user/token needs these permissions:

| Permission | Purpose |
|------------|---------|
| `VM.Backup` | Create vzdump backups |
| `VM.Config.Backup` | Access backup configuration |
| `Datastore.Allocate` | Write to storage (for restore) |
| `VM.Allocate` | Create new VMs (for restore) |
| `Sys.Audit` | View cluster/node status |

For full functionality, we recommend using the `PVEAdmin` role or creating a custom role with the permissions above.

## Configuration

### Basic Configuration

Add the Proxmox section to your `/etc/tapebackarr/config.json`:

```json
{
  "proxmox": {
    "enabled": true,
    "host": "192.168.1.100",
    "port": 8006,
    "skip_tls_verify": true,
    "username": "root",
    "password": "your-password",
    "realm": "pam",
    "default_mode": "snapshot",
    "default_compress": "zstd",
    "temp_dir": "/var/lib/tapebackarr/proxmox-tmp"
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable Proxmox integration |
| `host` | string | - | Proxmox host IP or hostname |
| `port` | integer | `8006` | Proxmox API port |
| `skip_tls_verify` | boolean | `true` | Skip SSL certificate verification |
| `username` | string | - | Proxmox username (without @realm) |
| `password` | string | - | User password |
| `realm` | string | `pam` | Authentication realm (`pam`, `pve`) |
| `token_id` | string | - | API token ID (alternative auth) |
| `token_secret` | string | - | API token secret |
| `default_mode` | string | `snapshot` | Default backup mode |
| `default_compress` | string | `zstd` | Default compression |
| `temp_dir` | string | `/var/lib/tapebackarr/proxmox-tmp` | Temporary directory |

### Backup Modes

| Mode | Description | Downtime |
|------|-------------|----------|
| `snapshot` | Live snapshot backup (recommended) | None |
| `suspend` | Suspend VM during backup | Brief |
| `stop` | Stop VM during backup | Full |

### Compression Options

| Option | Speed | Ratio | Recommended For |
|--------|-------|-------|-----------------|
| `zstd` | Fast | High | Most use cases |
| `lzo` | Fastest | Lower | Large VMs |
| `gzip` | Slow | Good | Compatibility |
| *(empty)* | N/A | None | Maximum speed |

## Authentication

### Option 1: Username/Password

```json
{
  "proxmox": {
    "username": "root",
    "password": "your-secure-password",
    "realm": "pam"
  }
}
```

Common realms:
- `pam` - Linux PAM (system users like root)
- `pve` - Proxmox VE authentication server

### Option 2: API Token (Recommended)

1. Create an API token in Proxmox:
   - Go to Datacenter → Permissions → API Tokens
   - Add → Select user, token name, and uncheck "Privilege Separation"
   
2. Configure TapeBackarr:
```json
{
  "proxmox": {
    "token_id": "root@pam!tapebackarr",
    "token_secret": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }
}
```

**Benefits of API tokens:**
- No password stored in config
- Token can be revoked independently
- Audit trail in Proxmox

## Backup Operations

### Backup Single Guest

**API Endpoint:** `POST /api/v1/proxmox/backups`

```bash
curl -X POST http://localhost:8080/api/v1/proxmox/backups \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "node": "pve1",
    "vmid": 100,
    "guest_type": "qemu",
    "guest_name": "web-server",
    "backup_mode": "snapshot",
    "compress": "zstd",
    "tape_id": 1,
    "notes": "Weekly backup"
  }'
```

### Backup All Guests

**API Endpoint:** `POST /api/v1/proxmox/backups/all`

```bash
curl -X POST http://localhost:8080/api/v1/proxmox/backups/all \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "node": "",
    "tape_id": 1,
    "mode": "snapshot",
    "compress": "zstd"
  }'
```

**Parameters:**
- `node`: Empty string = backup all nodes
- `tape_id`: Target tape (must be loaded)
- `mode`: Backup mode (snapshot/suspend/stop)
- `compress`: Compression algorithm

### What Gets Backed Up

Each Proxmox backup includes:

1. **Guest Configuration**
   - CPU, memory, disk settings
   - Network configuration
   - Boot order and options
   - Cloud-init settings
   - Custom options

2. **Disk Images**
   - All attached disks
   - EFI disk (for UEFI VMs)
   - TPM state (if present)

3. **LXC-Specific**
   - Mount points
   - Container features
   - AppArmor profile

### Backup Process

1. TapeBackarr connects to Proxmox API
2. Retrieves guest configuration (saved to database)
3. Writes metadata to tape
4. Executes vzdump with `--stdout` flag
5. Streams vzdump output directly to tape
6. Writes file mark to separate backups
7. Updates database with backup details

## Restore Operations

### Plan Restore

Before restoring, check which tapes are needed:

**API Endpoint:** `POST /api/v1/proxmox/restores/plan`

```bash
curl -X POST http://localhost:8080/api/v1/proxmox/restores/plan \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "backup_id": 5
  }'
```

**Response:**
```json
{
  "required_tapes": [
    {
      "tape_id": 1,
      "barcode": "LTO001",
      "label": "DAILY-001",
      "status": "active",
      "total_bytes": 10737418240,
      "order": 1
    }
  ]
}
```

### Restore Guest

**API Endpoint:** `POST /api/v1/proxmox/restores`

```bash
curl -X POST http://localhost:8080/api/v1/proxmox/restores \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "backup_id": 5,
    "target_node": "pve2",
    "target_vmid": 200,
    "target_name": "web-server-restored",
    "storage": "local-lvm",
    "start_after": false,
    "overwrite": false
  }'
```

**Parameters:**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `backup_id` | Yes | ID of the backup to restore |
| `target_node` | No | Node to restore to (default: original) |
| `target_vmid` | No | New VMID (default: original) |
| `target_name` | No | New name (default: original) |
| `storage` | Yes | Target storage for disks |
| `start_after` | No | Start guest after restore |
| `overwrite` | No | Overwrite if VMID exists |

### Restore to Different Node

To restore a VM to a different node (disaster recovery):

```bash
curl -X POST http://localhost:8080/api/v1/proxmox/restores \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "backup_id": 5,
    "target_node": "pve-backup",
    "target_vmid": 9100,
    "storage": "local-zfs",
    "start_after": true
  }'
```

## Scheduled Jobs

### Create Backup Job

**API Endpoint:** `POST /api/v1/proxmox/jobs`

```bash
curl -X POST http://localhost:8080/api/v1/proxmox/jobs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Daily Proxmox Backup",
    "description": "Backup all VMs and containers nightly",
    "node": "",
    "guest_type_filter": "all",
    "pool_id": 1,
    "backup_mode": "snapshot",
    "compress": "zstd",
    "schedule_cron": "0 0 2 * * *",
    "retention_days": 30,
    "enabled": true
  }'
```

### Cron Schedule Format

TapeBackarr uses 6-field cron expressions with seconds:

```
┌──────────── second (0-59)
│ ┌────────── minute (0-59)
│ │ ┌──────── hour (0-23)
│ │ │ ┌────── day of month (1-31)
│ │ │ │ ┌──── month (1-12)
│ │ │ │ │ ┌── day of week (0-6, Sunday=0)
│ │ │ │ │ │
* * * * * *
```

**Examples:**
- `0 0 2 * * *` - Daily at 2:00 AM
- `0 0 3 * * 0` - Weekly on Sunday at 3:00 AM
- `0 0 4 1 * *` - Monthly on the 1st at 4:00 AM

### Filter Options

| Filter | Description | Example |
|--------|-------------|---------|
| `node` | Backup specific node only | `"pve1"` |
| `vmid_filter` | JSON array of VMIDs | `"[100, 101, 200]"` |
| `guest_type_filter` | Type filter | `"qemu"`, `"lxc"`, `"all"` |
| `tag_filter` | Match guests by tags | `"production,critical"` |

### Run Job Manually

**API Endpoint:** `POST /api/v1/proxmox/jobs/{id}/run`

```bash
curl -X POST http://localhost:8080/api/v1/proxmox/jobs/1/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tape_id": 1
  }'
```

## API Reference

### Discovery Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/proxmox/nodes` | List all Proxmox nodes |
| GET | `/api/v1/proxmox/guests` | List all VMs and LXCs |
| GET | `/api/v1/proxmox/guests/{vmid}` | Get guest details |
| GET | `/api/v1/proxmox/guests/{vmid}/config` | Get guest configuration |
| GET | `/api/v1/proxmox/cluster/status` | Get cluster status |

### Backup Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/proxmox/backups` | List all backups |
| GET | `/api/v1/proxmox/backups/{id}` | Get backup details |
| POST | `/api/v1/proxmox/backups` | Create single backup |
| POST | `/api/v1/proxmox/backups/all` | Backup all guests |

### Restore Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/proxmox/restores` | List all restores |
| POST | `/api/v1/proxmox/restores` | Start restore |
| POST | `/api/v1/proxmox/restores/plan` | Plan restore (get tapes) |

### Job Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/proxmox/jobs` | List all jobs |
| POST | `/api/v1/proxmox/jobs` | Create job |
| GET | `/api/v1/proxmox/jobs/{id}` | Get job details |
| PUT | `/api/v1/proxmox/jobs/{id}` | Update job |
| DELETE | `/api/v1/proxmox/jobs/{id}` | Delete job |
| POST | `/api/v1/proxmox/jobs/{id}/run` | Run job manually |

## Best Practices

### Backup Strategy

1. **Daily Incremental** - Snapshot mode for minimal impact
2. **Weekly Full** - Stop mode for consistency (maintenance window)
3. **Monthly Archive** - Full backup to offsite tape

### Tape Rotation

```
Week 1: Tape A (Mon-Fri daily)
Week 2: Tape B (Mon-Fri daily)
Week 3: Tape C (Mon-Fri daily)
Week 4: Tape D (Mon-Fri daily)
Monthly: Tape M1, M2, etc. (offsite)
```

### Performance Tips

1. **Use zstd compression** - Best speed/ratio balance
2. **Snapshot mode** - No VM downtime
3. **Schedule during low-usage periods**
4. **Use API tokens** - Faster than ticket auth
5. **Dedicated network** - Separate backup traffic

### Security

1. **Use API tokens** instead of passwords
2. **Minimal permissions** - Only what's needed
3. **TLS verification** in production
4. **Regular token rotation**
5. **Audit log review**

## Troubleshooting

### Connection Issues

**Error:** `authentication failed`
- Verify username/password or API token
- Check realm (pam vs pve)
- Ensure user has login permission

**Error:** `certificate verify failed`
- Set `skip_tls_verify: true` or
- Add Proxmox CA certificate to system trust

### Backup Issues

**Error:** `vzdump failed: permission denied`
- User needs `VM.Backup` permission
- Check vzdump executable permissions

**Error:** `no available tape in pool`
- Load tape in drive
- Check tape pool assignment
- Verify tape status (not retired/full)

### Restore Issues

**Error:** `VMID already exists`
- Use `overwrite: true` or
- Specify different `target_vmid`

**Error:** `storage not found`
- Verify storage name on target node
- Check storage permissions

### Logs

Check logs for detailed error information:

```bash
# TapeBackarr logs
tail -f /var/log/tapebackarr/tapebackarr.log | jq

# Proxmox task logs (on Proxmox host)
cat /var/log/pve/tasks/*/vzdump-*
```

### Debug Mode

Enable debug logging in config:

```json
{
  "logging": {
    "level": "debug"
  }
}
```

## Example: Complete Disaster Recovery

### 1. Backup Production Cluster

```bash
# Create backup job for all production VMs
curl -X POST http://localhost:8080/api/v1/proxmox/jobs \
  -d '{
    "name": "Production DR Backup",
    "tag_filter": "production",
    "backup_mode": "snapshot",
    "schedule_cron": "0 0 1 * * *",
    "enabled": true
  }'
```

### 2. Store Tape Offsite

After backup completes:
1. Eject tape via API
2. Store in secure offsite location
3. Document tape barcode and backup date

### 3. Disaster Recovery

On recovery site:

```bash
# 1. Load tape
# 2. Find backup
curl http://localhost:8080/api/v1/proxmox/backups?limit=100

# 3. Plan restore
curl -X POST http://localhost:8080/api/v1/proxmox/restores/plan \
  -d '{"backup_id": 42}'

# 4. Restore to DR node
curl -X POST http://localhost:8080/api/v1/proxmox/restores \
  -d '{
    "backup_id": 42,
    "target_node": "dr-node",
    "storage": "local-lvm",
    "start_after": true
  }'
```

---

For additional help, see:
- [Architecture Guide](ARCHITECTURE.md)
- [API Reference](API_REFERENCE.md)
- [Operator Guide](OPERATOR_GUIDE.md)
