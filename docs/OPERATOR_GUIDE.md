# TapeBackarr Operator Guide

Quick reference guide for operators managing daily tape backup operations.

## Quick Reference Card

### Common Tasks

| Task | Steps |
|------|-------|
| Check system status | Dashboard → View status cards |
| Run manual backup | Jobs → Select job → Run Now |
| Handle tape change | Wait for notification → Swap tape → Acknowledge |
| Restore files | Restore → Search → Select → Insert tape → Restore |
| View recent errors | Logs → Filter by Error level |

### Emergency Contacts

| Issue | Action |
|-------|--------|
| Drive hardware failure | Contact IT support |
| Tape stuck in drive | DO NOT force → Contact IT |
| System not responding | Restart service: `sudo systemctl restart tapebackarr` |

---

## Daily Operations Checklist

### Morning Check (Start of Shift)

- [ ] Check Dashboard for overnight job status
- [ ] Review any failed jobs and errors
- [ ] Verify tape drive is online
- [ ] Check for pending tape change requests
- [ ] Ensure spare tapes are available

### End of Day Check

- [ ] Verify daily backups completed successfully
- [ ] Note any tapes that need to go offsite
- [ ] Check tape pool levels (adequate blanks available)
- [ ] Clear any warnings/alerts

---

## Tape Handling Procedures

### Inserting a Tape

1. **Verify the tape** matches what the system expects
2. **Check for damage** - no visible damage to cartridge
3. **Insert gently** - don't force the tape
4. **Wait for ready** - LED should indicate ready state
5. **Acknowledge** in the web interface if prompted

### Removing a Tape

1. **Wait for operations to complete** - never remove during writes
2. **Use the Eject function** in the web UI or on the drive
3. **Wait for full ejection** before removing
4. **Label the tape** with any notes if needed
5. **Store properly** in case or appropriate storage

### Labeling Best Practices

**Physical Label (on cartridge):**
```
WEEKLY-001
Pool: WEEKLY
Created: 2024-01-15
```

**In TapeBackarr:**
- Use consistent naming: `POOL-NUMBER` (e.g., WEEKLY-001)
- Add notes for special circumstances
- Update status when moving offsite

---

## Handling Tape Changes

### When Tape Change is Requested

You'll receive notification via:
1. **Web Dashboard** - Alert banner appears
2. **Telegram** (if configured) - Push notification

### Step-by-Step Procedure

1. **Read the notification carefully**
   - Note which tape to remove
   - Note which tape to insert

2. **Go to the tape drive**
   - Verify the drive LED shows safe to remove
   - Press the physical eject button OR use web UI

3. **Remove the current tape**
   - Wait for full ejection
   - Store the tape properly

4. **Get the requested tape**
   - If new tape: get a blank from storage
   - If specific tape: locate in tape library

5. **Insert the new tape**
   - Verify it's the correct tape
   - Insert gently until it seats

6. **Acknowledge in Web UI**
   - Go to Dashboard or current job status
   - Click "Tape Changed" or "Acknowledge"

7. **Wait for operation to resume**
   - Monitor status to ensure success

### Tape Change Timeout

If a tape change is not acknowledged within **30 minutes**:
- The operation will be paused
- Additional notifications will be sent
- Job can be resumed once tape is inserted

---

## Multi-Tape Backup Spanning

### Understanding Spanning

When a backup is larger than one tape, it automatically spans:

```
Backup Job: Full-FileServer (500GB)

Tape 1: WEEKLY-001 [||||||||||||||||||||] 400GB ✓
         ↓ (spanning marker)
Tape 2: WEEKLY-002 [||||||||           ] 100GB ✓

Backup Complete!
```

### Spanning Sequence

1. Backup starts on first tape
2. When tape fills, system pauses
3. Notification sent for tape change
4. Operator inserts new tape
5. Operator acknowledges in web UI
6. Backup continues on new tape
7. Process repeats until backup completes

### Tracking Spanning Sets

Each spanning set is tracked with:
- **Set ID**: Links all tapes together
- **Sequence**: 1, 2, 3... order of tapes
- **Markers**: Written on each tape for recovery

---

## Restoring from Tape

### Simple Single-File Restore

1. Navigate to **Restore**
2. Search for the file: `invoice-2024.pdf`
3. Select the file from results
4. Click **Restore**
5. Choose destination path
6. Insert required tape when prompted
7. Wait for restore to complete

### Multi-File Restore

1. Search and select multiple files
2. Review the "Required Tapes" list
3. Note the insertion order
4. Click **Start Restore**
5. Insert tapes in order as prompted
6. Acknowledge each tape change

### Multi-Tape Spanning Restore

When restoring from a spanning set:

```
Required Tapes (in order):
1. WEEKLY-001 (insert first)
2. WEEKLY-002 (will prompt when needed)
3. WEEKLY-003 (will prompt when needed)
```

**Procedure:**
1. Insert first tape in the sequence
2. Click "Continue"
3. System reads data from this tape
4. When prompted, eject and insert next tape
5. Repeat until restore completes

---

## Common Scenarios

### Scenario 1: Daily Backup Completes Successfully

