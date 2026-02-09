# Manual Tape Recovery Guide

This guide documents how to recover data from TapeBackarr tapes **without** using the TapeBackarr application. This is essential for disaster recovery scenarios where the application server may not be available.

## Prerequisites

### Required Packages (Debian/Ubuntu)

```bash
sudo apt-get update
sudo apt-get install mt-st tar mbuffer lsscsi pigz
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

TapeBackarr writes data to tape in a self-describing format:

```
[Label Block] [FM] [Backup Data] [FM] [TOC] [FM] [EOD]
  File #0            File #1           File #2
```

- **Label Block** (File #0): First 512 bytes contain `TAPEBACKARR|label|uuid|pool|timestamp|encryption_fingerprint|compression_type`
- **FM**: File mark separator between sections
- **Backup Data** (File #1): Standard tar archive of files (optionally encrypted/compressed)
- **TOC** (File #2): JSON Table of Contents listing every file in the backup set, including paths, sizes, timestamps, and checksums. This makes the tape self-describing even without access to the TapeBackarr database. Written in 64KB blocks, padded with null bytes.
- **EOD**: End of Data marker

---

## Reading the Table of Contents (TOC)

The TOC is a JSON document stored at file #2 on the tape. It contains the complete
file catalog for the backup, allowing you to see what is on the tape without the
TapeBackarr database and without reading the entire tar archive.

### Quick TOC Read

```bash
# Rewind and skip to TOC (file #2)
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 2

# Read the TOC — strip null padding with tr
dd if=/dev/nst0 bs=64k 2>/dev/null | tr -d '\0'
```

### Pretty-Print the TOC

```bash
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 2
dd if=/dev/nst0 bs=64k 2>/dev/null | tr -d '\0' | python3 -m json.tool
```

### TOC Structure

The TOC is a JSON object with this structure:

```json
{
  "magic": "TAPEBACKARR_TOC",
  "version": 1,
  "tape_label": "WEEKLY-001",
  "tape_uuid": "abc-def-123",
  "pool": "weekly",
  "created_at": "2026-02-08T10:00:00Z",
  "backup_sets": [
    {
      "file_number": 1,
      "job_name": "nightly-full",
      "backup_type": "full",
      "start_time": "2026-02-08T02:00:00Z",
      "end_time": "2026-02-08T04:30:00Z",
      "file_count": 1500,
      "total_bytes": 52428800,
      "encrypted": false,
      "compressed": false,
      "files": [
        {
          "path": "documents/report.pdf",
          "size": 5000,
          "mod_time": "2026-02-07T15:00:00Z",
          "checksum": "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"
        }
      ]
    }
  ]
}
```

### Using the TOC for File Recovery

```bash
# 1. Read the TOC to find your file
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 2
dd if=/dev/nst0 bs=64k 2>/dev/null | tr -d '\0' | python3 -c "
import json, sys
toc = json.load(sys.stdin)
for bs in toc['backup_sets']:
    for f in bs['files']:
        if 'report' in f['path'].lower():
            print(f'  {f[\"path\"]} ({f[\"size\"]} bytes)')
"

# 2. Once you know the file is there, extract it from the backup data (file #1)
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1
tar -xvf /dev/nst0 documents/report.pdf
```

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
TAPEBACKARR|WEEKLY-001|a1b2c3d4-e5f6-7890-abcd-ef1234567890|WEEKLY|1705334400||none
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

## Recovering Compressed Tapes

If your backup was created with compression enabled, you need to decompress the data after extracting from tape.

### Gzip Compressed Tapes

```bash
# Rewind and skip past the label
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# Extract and decompress in one step
dd if=/dev/nst0 bs=65536 | gunzip | tar xvf - -C /restore/path/

# Or save compressed data first, then decompress
dd if=/dev/nst0 bs=65536 of=/tmp/backup.tar.gz
tar xzf /tmp/backup.tar.gz -C /restore/path/
```

### Zstd Compressed Tapes

```bash
# Rewind and skip past the label
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# Extract and decompress in one step
dd if=/dev/nst0 bs=65536 | zstd -d | tar xvf - -C /restore/path/

