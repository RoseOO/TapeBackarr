#!/bin/bash

# TapeBackarr Installation Script
# This script installs and configures TapeBackarr as a systemd service on Debian-based systems

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default paths
INSTALL_DIR="/opt/tapebackarr"
CONFIG_DIR="/etc/tapebackarr"
DATA_DIR="/var/lib/tapebackarr"
LOG_DIR="/var/log/tapebackarr"
SERVICE_FILE="/etc/systemd/system/tapebackarr.service"

# Script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." &> /dev/null && pwd )"

echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}   TapeBackarr Installation Script${NC}"
echo -e "${GREEN}==========================================${NC}"
echo

# Check for root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    exit 1
fi

# Check for Debian-based system
if ! command -v apt-get &> /dev/null; then
    echo -e "${YELLOW}Warning: This script is designed for Debian-based systems${NC}"
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Function to print step info
step() {
    echo -e "\n${GREEN}==>${NC} $1"
}

# Function to print substep info
substep() {
    echo -e "    ${YELLOW}→${NC} $1"
}

# Install dependencies
step "Installing system dependencies..."
apt-get update -qq
apt-get install -y -qq mt-st tar mbuffer sg3-utils lsscsi 2>/dev/null || {
    substep "Some optional packages may not be available"
}

# Create directories
step "Creating directories..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$DATA_DIR"
mkdir -p "$LOG_DIR"

substep "Created $INSTALL_DIR"
substep "Created $CONFIG_DIR"
substep "Created $DATA_DIR"
substep "Created $LOG_DIR"

# Check if binary exists in project root
BINARY_PATH=""
if [ -f "$PROJECT_ROOT/tapebackarr" ]; then
    BINARY_PATH="$PROJECT_ROOT/tapebackarr"
elif [ -f "./tapebackarr" ]; then
    BINARY_PATH="./tapebackarr"
fi

if [ -n "$BINARY_PATH" ]; then
    step "Installing TapeBackarr binary..."
    cp "$BINARY_PATH" "$INSTALL_DIR/tapebackarr"
    chmod +x "$INSTALL_DIR/tapebackarr"
    substep "Binary installed to $INSTALL_DIR/tapebackarr"
else
    echo -e "${YELLOW}Warning: Binary not found. You will need to build and copy it manually.${NC}"
    echo "Run: go build -o tapebackarr ./cmd/tapebackarr"
    echo "Then: sudo cp tapebackarr $INSTALL_DIR/"
fi

# Copy frontend static files
FRONTEND_BUILD=""
if [ -d "$PROJECT_ROOT/web/frontend/build" ]; then
    FRONTEND_BUILD="$PROJECT_ROOT/web/frontend/build"
elif [ -d "./web/frontend/build" ]; then
    FRONTEND_BUILD="./web/frontend/build"
fi

if [ -n "$FRONTEND_BUILD" ]; then
    step "Installing frontend static files..."
    rm -rf "$INSTALL_DIR/static"
    cp -r "$FRONTEND_BUILD" "$INSTALL_DIR/static"
    substep "Frontend files installed to $INSTALL_DIR/static"
else
    if [ ! -d "$INSTALL_DIR/static" ]; then
        echo -e "${YELLOW}Warning: Frontend build not found. You will need to build and copy it manually.${NC}"
        echo "Run: cd web/frontend && npm install && npm run build"
        echo "Then: sudo cp -r web/frontend/build $INSTALL_DIR/static"
    fi
fi

