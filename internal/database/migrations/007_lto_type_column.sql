-- Add lto_type column to tapes for automatic capacity inference
ALTER TABLE tapes ADD COLUMN lto_type TEXT DEFAULT '';
