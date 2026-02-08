#!/usr/bin/env bash

# TapeBackarr Proxmox LXC Installation Script
# This script creates an LXC container on Proxmox and installs TapeBackarr
# Compatible with Proxmox VE 7.x and 8.x
#
# Usage: bash -c "$(wget -qLO - https://github.com/RoseOO/TapeBackarr/raw/main/deploy/proxmox-lxc-install.sh)"
#
# Requirements:
# - Proxmox VE 7.0 or later
# - Internet connection
# - Available LXC template (Debian 12 preferred)

set -e

# ============================================================================
# CONFIGURATION VARIABLES
# ============================================================================

# Container defaults (can be overridden by environment variables)
CT_ID="${CT_ID:-}"                          # Will auto-select next available if not set
CT_HOSTNAME="${CT_HOSTNAME:-tapebackarr}"
CT_MEMORY="${CT_MEMORY:-2048}"              # MB
CT_DISK="${CT_DISK:-8}"                     # GB
CT_CORES="${CT_CORES:-2}"
CT_NETWORK="${CT_NETWORK:-dhcp}"            # "dhcp" or static IP like "192.168.1.100/24"
CT_GATEWAY="${CT_GATEWAY:-}"                # Required if using static IP
CT_DNS="${CT_DNS:-}"                        # DNS server (optional)
CT_PASSWORD="${CT_PASSWORD:-}"              # Root password (will prompt if not set)
CT_STORAGE="${CT_STORAGE:-local-lvm}"       # Storage for container disk
CT_TEMPLATE="${CT_TEMPLATE:-}"              # Template to use (auto-select if not set)
TAPE_DEVICE="${TAPE_DEVICE:-/dev/nst0}"     # Tape device to pass through

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

msg_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
msg_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
msg_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
msg_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

header() {
    echo
    echo -e "${GREEN}================================================================${NC}"
    echo -e "${GREEN}   TapeBackarr LXC Container Installation for Proxmox VE${NC}"
    echo -e "${GREEN}================================================================${NC}"
    echo
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        msg_error "This script must be run as root on the Proxmox host"
    fi
}

check_proxmox() {
    if ! command -v pveversion &> /dev/null; then
        msg_error "This script must be run on a Proxmox VE host"
    fi
    PVE_VERSION=$(pveversion | cut -d/ -f2 | cut -d- -f1)
    msg_ok "Detected Proxmox VE $PVE_VERSION"
}

get_next_ct_id() {
    if [[ -n "$CT_ID" ]]; then
        if pct status "$CT_ID" &> /dev/null; then
            msg_error "Container ID $CT_ID already exists"
        fi
        echo "$CT_ID"
    else
        # Find next available ID starting from 100
        local id=100
        while pct status "$id" &> /dev/null 2>&1; do
            ((id++))
        done
        echo "$id"
    fi
}

select_template() {
    if [[ -n "$CT_TEMPLATE" ]]; then
        echo "$CT_TEMPLATE"
        return
    fi
    
    msg_info "Looking for Debian templates..."
    
    # List available templates and select the best one
    local templates
    templates=$(pveam list local 2>/dev/null | grep -E "debian-1[12]" | head -1 | awk '{print $1}')
    
    if [[ -z "$templates" ]]; then
        msg_info "No Debian template found. Downloading Debian 12..."
        pveam update
        
        # Get available Debian 12 templates dynamically from pveam available
        local available_template
        available_template=$(pveam available --section system 2>/dev/null | grep -E "debian-12-standard_[0-9].*_amd64" | head -1 | awk '{print $2}')
        
        if [[ -z "$available_template" ]]; then
            msg_error "Could not find a Debian 12 template in the available templates. Check your network connection and run 'pveam available --section system' to see available templates."
        fi
        
        msg_info "Downloading template: $available_template"
        pveam download local "$available_template"
        
        templates=$(pveam list local 2>/dev/null | grep "debian-12" | head -1 | awk '{print $1}')
    fi
    
    if [[ -z "$templates" ]]; then
        msg_error "Could not find or download a suitable Debian template"
    fi
    
    msg_ok "Using template: $templates"
    echo "$templates"
}

prompt_password() {
    if [[ -z "$CT_PASSWORD" ]]; then
        echo
        read -s -p "Enter root password for container: " CT_PASSWORD
        echo
        read -s -p "Confirm password: " CT_PASSWORD_CONFIRM
        echo
        if [[ "$CT_PASSWORD" != "$CT_PASSWORD_CONFIRM" ]]; then
            msg_error "Passwords do not match"
        fi
    fi
}

