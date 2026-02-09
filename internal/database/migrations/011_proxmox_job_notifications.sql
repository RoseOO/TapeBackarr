-- Add notification and notes columns to proxmox_backup_jobs
ALTER TABLE proxmox_backup_jobs ADD COLUMN notify_on_success BOOLEAN DEFAULT 0;
ALTER TABLE proxmox_backup_jobs ADD COLUMN notify_on_failure BOOLEAN DEFAULT 1;
ALTER TABLE proxmox_backup_jobs ADD COLUMN notes TEXT DEFAULT '';
