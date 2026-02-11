-- Add hardware encryption fields to backup_jobs (per-job hw encryption)
ALTER TABLE backup_jobs ADD COLUMN hw_encryption_enabled BOOLEAN DEFAULT 0;
ALTER TABLE backup_jobs ADD COLUMN hw_encryption_key_id INTEGER REFERENCES encryption_keys(id);

-- Add hardware encryption fields to backup_sets (tracking which hw key was used)
ALTER TABLE backup_sets ADD COLUMN hw_encrypted BOOLEAN DEFAULT 0;
ALTER TABLE backup_sets ADD COLUMN hw_encryption_key_id INTEGER REFERENCES encryption_keys(id);
