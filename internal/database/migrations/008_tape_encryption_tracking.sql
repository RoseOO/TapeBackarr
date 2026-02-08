-- Track encryption key fingerprint on tapes for visibility in the tape library
ALTER TABLE tapes ADD COLUMN encryption_key_fingerprint TEXT DEFAULT '';
ALTER TABLE tapes ADD COLUMN encryption_key_name TEXT DEFAULT '';
