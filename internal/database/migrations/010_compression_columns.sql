-- Add compression support columns
ALTER TABLE backup_jobs ADD COLUMN compression TEXT DEFAULT 'none';
ALTER TABLE backup_sets ADD COLUMN compressed BOOLEAN DEFAULT 0;
ALTER TABLE backup_sets ADD COLUMN compression_type TEXT DEFAULT 'none';
