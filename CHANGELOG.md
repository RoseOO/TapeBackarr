# Changelog

All notable changes to TapeBackarr will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Docker and Docker Compose support for containerized deployment
- Proxmox LXC installation script for community scripts compatibility
- Email notification support (SMTP)
- Health check API endpoint
- CONTRIBUTING.md guidelines
- SECURITY.md policy
- CHANGELOG.md version history

### Changed
- Updated documentation with LXC deployment instructions

## [0.1.0] - 2024-01-15

### Added

#### Core Features
- Direct streaming from SMB/NFS/local filesystem to tape
- Full file-level catalog with block offset tracking
- Incremental backup support with timestamp and size comparison
- Multi-tape spanning with automatic continuation markers
- Guided restore workflow with tape insertion guidance
- Telegram notifications for tape change requests and job status
- Database backup to tape for disaster recovery
- Multi-drive support for managing multiple tape drives

#### Tape Management
- Tape labeling and pool assignment (DAILY, WEEKLY, MONTHLY, ARCHIVE)
- Status tracking (blank, active, full, retired, offsite)
- Capacity and usage monitoring
- Write count tracking
- Offsite location tracking

#### Backup Operations
- Scheduled backups with cron expressions
- Manual backup execution
- Glob-based include/exclude patterns
- Full and incremental backup types
- Job state persistence for resume after crash

#### Restore Operations
- Restore to local filesystem
- Restore to network destinations (SMB/NFS)
- File verification with checksums
- Guided multi-tape restore workflow

#### Web Interface
- Modern, responsive dashboard
- Tape management with status updates
- Multi-drive management and selection
- Backup job configuration and scheduling
- Catalog browsing and file search
- Guided restore wizard
- Audit log viewer with export
- Role-based access control (admin/operator/read-only)
- In-app documentation access

#### Proxmox Integration
- Discover VMs and LXC containers across nodes/clusters
- Backup individual guests or all guests to tape
- Restore guests from tape to same or different nodes
- Scheduled automated backups with retention policies
- Preserve complete guest configuration for disaster recovery

#### API
- RESTful API for all operations
- JWT-based authentication
- Rate limiting
- Documentation endpoint

#### Security
- JWT-based authentication
- Role-based access control
- bcrypt password hashing
- Audit logging for compliance
- AES-256 encryption support for backups

### Security

- Initial security model implementation
- See SECURITY.md for security policy

---

## Version History Summary

| Version | Release Date | Highlights |
|---------|--------------|------------|
| 0.1.0   | 2024-01-15   | Initial release with core features |

---

## Upgrade Notes

### Upgrading to 0.1.x

This is the initial release. No upgrade path required.

For future upgrades:
1. Stop the TapeBackarr service
2. Backup the database: `sqlite3 /var/lib/tapebackarr/tapebackarr.db ".backup backup.db"`
3. Install the new version
4. Run any database migrations (will be automatic in future versions)
5. Start the service

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on contributing to this project.