# Or save compressed data first, then decompress
dd if=/dev/nst0 bs=65536 of=/tmp/backup.tar.zst
zstd -d /tmp/backup.tar.zst -o /tmp/backup.tar
tar xf /tmp/backup.tar -C /restore/path/
```

### Compressed AND Encrypted Tapes

If the tape is both compressed and encrypted, you must decrypt first, then decompress:

```bash
# Rewind and skip past the label
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# Gzip + encrypted: decrypt then decompress
dd if=/dev/nst0 bs=65536 | openssl enc -aes-256-cbc -d -salt -pbkdf2 -iter 100000 -pass pass:YOUR_KEY | gunzip | tar xvf - -C /restore/path/

# Zstd + encrypted: decrypt then decompress
dd if=/dev/nst0 bs=65536 | openssl enc -aes-256-cbc -d -salt -pbkdf2 -iter 100000 -pass pass:YOUR_KEY | zstd -d | tar xvf - -C /restore/path/
```

### Identifying Compression Type

The tape label (first block on tape) contains metadata about the compression type used.
To read the label:

```bash
mt -f /dev/nst0 rewind
dd if=/dev/nst0 bs=512 count=1 2>/dev/null
```

The label is a pipe-delimited string: `TAPEBACKARR|label|uuid|pool|timestamp|encryption_fingerprint|compression_type`. The last field indicates the compression type. Values are: `none`, `gzip`, or `zstd`.

Alternatively, read the TOC (file #2) for structured JSON metadata:

```bash
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 2
dd if=/dev/nst0 bs=64k 2>/dev/null | tr -d '\0' | python3 -m json.tool
```

The `compressed` and `compression_type` fields in each backup set entry indicate whether compression was used.

## Recovering Database Backups from Tape

If you have lost your TapeBackarr database but have a database backup on tape, you can recover it without any prior knowledge of what's on the tape.

### Scanning for Database Backups

```bash
# Rewind the tape
mt -f /dev/nst0 rewind

# Read the label to identify the tape
dd if=/dev/nst0 bs=512 count=1 2>/dev/null

# Skip to the data section (past label)
mt -f /dev/nst0 fsf 1

# Try to list the tar contents - database backups are named tapebackarr-db-*.sql
dd if=/dev/nst0 bs=65536 | tar tvf - 2>/dev/null | grep -i "tapebackarr-db"

# If encrypted, try with your key:
dd if=/dev/nst0 bs=65536 | openssl enc -aes-256-cbc -d -salt -pbkdf2 -iter 100000 -pass pass:YOUR_KEY | tar tvf - 2>/dev/null | grep -i "tapebackarr-db"
```

### Extracting the Database Backup

```bash
# Position tape
mt -f /dev/nst0 rewind
mt -f /dev/nst0 fsf 1

# Extract just the database file
dd if=/dev/nst0 bs=65536 | tar xvf - -C /tmp/ --wildcards '*tapebackarr-db*'

# For encrypted tapes:
dd if=/dev/nst0 bs=65536 | openssl enc -aes-256-cbc -d -salt -pbkdf2 -iter 100000 -pass pass:YOUR_KEY | tar xvf - -C /tmp/ --wildcards '*tapebackarr-db*'

# The extracted .sql file can be imported into a fresh TapeBackarr database
```

### Using TapeBackarr's Built-in Recovery

TapeBackarr includes a tape scanning endpoint that can discover database backups:

```bash
# Via API (if TapeBackarr is running with a fresh database):
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:8080/api/v1/drives/1/scan-for-db-backup

# This will scan the tape, find any database backup files, and offer to restore them
```

## Document Version

- Version: 1.2
- Last Updated: February 2026
- Applies to: TapeBackarr 0.x, LTO-5 through LTO-9 drives
- Added: Compressed tape recovery procedures, database backup recovery from tape
