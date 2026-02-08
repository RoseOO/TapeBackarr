-- Encryption key management

-- Encryption keys table - stores encryption keys for backup encryption
CREATE TABLE IF NOT EXISTS encryption_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    algorithm TEXT NOT NULL DEFAULT 'aes-256-gcm',
    key_data TEXT NOT NULL,  -- Base64 encoded encrypted key (encrypted with master password or stored securely)
    key_fingerprint TEXT NOT NULL,  -- SHA256 fingerprint for identification without exposing key
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_encryption_keys_name ON encryption_keys(name);
CREATE INDEX IF NOT EXISTS idx_encryption_keys_fingerprint ON encryption_keys(key_fingerprint);

-- Add encryption fields to backup_jobs
ALTER TABLE backup_jobs ADD COLUMN encryption_enabled BOOLEAN DEFAULT 0;
ALTER TABLE backup_jobs ADD COLUMN encryption_key_id INTEGER REFERENCES encryption_keys(id);

-- Add encryption fields to backup_sets
ALTER TABLE backup_sets ADD COLUMN encrypted BOOLEAN DEFAULT 0;
ALTER TABLE backup_sets ADD COLUMN encryption_key_id INTEGER REFERENCES encryption_keys(id);