**What you'll see:**
- Dashboard shows "Backup Completed"
- Green status indicator
- Telegram notification (if configured)

**Action required:** None - just verify completion

### Scenario 2: Tape Full During Backup

**What you'll see:**
- Dashboard shows "Waiting for Tape"
- Notification: "Tape WEEKLY-001 full. Insert new tape."

**Action:**
1. Eject current tape
2. Label and store it properly
3. Insert a blank tape from the pool
4. Acknowledge in web UI

### Scenario 3: Wrong Tape Inserted

**What you'll see:**
- Warning: "Wrong tape inserted"
- Shows expected vs actual tape

**Action:**
1. Eject the wrong tape
2. Find and insert the correct tape
3. System will automatically recognize it

### Scenario 4: Backup Job Fails

**What you'll see:**
- Dashboard shows "Backup Failed"
- Red status indicator
- Error message displayed

**Action:**
1. Check the error message
2. Common issues:
   - Source path not accessible → Check network mounts
   - Tape write error → Check tape/drive
   - No tape in drive → Insert tape
3. Fix the issue
4. Re-run the job manually

### Scenario 5: Tape Not Recognized

**Symptoms:**
- Drive shows "No tape" even though one is inserted
- Status shows offline

**Actions to try:**
1. Eject and re-insert the tape
2. Check if tape is damaged
3. Try a different tape
4. Check drive connections
5. Contact IT if problem persists

---

## Tape Pool Management

### Rotation Schedules

**Suggested Rotation:**

| Pool | Tapes | Rotation | Offsite |
|------|-------|----------|---------|
| DAILY | 5 | Monday-Friday | Weekly |
| WEEKLY | 4 | Weekly on Sunday | Monthly |
| MONTHLY | 12 | 1st of month | Quarterly |
| ARCHIVE | As needed | Yearly | Always |

### Moving Tapes Offsite

1. In TapeBackarr: Mark tape as "Offsite"
2. Record offsite location in notes
3. Add to physical offsite log
4. Transport tape properly (protective case)

### Returning Tapes from Offsite

1. Transport tape carefully
2. Inspect for damage
3. In TapeBackarr: Mark as "Returned"
4. Update offsite location to blank
5. Return to rotation pool

---

## Troubleshooting Quick Reference

### Drive Issues

| Symptom | Check | Action |
|---------|-------|--------|
| Drive not detected | Power, cables | Contact IT |
| Tape stuck | Drive status LED | Press eject, don't force |
| Read errors | Tape condition | Try cleaning tape, try different tape |
| Write errors | Write-protect tab | Check tab, try different tape |

### Backup Issues

| Symptom | Check | Action |
|---------|-------|--------|
| Backup not starting | Job enabled? | Enable job |
| Source not found | Mount status | Verify network mount |
| Slow backup | Network, drive | Check for bottlenecks |
| Backup fails midway | Logs | Check specific error |

### Restore Issues

| Symptom | Check | Action |
|---------|-------|--------|
| File not found | Correct backup set? | Search in different timeframe |
| Wrong tape | Label vs catalog | Verify tape barcode |
| Checksum error | Tape damage | Try restore again, may need different backup |

---

## Service Commands

For IT administrators - common service commands:

```bash
# Check service status
sudo systemctl status tapebackarr

# Restart service
sudo systemctl restart tapebackarr

# View live logs
sudo journalctl -u tapebackarr -f

# Check tape drive
mt -f /dev/nst0 status

# Manual tape eject
mt -f /dev/nst0 eject

# Manual tape rewind
mt -f /dev/nst0 rewind
```

---

## Glossary

| Term | Definition |
|------|------------|
| **Backup Set** | A single backup operation, may span multiple tapes |
| **Catalog** | Database of all backed-up files and their locations |
| **File Mark** | Tape marker separating backup sets |
| **Full Backup** | Complete backup of all files |
| **Incremental** | Backup of only changed files since last backup |
| **LTO** | Linear Tape-Open, the tape technology |
| **Pool** | Group of tapes with same purpose (DAILY, WEEKLY, etc.) |
| **Spanning** | Backup continuing across multiple tapes |
| **Tape Label** | Identifier written at the start of the tape |

---

## Safety Reminders

⚠️ **NEVER:**
- Force a tape into or out of the drive
- Remove a tape while the busy LED is on
- Touch the tape media inside the cartridge
- Expose tapes to magnetic fields
- Stack tapes on electronic equipment

✅ **ALWAYS:**
- Handle tapes by the cartridge edges
- Store tapes in protective cases
- Keep tapes in a climate-controlled environment
- Report any tape or drive damage immediately
- Follow the labeling convention

---

## Emergency Procedures

### Power Failure During Backup

1. **Wait** - system should resume when power returns
2. Check Dashboard for job status
3. If needed, re-run the backup

### System Unresponsive

1. Try accessing web UI from different browser/computer
2. Check if server is reachable: `ping server-address`
3. If still unresponsive, contact IT to restart service

### Data Recovery Urgently Needed

1. Note the exact files needed
2. Search catalog for the files
3. Identify required tapes
4. Retrieve tapes from storage (including offsite if needed)
5. Perform restore with verification enabled
6. Confirm restored data is complete