check_tape_device() {
    if [[ -e "$TAPE_DEVICE" ]]; then
        msg_ok "Tape device $TAPE_DEVICE found on host"
        TAPE_PASSTHROUGH=true
    else
        msg_warn "Tape device $TAPE_DEVICE not found on host"
        msg_warn "Container will be created without tape device passthrough"
        msg_warn "You can add it later by editing the container configuration"
        TAPE_PASSTHROUGH=false
    fi
}

# ============================================================================
# MAIN INSTALLATION FUNCTIONS
# ============================================================================

create_container() {
    local ct_id=$1
    local template=$2
    
    msg_info "Creating LXC container $ct_id..."
    
    # Build network configuration
    local net_config
    if [[ "$CT_NETWORK" == "dhcp" ]]; then
        net_config="name=eth0,bridge=vmbr0,ip=dhcp"
    else
        if [[ -z "$CT_GATEWAY" ]]; then
            msg_error "Gateway is required when using static IP"
        fi
        net_config="name=eth0,bridge=vmbr0,ip=$CT_NETWORK,gw=$CT_GATEWAY"
    fi
    
    # Create the container
    pct create "$ct_id" "local:vztmpl/$template" \
        --hostname "$CT_HOSTNAME" \
        --memory "$CT_MEMORY" \
        --cores "$CT_CORES" \
        --rootfs "$CT_STORAGE:$CT_DISK" \
        --net0 "$net_config" \
        --password "$CT_PASSWORD" \
        --unprivileged 0 \
        --features nesting=1 \
        --onboot 1 \
        --start 0
    
    msg_ok "Container $ct_id created"
}

configure_tape_passthrough() {
    local ct_id=$1
    
    if [[ "$TAPE_PASSTHROUGH" != "true" ]]; then
        return
    fi
    
    msg_info "Configuring tape device passthrough..."
    
    # Get the major/minor numbers for the tape device
    local major minor
    major=$(stat -c '%t' "$TAPE_DEVICE" 2>/dev/null | xargs printf "%d")
    minor=$(stat -c '%T' "$TAPE_DEVICE" 2>/dev/null | xargs printf "%d")
    
    if [[ -n "$major" && -n "$minor" ]]; then
        # Add device passthrough to container config
        cat >> "/etc/pve/lxc/$ct_id.conf" << EOF

# Tape device passthrough
lxc.cgroup2.devices.allow: c ${major}:* rwm
lxc.mount.entry: /dev/nst0 dev/nst0 none bind,optional,create=file
lxc.mount.entry: /dev/st0 dev/st0 none bind,optional,create=file
EOF
        
        # Also try to passthrough the SCSI generic device if available
        local sg_device
        sg_device=$(ls -la /sys/class/scsi_tape/nst0/device/scsi_generic/ 2>/dev/null | grep sg | awk '{print $9}')
        if [[ -n "$sg_device" ]]; then
            local sg_major sg_minor
            sg_major=$(stat -c '%t' "/dev/$sg_device" 2>/dev/null | xargs printf "%d")
            sg_minor=$(stat -c '%T' "/dev/$sg_device" 2>/dev/null | xargs printf "%d")
            if [[ -n "$sg_major" ]]; then
                echo "lxc.cgroup2.devices.allow: c ${sg_major}:${sg_minor} rwm" >> "/etc/pve/lxc/$ct_id.conf"
                echo "lxc.mount.entry: /dev/$sg_device dev/$sg_device none bind,optional,create=file" >> "/etc/pve/lxc/$ct_id.conf"
            fi
        fi
        
        msg_ok "Tape device passthrough configured"
    else
        msg_warn "Could not determine tape device major/minor numbers"
    fi
}

start_container() {
    local ct_id=$1
    
    msg_info "Starting container $ct_id..."
    pct start "$ct_id"
    
    # Wait for container to be fully up
    local count=0
    while ! pct exec "$ct_id" -- systemctl is-system-running &>/dev/null; do
        sleep 1
        ((count++))
        if [[ $count -ge 60 ]]; then
            msg_warn "Container took too long to start, continuing anyway..."
            break
        fi
    done
    
    msg_ok "Container started"
}

