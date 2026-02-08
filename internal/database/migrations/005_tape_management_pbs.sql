-- PBS-style tape management: UUID labels, lifecycle states, pool reuse rules, drive scanning

-- Recreate tapes table with expanded lifecycle states and UUID support
-- SQLite requires table recreation to modify CHECK constraints
PRAGMA foreign_keys=OFF;

CREATE TABLE tapes_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT,
    barcode TEXT UNIQUE,
    label TEXT NOT NULL,
    pool_id INTEGER REFERENCES tape_pools(id),
    status TEXT NOT NULL CHECK (status IN ('blank', 'active', 'full', 'expired', 'retired', 'exported')),
    capacity_bytes INTEGER DEFAULT 0,
    used_bytes INTEGER DEFAULT 0,
    write_count INTEGER DEFAULT 0,
    last_written_at DATETIME,
    offsite_location TEXT,
    export_time DATETIME,
    import_time DATETIME,
    labeled_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO tapes_new (id, uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes,
    write_count, last_written_at, offsite_location, export_time, import_time, labeled_at, created_at, updated_at)
SELECT id,
    lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)),2) || '-' || substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6))),
    barcode, label, pool_id,
    CASE WHEN status = 'offsite' THEN 'exported' ELSE status END,
    capacity_bytes, used_bytes, write_count, last_written_at, offsite_location,
    CASE WHEN status = 'offsite' THEN CURRENT_TIMESTAMP ELSE NULL END,
    NULL, NULL, created_at, updated_at
FROM tapes;

DROP TABLE tapes;
ALTER TABLE tapes_new RENAME TO tapes;

PRAGMA foreign_keys=ON;

-- Add pool management columns for reuse rules and allocation policy
ALTER TABLE tape_pools ADD COLUMN allow_reuse INTEGER DEFAULT 1;
ALTER TABLE tape_pools ADD COLUMN allocation_policy TEXT DEFAULT 'continue';
