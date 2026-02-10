-- Add format_type column to tapes table to track whether a tape uses raw (tar)
-- or LTFS format. Default is 'raw' for backwards compatibility with existing tapes.
ALTER TABLE tapes ADD COLUMN format_type TEXT NOT NULL DEFAULT 'raw' CHECK (format_type IN ('raw', 'ltfs'));

-- Add format_type column to backup_sets table to record the format used for
-- each backup set, allowing correct restore logic (tar streaming vs LTFS copy).
ALTER TABLE backup_sets ADD COLUMN format_type TEXT NOT NULL DEFAULT 'raw' CHECK (format_type IN ('raw', 'ltfs'));
