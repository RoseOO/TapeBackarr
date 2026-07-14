-- Add backup_destinations table for configuring backup targets (tape pools, file paths, NFS shares, etc.)
CREATE TABLE IF NOT EXISTS backup_destinations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    destination_type TEXT NOT NULL DEFAULT 'tape_pool', -- tape_pool, file
    path            TEXT DEFAULT '',                     -- used for file destinations
    pool_id         INTEGER REFERENCES tape_pools(id),  -- used for tape_pool destinations
    enabled         BOOLEAN DEFAULT 1,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add destination_id to backup_jobs (NULL = use pool_id, for backward compatibility)
ALTER TABLE backup_jobs ADD COLUMN destination_id INTEGER REFERENCES backup_destinations(id);

-- Add file_path to backup_sets (used for file-based backups instead of tape)
ALTER TABLE backup_sets ADD COLUMN file_path TEXT DEFAULT '';
