# Manual Tape Recovery Guide

This guide documents how to recover data from TapeBackarr tapes **without** using the TapeBackarr application. This is essential for disaster recovery scenarios where the application server may not be available.

## Prerequisites

### Required Packages (Debian/Ubuntu)

```bash
sudo apt-get update
sudo apt-get install mt-st tar mbuffer
```

### Verify Tape Drive

```bash
# List tape devices
ls -la /dev/st* /dev/nst*

# Check drive status
mt -f /dev/nst0 status
```

**Device naming:**
- `/dev/st0` - Rewinding device (rewinds after each operation)
- `/dev/nst0` - Non-rewinding device (recommended for sequential operations)

---

## Understanding TapeBackarr Tape Format

TapeBackarr writes data to tape in the following format:

```
[Label Block] [EOF] [Backup Set 1] [EOF] [Backup Set 2] [EOF] ... [EOD]
```

- **Label Block**: First 512 bytes contain `TAPEBACKARR|label|timestamp`
- **EOF**: File mark separator between backup sets
- **Backup Set**: Standard tar archive of files
- **EOD**: End of Data marker

---

## Basic Tape Operations

### 1. Check Tape Status

```bash
mt -f /dev/nst0 status
```

Output example:
```
SCSI 2 tape drive:
File number=0, block number=0, partition=0.
Tape block size 0 bytes. Density code 0x58 (LTO-6).
Soft error count since last status=0
General status bits on (41010000):
 BOT ONLINE IM_REP_EN
```

### 2. Rewind Tape

```bash
mt -f /dev/nst0 rewind
```

### 3. Eject Tape

```bash
mt -f /dev/nst0 eject
```

### 4. Read Tape Label

```bash
mt -f /dev/nst0 rewind
dd if=/dev/nst0 bs=512 count=1 2>/dev/null
```

Output example:
```
TAPEBACKARR|WEEKLY-001|1705334400
```

### 5. Skip to File Mark

```bash
# Skip forward N file marks
mt -f /dev/nst0 fsf 1   # Skip to file 1 (after label)
mt -f /dev/nst0 fsf 2   # Skip to file 2

# Skip backward N file marks
mt -f /dev/nst0 bsf 1
```

### 6. Position at Specific Block

```bash
# Seek to specific block number
mt -f /dev/nst0 seek 12345
```

---

## Restore Procedures

### Scenario 1: Restore Entire Backup Set

This procedure restores all files from a single backup set.

```bash
# 1. Rewind tape
mt -f /dev/nst0 rewind

# 2. Skip the label block (file 0 is the label)
mt -f /dev/nst0 fsf 1

# 3. If you want a specific backup set, skip to it
# For example, to get the third backup set:
mt -f /dev/nst0 fsf 2

# 4. List contents without extracting
tar -tvf /dev/nst0

# 5. Extract all files to current directory
tar -xvf /dev/nst0

# Or extract to specific directory:
tar -xvf /dev/nst0 -C /restore/destination
```

### Scenario 2: Restore Specific Files

```bash
# 1. Position tape (after rewinding and skipping label)
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# 2. Extract specific files
tar -xvf /dev/nst0 path/to/file1.txt path/to/file2.doc

# 3. Or extract files matching a pattern
tar -xvf /dev/nst0 --wildcards "*.pdf"
```

### Scenario 3: Restore to Network Location

```bash
# 1. Mount network share
# For SMB/CIFS:
sudo mount -t cifs //server/share /mnt/restore -o username=user,password=pass

# For NFS:
sudo mount -t nfs server:/export/path /mnt/restore

# 2. Position tape and extract
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1
tar -xvf /dev/nst0 -C /mnt/restore

# 3. Unmount when done
sudo umount /mnt/restore
```

### Scenario 4: Restore from Spanning Set (Multi-Tape)

When a backup spans multiple tapes:

```bash
# Tape 1
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1
tar -cvMf /dev/nst0 -C /restore/path

# When prompted "Prepare volume #2 for '/dev/nst0' and hit return":
# 1. Eject current tape
mt -f /dev/nst0 eject

# 2. Insert next tape in sequence
# 3. Press Enter to continue

# Repeat until restore completes
```