install_tapebackarr() {
    local ct_id=$1
    
    msg_info "Installing TapeBackarr in container..."
    
    # Update and install dependencies
    pct exec "$ct_id" -- bash -c "
        export DEBIAN_FRONTEND=noninteractive
        apt-get update -qq
        apt-get upgrade -y -qq
        apt-get install -y -qq wget curl git mt-st tar mbuffer sg3-utils
    "
    msg_ok "Dependencies installed"
    
    # Install Go
    msg_info "Installing Go..."
    pct exec "$ct_id" -- bash -c "
        wget -q https://go.dev/dl/go1.22.0.linux-amd64.tar.gz -O /tmp/go.tar.gz
        rm -rf /usr/local/go
        tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
        echo 'export PATH=\$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
    "
    msg_ok "Go installed"
    
    # Install Node.js
    msg_info "Installing Node.js..."
    pct exec "$ct_id" -- bash -c "
        curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
        apt-get install -y -qq nodejs
    "
    msg_ok "Node.js installed"
    
    # Clone and build TapeBackarr
    msg_info "Building TapeBackarr (this may take a few minutes)..."
    pct exec "$ct_id" -- bash -c "
        export PATH=\$PATH:/usr/local/go/bin
        cd /opt
        git clone https://github.com/RoseOO/TapeBackarr.git
        cd TapeBackarr
        go build -o tapebackarr ./cmd/tapebackarr
        cd web/frontend
        npm install
        npm run build
    "
    msg_ok "TapeBackarr built"
    
    # Run the install script
    msg_info "Running installation script..."
    pct exec "$ct_id" -- bash -c "
        cd /opt/TapeBackarr
        ./deploy/install.sh
    "
    msg_ok "TapeBackarr installed"
    
    # Start the service
    msg_info "Starting TapeBackarr service..."
    pct exec "$ct_id" -- systemctl start tapebackarr
    pct exec "$ct_id" -- systemctl enable tapebackarr
    msg_ok "TapeBackarr service started"
}

show_summary() {
    local ct_id=$1
    
    # Get the container IP
    local ip_addr
    ip_addr=$(pct exec "$ct_id" -- hostname -I 2>/dev/null | awk '{print $1}')
    
    echo
    echo -e "${GREEN}================================================================${NC}"
    echo -e "${GREEN}   TapeBackarr Installation Complete!${NC}"
    echo -e "${GREEN}================================================================${NC}"
    echo
    echo -e "  Container ID:     ${BLUE}$ct_id${NC}"
    echo -e "  Hostname:         ${BLUE}$CT_HOSTNAME${NC}"
    echo -e "  IP Address:       ${BLUE}${ip_addr:-DHCP (check container)}${NC}"
    echo
    echo -e "  ${GREEN}Web Interface:${NC}"
    echo -e "    http://${ip_addr:-<container-ip>}:8080"
    echo
    echo -e "  ${GREEN}Default Credentials:${NC}"
    echo -e "    Username: ${BLUE}admin${NC}"
    echo -e "    Password: ${BLUE}changeme${NC}"
    echo
    echo -e "  ${YELLOW}⚠️  IMPORTANT: Change the default password immediately!${NC}"
    echo
    if [[ "$TAPE_PASSTHROUGH" == "true" ]]; then
        echo -e "  ${GREEN}Tape Device:${NC} $TAPE_DEVICE passed through to container"
    else
        echo -e "  ${YELLOW}Tape Device:${NC} Not configured. Add manually if needed."
    fi
    echo
    echo -e "  ${GREEN}Useful Commands:${NC}"
    echo -e "    Container console: ${BLUE}pct enter $ct_id${NC}"
    echo -e "    View logs:         ${BLUE}pct exec $ct_id -- journalctl -u tapebackarr -f${NC}"
    echo -e "    Restart service:   ${BLUE}pct exec $ct_id -- systemctl restart tapebackarr${NC}"
    echo
    echo -e "  ${GREEN}Configuration File:${NC}"
    echo -e "    /etc/tapebackarr/config.json (inside container)"
    echo
    echo -e "${GREEN}================================================================${NC}"
    echo
}

# ============================================================================
# MAIN SCRIPT
# ============================================================================

main() {
    header
    check_root
    check_proxmox
    
    # Get container ID
    CT_ID=$(get_next_ct_id)
    msg_ok "Using container ID: $CT_ID"
    
    # Select template
    CT_TEMPLATE=$(select_template)
    
    # Check for tape device
    check_tape_device
    
    # Prompt for password if not set
    prompt_password
    
    # Create the container
    create_container "$CT_ID" "$CT_TEMPLATE"
    
    # Configure tape passthrough
    configure_tape_passthrough "$CT_ID"
    
    # Start the container
    start_container "$CT_ID"
    
    # Install TapeBackarr
    install_tapebackarr "$CT_ID"
    
    # Show summary
    show_summary "$CT_ID"
}

# Handle script being sourced vs executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
