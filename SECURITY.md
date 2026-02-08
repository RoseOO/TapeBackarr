# Security Policy

## Supported Versions

We release patches for security vulnerabilities for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

We take security issues seriously. If you discover a security vulnerability within TapeBackarr, please report it responsibly.

### How to Report

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please send an email to the project maintainers with:

1. **Description**: A clear description of the vulnerability
2. **Impact**: What could an attacker do with this vulnerability?
3. **Reproduction**: Step-by-step instructions to reproduce the issue
4. **Affected Versions**: Which versions are affected
5. **Suggested Fix**: If you have a suggestion for how to fix it

### What to Expect

- **Acknowledgment**: We will acknowledge receipt of your report within 48 hours
- **Updates**: We will keep you informed about our progress
- **Credit**: We will credit you in the security advisory (unless you prefer anonymity)
- **Timeline**: We aim to address critical vulnerabilities within 7 days

### Disclosure Policy

- We will work with you to understand and resolve the issue
- We will prepare a fix and coordinate the release
- We will publish a security advisory with appropriate details
- We ask that you do not disclose the vulnerability publicly until we have released a fix

## Security Best Practices

When deploying TapeBackarr:

### Authentication

1. **Change default credentials immediately** after installation
2. Use **strong passwords** (minimum 12 characters, mixed case, numbers, symbols)
3. Enable **token-based API access** for automation
4. Regularly **rotate API tokens**

### Network Security

1. **Use a reverse proxy** (nginx, Caddy) with TLS/HTTPS
2. **Restrict network access** to trusted networks only
3. Configure **firewall rules** to limit access to port 8080
4. Use **VPN** for remote access

### System Security

1. Run TapeBackarr as a **dedicated user** (not root when possible)
2. Keep the **system updated** with security patches
3. Use **read-only file systems** where possible
4. Monitor **audit logs** regularly

### Data Security

1. **Encrypt sensitive backups** using the built-in encryption feature
2. **Store encryption keys securely** (print key sheets for disaster recovery)
3. **Protect tape media** during transport and storage
4. Use **offsite storage** for critical backups

### Configuration

```json
{
  "auth": {
    "jwt_secret": "USE_A_STRONG_RANDOM_SECRET_HERE",
    "token_expiration": 24,
    "session_timeout": 60
  }
}
```

**Never use default or weak JWT secrets in production.**

Generate a secure secret:
```bash
openssl rand -base64 32
```

## Known Security Considerations

### Tape Device Access

TapeBackarr requires access to tape devices (`/dev/st*`, `/dev/nst*`). This requires:
- Membership in the `tape` group, OR
- Running as root (not recommended for production)

### Database Security

The SQLite database contains:
- Password hashes (bcrypt)
- Tape catalog information
- Configuration data

Protect the database file with appropriate permissions:
```bash
chmod 600 /var/lib/tapebackarr/tapebackarr.db
```

### Log Files

Log files may contain:
- File paths from backup operations
- IP addresses
- Usernames

Ensure log files are protected:
```bash
chmod 640 /var/log/tapebackarr/*.log
```

### Proxmox Integration

When using Proxmox integration:
- Use **API tokens** instead of passwords
- Apply **principle of least privilege** to API token permissions
- Rotate tokens periodically

## Security Hardening Checklist

- [ ] Changed default admin password
- [ ] Generated a secure JWT secret
- [ ] Configured TLS/HTTPS (via reverse proxy)
- [ ] Restricted network access
- [ ] Set up firewall rules
- [ ] Configured appropriate file permissions
- [ ] Enabled audit logging
- [ ] Set up log rotation
- [ ] Configured Telegram alerts for security events
- [ ] Printed and secured encryption key sheets
- [ ] Documented disaster recovery procedures

## Third-Party Dependencies

TapeBackarr uses the following key dependencies:

| Package | Purpose | Security Notes |
|---------|---------|----------------|
| `golang.org/x/crypto` | Password hashing | Uses bcrypt |
| `github.com/golang-jwt/jwt/v5` | JWT tokens | Industry standard |
| `github.com/mattn/go-sqlite3` | Database | CGO-based SQLite |
| `github.com/go-chi/chi/v5` | HTTP router | Well-maintained |

We regularly update dependencies to include security patches.

## Contact

For security-related questions or to report vulnerabilities, please contact the project maintainers through the appropriate channels.

Thank you for helping keep TapeBackarr secure!
