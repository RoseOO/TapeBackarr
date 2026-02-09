-- Tape library (autochanger) support
CREATE TABLE IF NOT EXISTS tape_libraries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    device_path TEXT NOT NULL,  -- SCSI generic device e.g. /dev/sg3
    vendor TEXT DEFAULT '',
    model TEXT DEFAULT '',
    serial_number TEXT DEFAULT '',
    num_slots INTEGER DEFAULT 0,
    num_drives INTEGER DEFAULT 0,
    num_import_export INTEGER DEFAULT 0,  -- mail slots / import-export elements
    barcode_reader BOOLEAN DEFAULT 0,
    enabled BOOLEAN DEFAULT 1,
    last_inventory_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tape library slots track what is in each slot
CREATE TABLE IF NOT EXISTS tape_library_slots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES tape_libraries(id) ON DELETE CASCADE,
    slot_number INTEGER NOT NULL,
    slot_type TEXT NOT NULL DEFAULT 'storage' CHECK (slot_type IN ('storage', 'import_export', 'drive')),
    tape_id INTEGER REFERENCES tapes(id),
    barcode TEXT DEFAULT '',
    is_empty BOOLEAN DEFAULT 1,
    drive_id INTEGER REFERENCES tape_drives(id),  -- only for drive slots
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(library_id, slot_number)
);

CREATE INDEX IF NOT EXISTS idx_library_slots_library ON tape_library_slots(library_id);
CREATE INDEX IF NOT EXISTS idx_library_slots_tape ON tape_library_slots(tape_id);

-- Link drives to libraries
ALTER TABLE tape_drives ADD COLUMN library_id INTEGER REFERENCES tape_libraries(id);
ALTER TABLE tape_drives ADD COLUMN library_drive_number INTEGER;