**Alternative using shell script:**

```bash
#!/bin/bash

DEVICE="/dev/nst0"
RESTORE_PATH="/restore/destination"
TAPE_NUM=1

while true; do
    echo "Insert tape $TAPE_NUM and press Enter..."
    read
    
    mt -f $DEVICE rewind
    mt -f $DEVICE fsf 1
    
    if tar -xMf $DEVICE -C "$RESTORE_PATH"; then
        echo "Restore complete!"
        break
    else
        echo "Tape ended, need next tape..."
        mt -f $DEVICE eject
        ((TAPE_NUM++))
    fi
done
```

---

## Recovering the TapeBackarr Database

If a database backup was written to tape:

```bash
# 1. Position to the database backup
# Database backups are typically at a known position
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# 2. List to find the database backup
tar -tvf /dev/nst0 | grep "tapebackarr.db"

# 3. Extract the database
tar -xvf /dev/nst0 tapebackarr.db

# 4. Restore to proper location
sudo mv tapebackarr.db /var/lib/tapebackarr/
sudo chown root:root /var/lib/tapebackarr/tapebackarr.db
```

---

## Advanced Recovery Techniques

### Using mbuffer for Reliable Reading

For large restores, use mbuffer to handle tape streaming:

```bash
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1
mbuffer -i /dev/nst0 -m 256M | tar -xvf - -C /restore/path
```

### Reading Tape with Specific Block Size

TapeBackarr uses 64KB blocks by default:

```bash
# Use matching block size
tar -xvf /dev/nst0 -b 128  # 128 x 512 = 65536 bytes
```

### Recovering Partially Damaged Tape

```bash
# Skip bad blocks and continue
tar -xvf /dev/nst0 --ignore-failed-read -C /restore/path

# Read with retries
dd if=/dev/nst0 of=backup.tar bs=65536 conv=noerror,sync
tar -xvf backup.tar -C /restore/path
```

### Finding File Positions on Tape

To search for a specific file across all backup sets:

```bash
#!/bin/bash

DEVICE="/dev/nst0"
FILE_PATTERN="$1"
FILE_NUM=0

mt -f $DEVICE rewind
mt -f $DEVICE fsf 1  # Skip label

while true; do
    echo "=== Backup Set $FILE_NUM ===" 
    if tar -tvf $DEVICE 2>/dev/null | grep -i "$FILE_PATTERN"; then
        echo "Found in file number: $((FILE_NUM + 1))"
    fi
    
    # Try to move to next file mark
    if ! mt -f $DEVICE fsf 1 2>/dev/null; then
        echo "End of tape reached"
        break
    fi
    
    ((FILE_NUM++))
done
```

---

## Tape Inventory Without TapeBackarr

To catalog a tape's contents:

```bash
#!/bin/bash

DEVICE="/dev/nst0"
OUTPUT_FILE="tape_inventory.txt"

echo "Tape Inventory" > $OUTPUT_FILE
echo "==============" >> $OUTPUT_FILE
echo >> $OUTPUT_FILE

# Read label
mt -f $DEVICE rewind
echo "Label: $(dd if=$DEVICE bs=512 count=1 2>/dev/null)" >> $OUTPUT_FILE
echo >> $OUTPUT_FILE

# Skip to first backup set
mt -f $DEVICE fsf 1
FILE_NUM=1

while true; do
    echo "=== Backup Set $FILE_NUM ===" >> $OUTPUT_FILE
    
    if ! tar -tvf $DEVICE >> $OUTPUT_FILE 2>/dev/null; then
        echo "End of tape or error" >> $OUTPUT_FILE
        break
    fi
    
    echo >> $OUTPUT_FILE
    ((FILE_NUM++))
done

echo "Inventory saved to $OUTPUT_FILE"
```

---

## Troubleshooting

### "No medium found"

