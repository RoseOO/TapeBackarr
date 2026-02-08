#!/bin/bash

# TapeBackarr Updater Script
# Run this script inside the LXC container to update TapeBackarr
# from the latest GitHub sources.
#
# Usage: bash /opt/TapeBackarr/deploy/updater.sh

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Paths
SOURCE_DIR="/opt/TapeBackarr"
INSTALL_DIR="/opt/tapebackarr"

msg_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
msg_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
msg_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
msg_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Check for root
if [ "$EUID" -ne 0 ]; then
    msg_error "This script must be run as root"
fi

# Ensure Go is available
export PATH=$PATH:/usr/local/go/bin

if ! command -v go &> /dev/null; then
    msg_error "Go is not installed. Please install Go first."
fi

if ! command -v node &> /dev/null; then
    msg_error "Node.js is not installed. Please install Node.js first."
fi

echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}   TapeBackarr Updater${NC}"
echo -e "${GREEN}==========================================${NC}"
echo

# Check if source directory exists
if [ ! -d "$SOURCE_DIR/.git" ]; then
    msg_error "Source directory $SOURCE_DIR not found or is not a git repository."
fi

# Pull latest changes
msg_info "Pulling latest changes from GitHub..."
cd "$SOURCE_DIR"
BEFORE=$(git rev-parse HEAD)
git fetch origin
git reset --hard origin/main
AFTER=$(git rev-parse HEAD)

if [ "$BEFORE" = "$AFTER" ]; then
    msg_warn "Already up to date (commit: ${BEFORE:0:8})"
    read -p "Rebuild anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 0
    fi
else
    msg_ok "Updated from ${BEFORE:0:8} to ${AFTER:0:8}"
fi

# Build backend
msg_info "Building Go backend..."
CGO_ENABLED=0 go build -o tapebackarr ./cmd/tapebackarr
msg_ok "Backend built"

# Build frontend
msg_info "Building frontend (this may take a moment)..."
cd web/frontend
npm install --no-audit --no-fund
npm run build
cd "$SOURCE_DIR"
msg_ok "Frontend built"

# Stop the service
msg_info "Stopping TapeBackarr service..."
systemctl stop tapebackarr || true

# Install new binary
msg_info "Installing new binary..."
cp tapebackarr "$INSTALL_DIR/tapebackarr"
chmod +x "$INSTALL_DIR/tapebackarr"
msg_ok "Binary installed"

# Install new frontend
msg_info "Installing new frontend..."
rm -rf "$INSTALL_DIR/static"
cp -r web/frontend/build "$INSTALL_DIR/static"
msg_ok "Frontend installed"

# Start the service
msg_info "Starting TapeBackarr service..."
systemctl start tapebackarr

# Wait a moment and check if it's running
sleep 2
if systemctl is-active --quiet tapebackarr; then
    msg_ok "TapeBackarr service is running"
else
    msg_error "TapeBackarr service failed to start. Check logs: journalctl -u tapebackarr -n 50"
fi

echo
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}   Update Complete!${NC}"
echo -e "${GREEN}==========================================${NC}"
echo
echo -e "  Version: $(${INSTALL_DIR}/tapebackarr -version 2>&1 || echo 'unknown')"
echo -e "  Commit:  ${AFTER:0:8}"
echo