# Create configuration file if it doesn't exist
if [ ! -f "$CONFIG_DIR/config.json" ]; then
    step "Creating configuration file..."
    
    # Generate a random JWT secret
    JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)
    
    cat > "$CONFIG_DIR/config.json" << EOF
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "static_dir": "/opt/tapebackarr/static"
  },
  "database": {
    "path": "$DATA_DIR/tapebackarr.db"
  },
  "tape": {
    "default_device": "/dev/nst0",
    "drives": [
      {
        "device_path": "/dev/nst0",
        "display_name": "Primary LTO Drive",
        "enabled": true
      }
    ],
    "buffer_size_mb": 256,
    "block_size": 65536,
    "write_retries": 3,
    "verify_after_write": true
  },
  "logging": {
    "level": "info",
    "format": "json",
    "output_path": "$LOG_DIR/tapebackarr.log"
  },
  "auth": {
    "jwt_secret": "$JWT_SECRET",
    "token_expiration": 24,
    "session_timeout": 60
  },
  "notifications": {
    "telegram": {
      "enabled": false,
      "bot_token": "YOUR_TELEGRAM_BOT_TOKEN",
      "chat_id": "YOUR_TELEGRAM_CHAT_ID"
    }
  }
}
EOF
    chmod 600 "$CONFIG_DIR/config.json"
    substep "Configuration created at $CONFIG_DIR/config.json"
    echo -e "${YELLOW}    Important: Edit $CONFIG_DIR/config.json to configure your tape drives${NC}"
else
    substep "Configuration file already exists, skipping..."
fi

# Install systemd service
step "Installing systemd service..."
if [ -f "$SCRIPT_DIR/tapebackarr.service" ]; then
    cp "$SCRIPT_DIR/tapebackarr.service" "$SERVICE_FILE"
else
    cat > "$SERVICE_FILE" << 'EOF'
[Unit]
Description=TapeBackarr Tape Library Management System
Documentation=https://github.com/RoseOO/TapeBackarr
After=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/tapebackarr

ExecStart=/opt/tapebackarr/tapebackarr -config /etc/tapebackarr/config.json

# Restart policy
Restart=on-failure
RestartSec=5s

# Logging
StandardOutput=append:/var/log/tapebackarr/service.log
StandardError=append:/var/log/tapebackarr/service.log

# Security
NoNewPrivileges=false
ProtectSystem=false
ProtectHome=false
ReadWritePaths=/var/lib/tapebackarr /var/log/tapebackarr /dev

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

# Allow tape device access
SupplementaryGroups=tape

[Install]
WantedBy=multi-user.target
EOF
fi

substep "Service file installed to $SERVICE_FILE"

# Reload systemd
step "Configuring systemd..."
systemctl daemon-reload
substep "Systemd daemon reloaded"

# Enable service
systemctl enable tapebackarr
substep "Service enabled for automatic start on boot"

# Set permissions
step "Setting permissions..."
chown -R root:root "$INSTALL_DIR"
chown -R root:root "$CONFIG_DIR"
chown -R root:root "$DATA_DIR"
chown -R root:root "$LOG_DIR"
chmod 755 "$INSTALL_DIR"
chmod 755 "$CONFIG_DIR"
chmod 755 "$DATA_DIR"
chmod 755 "$LOG_DIR"

# Check for tape devices
step "Checking for tape devices..."
if ls /dev/st* /dev/nst* 2>/dev/null; then
    substep "Tape devices found"
else
    echo -e "${YELLOW}    Warning: No tape devices found. Ensure your tape drive is connected.${NC}"
fi

# Final instructions
echo
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}   Installation Complete!${NC}"
echo -e "${GREEN}==========================================${NC}"
echo
echo "Next steps:"
echo "  1. Edit the configuration file:"
echo "     sudo nano $CONFIG_DIR/config.json"
echo
echo "  2. Start the service:"
echo "     sudo systemctl start tapebackarr"
echo
echo "  3. Check service status:"
echo "     sudo systemctl status tapebackarr"
echo
echo "  4. View logs:"
echo "     sudo journalctl -u tapebackarr -f"
echo
echo "  5. Access the web interface:"
echo "     http://localhost:8080"
echo
echo "  Default credentials:"
echo "     Username: admin"
echo "     Password: changeme"
echo
echo -e "${YELLOW}⚠️  Remember to change the default password!${NC}"
echo
