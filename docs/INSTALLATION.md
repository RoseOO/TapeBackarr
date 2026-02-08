# TapeBackarr Installation Guide

This guide covers all installation methods for TapeBackarr.

## Table of Contents

1. [Quick Install (Script)](#quick-install-script)
2. [Manual Installation](#manual-installation)
3. [Docker Installation](#docker-installation)
4. [Proxmox LXC Installation](#proxmox-lxc-installation)
5. [Post-Installation Setup](#post-installation-setup)
6. [Upgrading](#upgrading)
7. [Uninstalling](#uninstalling)

---

## Quick Install (Script)

The fastest way to install TapeBackarr on a Debian-based system:

```bash
# Clone repository
git clone https://github.com/RoseOO/TapeBackarr.git
cd TapeBackarr

# Build (requires Go 1.21+ and Node.js 18+)
make build

# Run installer
sudo ./deploy/install.sh
```

The installer will:
- Install system dependencies (mt-st, tar, mbuffer)
- Create directories and set permissions
- Install the binary to `/opt/tapebackarr`
- Generate a secure JWT secret
- Create and enable the systemd service

---

## Manual Installation

### Prerequisites

- Debian 12+ or Ubuntu 22.04+
- Go 1.21+
- Node.js 18+
- Root access

### Step 1: Install System Dependencies

```bash
sudo apt update
sudo apt install -y mt-st tar mbuffer sg3-utils
```

### Step 2: Build from Source

```bash
# Clone
git clone https://github.com/RoseOO/TapeBackarr.git
cd TapeBackarr

# Build backend
go build -o tapebackarr ./cmd/tapebackarr

# Build frontend
cd web/frontend
npm install
npm run build
cd ../..
```

### Step 3: Create Directories

```bash
sudo mkdir -p /opt/tapebackarr
sudo mkdir -p /etc/tapebackarr
sudo mkdir -p /var/lib/tapebackarr
sudo mkdir -p /var/log/tapebackarr
```

### Step 4: Install Files

```bash
# Copy binary
sudo cp tapebackarr /opt/tapebackarr/

# Copy frontend
sudo cp -r web/frontend/build /opt/tapebackarr/static

# Copy configuration
sudo cp deploy/config.example.json /etc/tapebackarr/config.json
sudo chmod 600 /etc/tapebackarr/config.json

# Copy systemd service
sudo cp deploy/tapebackarr.service /etc/systemd/system/
```

### Step 5: Configure

Edit `/etc/tapebackarr/config.json`:

```bash
sudo nano /etc/tapebackarr/config.json
```

**Important settings:**
- Set a secure `jwt_secret` (at least 32 random characters)
- Configure your tape drive(s) in the `tape.drives` section
- Update paths as needed

### Step 6: Start the Service

```bash
sudo systemctl daemon-reload
sudo systemctl enable tapebackarr
sudo systemctl start tapebackarr
```

### Step 7: Verify

```bash
sudo systemctl status tapebackarr
curl http://localhost:8080/api/v1/health
```

---

## Docker Installation

### Prerequisites

- Docker 20.10+
- Docker Compose 2.0+
- Access to tape devices (privileged mode)

### Quick Start

1. **Clone the repository:**
   ```bash
   git clone https://github.com/RoseOO/TapeBackarr.git
   cd TapeBackarr
   ```

2. **Create configuration:**
   ```bash
   cp deploy/config.example.json config.json
   # Edit config.json with your settings
   nano config.json
   ```

3. **Start with Docker Compose:**
   ```bash
   docker compose up -d
   ```

4. **Access the web interface:**
   Open `http://your-server:8080`

### Building the Docker Image

```bash
docker build -t tapebackarr:latest .
```

### Running Without Compose

```bash
docker run -d \
  --name tapebackarr \
  --privileged \
  -p 8080:8080 \
  -v $(pwd)/config.json:/etc/tapebackarr/config.json:ro \
  -v tapebackarr-data:/var/lib/tapebackarr \
  -v tapebackarr-logs:/var/log/tapebackarr \
  tapebackarr:latest
```

### Device Passthrough (Alternative to Privileged)

If you prefer not to use privileged mode, you can pass through specific devices:

```bash
docker run -d \
  --name tapebackarr \
  --device /dev/st0:/dev/st0 \
  --device /dev/nst0:/dev/nst0 \
  --device /dev/sg0:/dev/sg0 \
  -p 8080:8080 \
  -v $(pwd)/config.json:/etc/tapebackarr/config.json:ro \
  -v tapebackarr-data:/var/lib/tapebackarr \
  tapebackarr:latest
```

### Docker Compose Configuration

See `docker-compose.yml` for the full configuration. Key settings:

```yaml
services:
  tapebackarr:
    build: .
    privileged: true  # Required for tape access
    ports:
      - "8080:8080"
    volumes:
      - ./config.json:/etc/tapebackarr/config.json:ro
      - tapebackarr-data:/var/lib/tapebackarr
      - tapebackarr-logs:/var/log/tapebackarr
      # Mount backup source directories
      - /mnt/nfs:/mnt/nfs:ro
```

### Health Check

The Docker image includes a health check. Verify with:

```bash
docker inspect --format='{{.State.Health.Status}}' tapebackarr
```

---

## Proxmox LXC Installation

TapeBackarr can be installed in a Proxmox LXC container with tape device passthrough. This is ideal for Proxmox users who want to manage tape backups alongside their VMs.

### Automated Installation (Recommended)

Run this command on your **Proxmox host**:

```bash
bash -c "$(wget -qLO - https://github.com/RoseOO/TapeBackarr/raw/main/deploy/proxmox-lxc-install.sh)"
```

This will:
- Create a new LXC container (Debian 12)
- Configure tape device passthrough
- Install TapeBackarr and all dependencies
- Start the service

### Configuration Options

You can customize the installation with environment variables:

```bash
# Custom container settings
CT_ID=200 \
CT_HOSTNAME=tapebackarr \
CT_MEMORY=4096 \
CT_DISK=16 \
CT_CORES=4 \
CT_NETWORK="192.168.1.100/24" \
CT_GATEWAY="192.168.1.1" \
CT_STORAGE="local-lvm" \
TAPE_DEVICE="/dev/nst0" \
bash -c "$(wget -qLO - https://github.com/RoseOO/TapeBackarr/raw/main/deploy/proxmox-lxc-install.sh)"
```

| Variable | Default | Description |
|----------|---------|-------------|
| `CT_ID` | Auto | Container ID |
| `CT_HOSTNAME` | `tapebackarr` | Container hostname |
| `CT_MEMORY` | `2048` | Memory in MB |
| `CT_DISK` | `8` | Disk size in GB |
| `CT_CORES` | `2` | CPU cores |
| `CT_NETWORK` | `dhcp` | Network config (or static IP) |
| `CT_GATEWAY` | - | Gateway (required for static IP) |
| `CT_STORAGE` | `local-lvm` | Storage for container |
| `TAPE_DEVICE` | `/dev/nst0` | Tape device to passthrough |

### Manual LXC Setup

If you prefer manual setup:

1. **Create container:**
   
   First, find an available Debian 12 template:
   ```bash
   pveam update
   pveam available --section system | grep debian-12
   ```
   
   Download and create the container (replace `<template>` with the actual template name from the previous command):
   ```bash
   pveam download local <template>
   pct create 200 local:vztmpl/<template> \
     --hostname tapebackarr \
     --memory 2048 \
     --cores 2 \
     --rootfs local-lvm:8 \
     --net0 name=eth0,bridge=vmbr0,ip=dhcp \
     --unprivileged 0 \
     --features nesting=1
   ```

2. **Configure tape passthrough** (add to `/etc/pve/lxc/200.conf`):
   ```
   lxc.cgroup2.devices.allow: c 9:* rwm
   lxc.mount.entry: /dev/nst0 dev/nst0 none bind,optional,create=file
   lxc.mount.entry: /dev/st0 dev/st0 none bind,optional,create=file
   ```

3. **Start container and install:**
   ```bash
   pct start 200
   pct enter 200
   
   # Inside container:
   apt update && apt install -y wget curl git mt-st tar mbuffer
   # ... follow manual installation steps
   ```

### Tape Device Passthrough

For tape access in an LXC container, the container must be **privileged** and have device access configured. The installation script handles this automatically.

To verify tape access inside the container:

```bash
pct exec 200 -- mt -f /dev/nst0 status
```

### Proxmox Integration

TapeBackarr can also backup Proxmox VMs and LXCs. Configure the Proxmox integration in `config.json`:

```json
{
  "proxmox": {
    "enabled": true,
    "host": "192.168.1.1",
    "port": 8006,
    "token_id": "root@pam!tapebackarr",
    "token_secret": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }
}
```

See [PROXMOX_GUIDE.md](PROXMOX_GUIDE.md) for detailed Proxmox integration documentation.

---

## Post-Installation Setup

### 1. Access the Web Interface

Open `http://your-server:8080` in a browser.

### 2. Login

Default credentials:
- **Username:** `admin`
- **Password:** `changeme`

### 3. Change the Default Password

⚠️ **Important:** Change the default password immediately!

1. Go to Users in the sidebar
2. Select the admin user
3. Change the password

### 4. Configure Tape Drives

1. Go to **Drives** in the sidebar
2. Verify your drives are detected
3. Add or configure drives as needed

### 5. Create Tape Pools

1. Go to **Tapes** → **Pools**
2. Create pools (DAILY, WEEKLY, MONTHLY, ARCHIVE)
3. Set retention policies

### 6. Register Tapes

1. Go to **Tapes**
2. Click **Add Tape**
3. Enter barcode, label, and assign to pool

### 7. Configure Notifications (Optional)

Set up Telegram notifications:

```json
{
  "notifications": {
    "telegram": {
      "enabled": true,
      "bot_token": "your-bot-token",
      "chat_id": "your-chat-id"
    }
  }
}
```

Restart TapeBackarr after changing configuration.

---

## Upgrading

### Script Installation / Manual

1. **Stop the service:**
   ```bash
   sudo systemctl stop tapebackarr
   ```

2. **Backup the database:**
   ```bash
   sudo sqlite3 /var/lib/tapebackarr/tapebackarr.db ".backup /tmp/tapebackarr-backup.db"
   ```

3. **Update the code:**
   ```bash
   cd TapeBackarr
   git pull
   make build
   ```

4. **Install the new version:**
   ```bash
   sudo cp tapebackarr /opt/tapebackarr/
   sudo cp -r web/frontend/build /opt/tapebackarr/static
   ```

5. **Start the service:**
   ```bash
   sudo systemctl start tapebackarr
   ```

### Docker

```bash
cd TapeBackarr
git pull
docker compose build
docker compose up -d
```

### Proxmox LXC

1. **Enter the container:**
   ```bash
   pct enter 200
   ```

2. **Stop the service:**
   ```bash
   systemctl stop tapebackarr
   ```

3. **Backup the database:**
   ```bash
   sqlite3 /var/lib/tapebackarr/tapebackarr.db ".backup /tmp/tapebackarr-backup.db"
   ```

4. **Update the code:**
   ```bash
   cd /opt/tapebackarr/src/TapeBackarr
   git pull
   make build
   ```

   If the source was not cloned during install, clone it first:
   ```bash
   apt install -y git golang-go nodejs npm
   git clone https://github.com/RoseOO/TapeBackarr.git /opt/tapebackarr/src/TapeBackarr
   cd /opt/tapebackarr/src/TapeBackarr
   make build
   ```

5. **Install the new version:**
   ```bash
   cp tapebackarr /opt/tapebackarr/tapebackarr
   cp -r web/frontend/build /opt/tapebackarr/static
   ```

6. **Start the service:**
   ```bash
   systemctl start tapebackarr
   ```

7. **Verify the update:**
   ```bash
   systemctl status tapebackarr
   curl http://localhost:8080/api/v1/health
   ```

---

## Uninstalling

### Script Installation / Manual

```bash
# Stop and disable service
sudo systemctl stop tapebackarr
sudo systemctl disable tapebackarr

# Remove files
sudo rm -rf /opt/tapebackarr
sudo rm /etc/systemd/system/tapebackarr.service
sudo systemctl daemon-reload

# Optional: Remove data (WARNING: destroys database!)
# sudo rm -rf /var/lib/tapebackarr
# sudo rm -rf /var/log/tapebackarr
# sudo rm -rf /etc/tapebackarr
```

### Docker

```bash
docker compose down
docker volume rm tapebackarr-data tapebackarr-logs
```

### Proxmox LXC

```bash
pct stop 200
pct destroy 200
```

---

## Troubleshooting

### Service Won't Start

Check the logs:
```bash
sudo journalctl -u tapebackarr -f
cat /var/log/tapebackarr/tapebackarr.log
```

### Tape Device Not Found

1. Check if device exists:
   ```bash
   ls -la /dev/st* /dev/nst*
   ```

2. Check permissions:
   ```bash
   groups $(whoami)  # Should include 'tape'
   ```

3. Try manual status:
   ```bash
   mt -f /dev/nst0 status
   ```

### Permission Denied

Ensure the user is in the `tape` group or run as root:
```bash
sudo usermod -aG tape $(whoami)
# Log out and back in
```

### Docker Can't Access Tape

Use `--privileged` mode or pass through specific devices:
```bash
--device /dev/nst0:/dev/nst0
```

---

## Getting Help

- [GitHub Issues](https://github.com/RoseOO/TapeBackarr/issues)
- [Documentation](docs/)
- [API Reference](docs/API_REFERENCE.md)
