-- Tape spanning support for multi-tape backups
-- When a backup exceeds the capacity of a single tape, it spans across multiple
-- tapes in the same pool. Each tape gets its own self-describing TOC containing
-- only the files written to that specific tape.

-- Tracks a backup that spans multiple tapes
CREATE TABLE IF NOT EXISTS tape_spanning_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL REFERENCES backup_jobs(id),
    total_tapes INTEGER DEFAULT 0,
    total_bytes INTEGER DEFAULT 0,
    total_files INTEGER DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed', 'failed')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Each tape in a spanning set, with its backup set and file range
CREATE TABLE IF NOT EXISTS tape_spanning_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    spanning_set_id INTEGER NOT NULL REFERENCES tape_spanning_sets(id),
    tape_id INTEGER NOT NULL REFERENCES tapes(id),
    backup_set_id INTEGER NOT NULL REFERENCES backup_sets(id),
    sequence_number INTEGER NOT NULL,
    bytes_written INTEGER DEFAULT 0,
    files_start_index INTEGER DEFAULT 0,
    files_end_index INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_spanning_members_set ON tape_spanning_members(spanning_set_id);
CREATE INDEX IF NOT EXISTS idx_spanning_members_tape ON tape_spanning_members(tape_id);

-- Operator-visible tape change requests during spanning backups
CREATE TABLE IF NOT EXISTS tape_change_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_execution_id INTEGER REFERENCES job_executions(id),
    spanning_set_id INTEGER REFERENCES tape_spanning_sets(id),
    current_tape_id INTEGER NOT NULL REFERENCES tapes(id),
    reason TEXT NOT NULL DEFAULT 'tape_full' CHECK (reason IN ('tape_full', 'tape_error')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'acknowledged', 'completed', 'cancelled')),
    requested_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    acknowledged_at DATETIME,
    new_tape_id INTEGER REFERENCES tapes(id)
);

CREATE INDEX IF NOT EXISTS idx_tape_change_status ON tape_change_requests(status);