```bash
# Check if tape is loaded
mt -f /dev/nst0 status

# Try loading tape (if drive supports it)
mt -f /dev/nst0 load
```

### "I/O error" on read

```bash
# Clean the tape heads (use cleaning tape)
# Try a different tape
# Check cable connections

# Force retension the tape
mt -f /dev/nst0 retension
```

### "Wrong medium type"

```bash
# Check tape density
mt -f /dev/nst0 status | grep Density

# Ensure tape is compatible with drive
# LTO drives can typically read N-2 generations
```

### Cannot find files on tape

```bash
# Make sure you're at the right position
mt -f /dev/nst0 rewind
mt -f /dev/nst0 status  # Should show File number=0

# List the first backup set
mt -f /dev/nst0 fsf 1
tar -tvf /dev/nst0 | head -50
```

---

## Reference: Common mt Commands

| Command | Description |
|---------|-------------|
| `mt -f /dev/nst0 status` | Show drive and tape status |
| `mt -f /dev/nst0 rewind` | Rewind tape to beginning |
| `mt -f /dev/nst0 eject` | Eject tape from drive |
| `mt -f /dev/nst0 load` | Load tape (if supported) |
| `mt -f /dev/nst0 fsf N` | Forward skip N file marks |
| `mt -f /dev/nst0 bsf N` | Backward skip N file marks |
| `mt -f /dev/nst0 seek N` | Position to block N |
| `mt -f /dev/nst0 tell` | Show current position |
| `mt -f /dev/nst0 weof` | Write file mark |
| `mt -f /dev/nst0 erase` | Erase tape (DESTRUCTIVE!) |
| `mt -f /dev/nst0 retension` | Retension tape |
| `mt -f /dev/nst0 setblk N` | Set block size to N bytes |

---

## Reference: Tar Options for Tape

| Option | Description |
|--------|-------------|
| `-x` | Extract files |
| `-t` | List contents |
| `-v` | Verbose output |
| `-f DEVICE` | Use tape device |
| `-b N` | Block size (N x 512 bytes) |
| `-C DIR` | Extract to directory |
| `-M` | Multi-volume (spanning) |
| `--wildcards` | Use wildcards in file names |
| `--ignore-failed-read` | Continue on read errors |

---

## Restoring Encrypted Backups

TapeBackarr supports AES-256 encryption for backups. Encrypted backups require the encryption key to restore. This section covers manual decryption without the TapeBackarr application.

### Prerequisites for Encrypted Restore

In addition to the standard tools (mt, tar), you'll need:

```bash
# Install openssl (usually pre-installed)
sudo apt-get install openssl
```

### Obtaining Your Encryption Key

1. **From TapeBackarr UI**: Navigate to Settings → Encryption Keys → Print Key Sheet
2. **From API**: `GET /api/v1/encryption-keys/keysheet/text`
3. **From Database** (emergency):
   ```bash
   sqlite3 /var/lib/tapebackarr/tapebackarr.db \
     "SELECT name, key_data FROM encryption_keys"
   ```

### Restore Encrypted Backup Set

**Method 1: Using OpenSSL (Recommended)**

```bash
# 1. Position tape to the encrypted backup set
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1  # Skip label, adjust number for specific backup set

# 2. Decrypt and extract in one pipeline
# Replace YOUR_KEY_BASE64 with the actual key from your key sheet
openssl enc -d -aes-256-cbc -pbkdf2 -iter 100000 \
  -pass pass:YOUR_KEY_BASE64 \
  -in /dev/nst0 | tar -xvf - -C /restore/destination
```

**Method 2: Decrypt to File First (for verification)**

```bash
# 1. Position tape
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# 2. Decrypt to intermediate file
openssl enc -d -aes-256-cbc -pbkdf2 -iter 100000 \
  -pass pass:YOUR_KEY_BASE64 \
  -in /dev/nst0 -out backup.tar

# 3. Verify tar archive
tar -tvf backup.tar | head -50

# 4. Extract
tar -xvf backup.tar -C /restore/destination

# 5. Clean up
rm backup.tar
```

