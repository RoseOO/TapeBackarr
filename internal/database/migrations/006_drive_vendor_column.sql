-- Add vendor column to tape_drives for storing drive manufacturer info
ALTER TABLE tape_drives ADD COLUMN vendor TEXT DEFAULT '';