### Restore Specific Files from Encrypted Backup

```bash
# 1. Position tape
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# 2. Decrypt and extract specific files
openssl enc -d -aes-256-cbc -pbkdf2 -iter 100000 \
  -pass pass:YOUR_KEY_BASE64 \
  -in /dev/nst0 | tar -xvf - -C /restore/destination \
  path/to/specific/file.txt \
  another/path/to/restore/
```

### Restoring Encrypted Backup to Network Location

```bash
# 1. Mount network share
sudo mount -t cifs //server/share /mnt/restore -o username=user,password=pass

# 2. Position and restore
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

openssl enc -d -aes-256-cbc -pbkdf2 -iter 100000 \
  -pass pass:YOUR_KEY_BASE64 \
  -in /dev/nst0 | tar -xvf - -C /mnt/restore

# 3. Unmount
sudo umount /mnt/restore
```

### Multi-Tape Encrypted Restore

For encrypted backups spanning multiple tapes:

```bash
#!/bin/bash

DEVICE="/dev/nst0"
RESTORE_PATH="/restore/destination"
KEY="YOUR_KEY_BASE64"
TAPE_NUM=1

while true; do
    echo "Insert tape $TAPE_NUM and press Enter..."
    read
    
    mt -f $DEVICE rewind
    mt -f $DEVICE fsf 1
    
    # Decrypt and extract with multi-volume support
    if openssl enc -d -aes-256-cbc -pbkdf2 -iter 100000 \
        -pass pass:$KEY \
        -in $DEVICE | tar -xMvf - -C "$RESTORE_PATH"; then
        echo "Restore complete!"
        break
    else
        echo "Tape ended, need next tape..."
        mt -f $DEVICE eject
        ((TAPE_NUM++))
    fi
done
```

### Troubleshooting Encrypted Restore

**"bad decrypt" error:**
- Verify you're using the correct encryption key
- Check that the backup was actually encrypted (non-encrypted backups start with tar header)
- Ensure the key is the exact base64 string without extra spaces or newlines

**Checking if backup is encrypted:**
```bash
# Read first few bytes
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1
dd if=/dev/nst0 bs=1 count=20 2>/dev/null | xxd

# Encrypted data looks random; tar archives start with filename
```

**Finding which key was used:**
If you have multiple keys, you can identify the correct one by:
1. Check TapeBackarr database: `SELECT encryption_key_id FROM backup_sets WHERE id = N`
2. Match with keys: `SELECT id, name, key_fingerprint FROM encryption_keys`

### Key Sheet Format

When printing your key sheet for paper backup, it will contain:

```
===============================================================================
                    TAPEBACKARR ENCRYPTION KEY BACKUP
===============================================================================

Generated: 2026-02-08T10:00:00Z

IMPORTANT: Store this document in a secure location (safe, security deposit box).
This sheet contains encryption keys needed to restore encrypted backups.

-------------------------------------------------------------------------------
                              KEY LISTING
-------------------------------------------------------------------------------

KEY #1
  Name:        production-backups
  ID:          1
  Algorithm:   aes-256-gcm
  Fingerprint: a1b2c3d4e5f6...
  Created:     2026-01-15T08:30:00Z
  Key (Base64):
    dGhpcyBpcyBhIHNhbXBsZSBrZXkgZm9yIGRlbW9uc
    dHJhdGlvbiBwdXJwb3Nlcw==

-------------------------------------------------------------------------------
                          END OF KEY LISTING
-------------------------------------------------------------------------------

Store this document securely. Destroy old copies when regenerating.
```

---

## Emergency Contact Information

For hardware issues with tape drives:
- Check drive vendor documentation
- Contact your IT support team
- Refer to drive manufacturer support

For TapeBackarr software issues:
- GitHub: https://github.com/RoseOO/TapeBackarr
- Check existing issues or open a new one

---

## Document Version

- Version: 1.1
- Last Updated: February 2026
- Applies to: TapeBackarr 1.x, LTO-5 through LTO-9 drives
- Added: Encrypted backup restore procedures
