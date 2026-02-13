package tape

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LTFSDefaultMountPoint is the default directory where LTFS tapes are mounted.
const LTFSDefaultMountPoint = "/mnt/ltfs"

// driveStabilizationDelay is the delay after a tape rewind completes to allow
// the drive's internal mechanics to fully settle before performing read/write
// operations. This prevents I/O errors that can occur if operations are attempted
// while the drive is still stabilizing (e.g., head positioning, tension adjustment).
// The 1-second delay is conservative and works across various LTO drive models.
const driveStabilizationDelay = 1 * time.Second

// cartridgeReloadDelay is the delay between ejecting a tape (rewoffl) and
// reloading it (load) during a post-format cartridge reload. Some drives
// report "not ready" if the load is issued too quickly after the eject
// because the mechanical eject cycle has not fully completed. The 5-second
// pause is conservative and accommodates slower LTO drive models.
const cartridgeReloadDelay = 5 * time.Second

// LTFSMetadataFile is the filename written to the root of LTFS volumes for
// TapeBackarr identification. Defined as a constant for consistency across
// the codebase.
const LTFSMetadataFile = ".tapebackarr.json"

// LTFSService provides LTFS (Linear Tape File System) operations for tape drives.
// LTFS makes tapes self-describing by storing data as a standard POSIX filesystem,
// allowing any LTFS-compatible tool to read the tape without needing an external
// database or catalog.
//
// Key benefits of LTFS over raw tar streaming:
//   - Self-describing: each tape contains its own filesystem index
//   - Interoperable: tapes readable on any system with LTFS software
//   - Standard access: files accessible via normal filesystem operations
//   - Dual-partition: index on partition 0, data on partition 1 for fast lookups
//
// Requires LTO-5 or later drives and LTFS software (mkltfs, ltfs commands).
// Inspired by github.com/samuelncui/yatm which uses LTFS for tape management.
type LTFSService struct {
	devicePath string
	mountPoint string
}

// NewLTFSService creates a new LTFS service for the given tape device.
// mountPoint is the directory where LTFS tapes will be mounted; if empty,
// LTFSDefaultMountPoint is used.
func NewLTFSService(devicePath string, mountPoint string) *LTFSService {
	if mountPoint == "" {
		mountPoint = LTFSDefaultMountPoint
	}
	return &LTFSService{
		devicePath: devicePath,
		mountPoint: mountPoint,
	}
}

// DevicePath returns the configured device path.
func (l *LTFSService) DevicePath() string {
	return l.devicePath
}

// MountPoint returns the configured mount point.
func (l *LTFSService) MountPoint() string {
	return l.mountPoint
}

// IsAvailable checks whether the LTFS utilities (mkltfs, ltfs) are installed
// and accessible on the system PATH.
func IsAvailable() bool {
	_, mkErr := exec.LookPath("mkltfs")
	_, ltfsErr := exec.LookPath("ltfs")
	return mkErr == nil && ltfsErr == nil
}

// Format formats the tape in the drive with the LTFS filesystem.
// This erases all data on the tape. The optional label parameter sets the
// volume name stored in the LTFS index (max 6 characters for LTO barcodes).
//
// The --force flag is used to ensure formatting proceeds without an
// interactive confirmation prompt, which would cause the command to hang
// or silently skip formatting when run non-interactively.
//
// After a successful format the tape cartridge is ejected and reloaded to
// force the drive firmware to reinitialize its internal state (block size,
// mode pages, partition map). A simple rewind is not sufficient because
// some drives cache stale parameters that cause I/O errors when ltfsck or
// ltfs mount tries to read the freshly written LTFS structures.
//
// Equivalent to: mkltfs -d /dev/nst0 --force [-n label]
func (l *LTFSService) Format(ctx context.Context, label string) error {
	args := []string{"-d", l.devicePath, "--force"}
	if label != "" {
		args = append(args, "-n", label)
	}

	cmd := exec.CommandContext(ctx, "mkltfs", args...)
	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))
	if err != nil {
		return fmt.Errorf("mkltfs failed: %s: %w", outStr, err)
	}

	// Verify mkltfs output contains the success indicator.
	// Without this check a non-interactive run that silently skips
	// the actual format could be mistaken for success.
	if !ltfsFormatSuccessful(outStr) {
		return fmt.Errorf("mkltfs did not report success (possible confirmation prompt issue): %s", outStr)
	}

	// After mkltfs finishes, the tape drive's SG_IO state (block size,
	// mode settings, partition pointers) may be inconsistent with what
	// ltfsck or ltfs mount expects.  A simple rewind is not sufficient
	// because the drive firmware caches the old mode parameters.
	//
	// Performing a full cartridge reload (rewoffl → load → rewind) forces
	// the drive to re-initialize, clearing stale state that would
	// otherwise cause SG_IO ioctl READ errors (EINVAL / -21700) during
	// the subsequent ltfsck verification.
	if err := l.reloadCartridge(ctx); err != nil {
		return fmt.Errorf("post-format cartridge reload failed: %w", err)
	}

	return nil
}

// ltfsFormatSuccessful returns true when the mkltfs combined output
// contains the LTFS success message.  IBM LTFS emits the log code
// "LTFS15024I" while some open-source builds use the plain text
// "volume formatted successfully".  We check for both variants.
func ltfsFormatSuccessful(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "ltfs15024i") ||
		strings.Contains(lower, "volume formatted successfully")
}

// rewindTape rewinds the tape to the beginning using the mt utility and waits
// for the rewind operation to complete. The mt rewind command can return before
// the physical rewind is done, so this function polls the tape status to confirm
// the tape has reached BOT (Beginning of Tape) before returning.
func (l *LTFSService) rewindTape(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", l.devicePath, "rewind")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rewind failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Wait for the rewind to complete by polling the tape status.
	// The rewind command can return before the tape physically reaches BOT.
	return l.waitForRewindComplete(ctx)
}

// waitForRewindComplete polls the tape status until the tape reaches BOT
// (Beginning of Tape) and is no longer rewinding. This ensures the tape is
// ready for subsequent operations like ltfsck or mount.
func (l *LTFSService) waitForRewindComplete(ctx context.Context) error {
	const maxAttempts = 30
	const pollInterval = 2 * time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check tape status
		cmd := exec.CommandContext(ctx, "mt", "-f", l.devicePath, "status")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Status command failed, wait and retry
			time.Sleep(pollInterval)
			continue
		}

		statusStr := strings.ToLower(string(output))

		// Check if the tape is at BOT and not actively rewinding
		atBOT := strings.Contains(statusStr, "bot") || strings.Contains(statusStr, "beginning of tape")
		isRewinding := strings.Contains(statusStr, "rewinding")

		if atBOT && !isRewinding {
			// Tape has reached BOT and is no longer rewinding.
			// Add a delay to let the drive fully stabilize before returning.
			time.Sleep(driveStabilizationDelay)
			return nil
		}

		// Not ready yet, wait and poll again
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for tape rewind to complete after %d attempts", maxAttempts)
}

// reloadCartridge ejects and reloads the tape cartridge to force the drive
// firmware to re-initialize its internal state (block size, mode pages,
// partition map). This is necessary after mkltfs because some drives
// (notably Tandberg/Quantum LTO) cache stale mode parameters that cause
// SG_IO READ ioctl failures (EINVAL / error -21700) when ltfsck or ltfs
// mount attempts to read the freshly written LTFS structures.
//
// After the reload, the drive block size is explicitly set to variable
// mode (0) because LTFS uses variable-length blocks. Some drives retain
// a stale fixed block size after mkltfs, which causes SG_IO READ ioctl
// failures (EINVAL / -21700) when ltfsck performs its full medium
// consistency check.
//
// Sequence: mt rewoffl → mt load → rewind + wait-for-BOT + stabilize → mt setblk 0.
func (l *LTFSService) reloadCartridge(ctx context.Context) error {
	// Rewind-offline ejects the tape.
	cmd := exec.CommandContext(ctx, "mt", "-f", l.devicePath, "rewoffl")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rewoffl failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Allow the drive mechanics to fully complete the eject before
	// issuing the load. Without this pause some drives report "not
	// ready" on the subsequent load command.
	time.Sleep(cartridgeReloadDelay)

	// Reload the cartridge so the drive re-reads partition labels from
	// scratch with a clean internal state.
	cmd = exec.CommandContext(ctx, "mt", "-f", l.devicePath, "load")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("load after rewoffl failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Rewind to BOT and wait for the tape to be fully settled.
	if err := l.rewindTape(ctx); err != nil {
		return err
	}

	// Set the drive to variable block size mode. LTFS uses variable-length
	// blocks, but some drives retain a stale fixed block size after mkltfs.
	cmd = exec.CommandContext(ctx, "mt", "-f", l.devicePath, "setblk", "0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setblk 0 after reload failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// Mount mounts the LTFS tape at the configured mount point.
// The mount point directory is created if it does not exist.
//
// If the mount fails because partition labels cannot be read (a common
// symptom of a tape that was not fully formatted), the error message
// includes guidance to reformat the tape.
//
// Equivalent to: ltfs /mnt/ltfs -o devname=/dev/nst0
func (l *LTFSService) Mount(ctx context.Context) error {
	// Ensure mount point directory exists
	if err := os.MkdirAll(l.mountPoint, 0755); err != nil {
		return fmt.Errorf("failed to create mount point %s: %w", l.mountPoint, err)
	}

	cmd := exec.CommandContext(ctx, "ltfs", l.mountPoint, "-o", "devname="+l.devicePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(output))
		if isLabelReadFailure(outStr) {
			return fmt.Errorf("ltfs mount failed: tape does not contain valid LTFS partition labels — "+
				"reformat the tape with LTFS before mounting: %s: %w", outStr, err)
		}
		return fmt.Errorf("ltfs mount failed: %s: %w", outStr, err)
	}
	return nil
}

// isLabelReadFailure returns true when LTFS output indicates it could not
// read partition labels, which typically means the tape was never properly
// formatted with LTFS (e.g. mkltfs ran without --force and skipped the
// actual format).
func isLabelReadFailure(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "cannot read volume") ||
		strings.Contains(lower, "failed to read partition labels") ||
		strings.Contains(lower, "failed to read label") ||
		strings.Contains(lower, "cannot read ansi label")
}

// Unmount cleanly unmounts the LTFS tape. This writes the final index to the
// tape and ensures all data is flushed. Uses fusermount if available, falling
// back to umount.
func (l *LTFSService) Unmount(ctx context.Context) error {
	// Try fusermount first (LTFS uses FUSE)
	if _, err := exec.LookPath("fusermount"); err == nil {
		cmd := exec.CommandContext(ctx, "fusermount", "-u", l.mountPoint)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		// Fall through to umount on fusermount failure
		_ = output
	}

	cmd := exec.CommandContext(ctx, "umount", l.mountPoint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ltfs unmount failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// IsMounted checks whether the LTFS mount point is currently mounted by
// looking for it in /proc/mounts.
func (l *LTFSService) IsMounted() bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	// LTFS mounts appear as "ltfs <mountpoint> fuse ..."
	return strings.Contains(string(data), l.mountPoint)
}

// LTFSVolumeInfo contains metadata about a mounted LTFS volume.
type LTFSVolumeInfo struct {
	MountPoint string `json:"mount_point"`
	DevicePath string `json:"device_path"`
	Mounted    bool   `json:"mounted"`
	// Fields populated when mounted
	VolumeName      string `json:"volume_name,omitempty"`
	UsedBytes       int64  `json:"used_bytes,omitempty"`
	AvailBytes      int64  `json:"available_bytes,omitempty"`
	TotalFiles      int64  `json:"total_files,omitempty"`
	TotalDirs       int64  `json:"total_dirs,omitempty"`
	FormatTime      string `json:"format_time,omitempty"`
	LTFSVersion     string `json:"ltfs_version,omitempty"`
	BlockSize       int64  `json:"block_size,omitempty"`
	IndexGeneration int    `json:"index_generation,omitempty"`
}

// GetVolumeInfo returns information about the LTFS volume. If the tape is
// mounted, filesystem statistics are included. This never returns an error;
// unavailable fields are left at their zero values.
func (l *LTFSService) GetVolumeInfo(ctx context.Context) *LTFSVolumeInfo {
	info := &LTFSVolumeInfo{
		MountPoint: l.mountPoint,
		DevicePath: l.devicePath,
		Mounted:    l.IsMounted(),
	}

	if !info.Mounted {
		return info
	}

	// Read volume name from LTFS extended attribute if available
	cmd := exec.CommandContext(ctx, "getfattr", "-n", "ltfs.volumeName", "--only-values", l.mountPoint)
	if output, err := cmd.Output(); err == nil {
		info.VolumeName = strings.TrimSpace(string(output))
	}

	// Get filesystem usage via df
	cmd = exec.CommandContext(ctx, "df", "-B1", l.mountPoint)
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				fmt.Sscanf(fields[2], "%d", &info.UsedBytes)
				fmt.Sscanf(fields[3], "%d", &info.AvailBytes)
			}
		}
	}

	// Get LTFS version
	cmd = exec.CommandContext(ctx, "ltfs", "--version")
	if output, err := cmd.CombinedOutput(); err == nil {
		info.LTFSVersion = strings.TrimSpace(string(output))
	}

	return info
}

// WriteFiles copies a list of files to the mounted LTFS volume, preserving
// relative paths under the given source base directory. Files are written in
// the order provided (caller should pre-sort by path for optimal sequential
// reads from the source, following the github.com/samuelncui/acp approach).
//
// The optional progressCb is invoked after each file with the cumulative bytes
// written so far.
//
// Returns the total bytes written and the number of files copied.
func (l *LTFSService) WriteFiles(ctx context.Context, sourcePath string, files []string, progressCb func(bytesWritten int64)) (totalBytes int64, fileCount int64, err error) {
	if !l.IsMounted() {
		return 0, 0, fmt.Errorf("LTFS volume not mounted at %s", l.mountPoint)
	}

	for _, filePath := range files {
		select {
		case <-ctx.Done():
			return totalBytes, fileCount, ctx.Err()
		default:
		}

		relPath, relErr := filepath.Rel(sourcePath, filePath)
		if relErr != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to compute relative path for %s: %w", filePath, relErr)
		}

		destPath := filepath.Join(l.mountPoint, relPath)

		// Ensure destination directory exists
		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to create directory %s: %w", destDir, err)
		}

		// Copy file
		n, err := copyFile(filePath, destPath)
		if err != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to copy %s to LTFS: %w", relPath, err)
		}

		totalBytes += n
		fileCount++

		if progressCb != nil {
			progressCb(totalBytes)
		}
	}

	return totalBytes, fileCount, nil
}

// ReadFile reads a file from the mounted LTFS volume. Returns the file content
// or an error if the volume is not mounted or the file does not exist.
func (l *LTFSService) ReadFile(ctx context.Context, relativePath string) ([]byte, error) {
	if !l.IsMounted() {
		return nil, fmt.Errorf("LTFS volume not mounted at %s", l.mountPoint)
	}

	fullPath := filepath.Join(l.mountPoint, relativePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s from LTFS: %w", relativePath, err)
	}
	return data, nil
}

// ListFiles returns all files on the mounted LTFS volume with their sizes.
// Paths are relative to the mount point.
func (l *LTFSService) ListFiles(ctx context.Context) ([]LTFSFileEntry, error) {
	if !l.IsMounted() {
		return nil, fmt.Errorf("LTFS volume not mounted at %s", l.mountPoint)
	}

	var entries []LTFSFileEntry
	err := filepath.Walk(l.mountPoint, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(l.mountPoint, path)
		entries = append(entries, LTFSFileEntry{
			Path:    relPath,
			Size:    info.Size(),
			Mode:    int(info.Mode()),
			ModTime: info.ModTime(),
		})
		return nil
	})
	return entries, err
}

// LTFSFileEntry represents a file found on an LTFS volume.
type LTFSFileEntry struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    int       `json:"mode"`
	ModTime time.Time `json:"mod_time"`
}

// RestoreFiles copies files from the mounted LTFS volume to a destination
// directory, preserving relative paths. If filePaths is empty, all files are
// restored.
func (l *LTFSService) RestoreFiles(ctx context.Context, destPath string, filePaths []string) (totalBytes int64, fileCount int64, err error) {
	if !l.IsMounted() {
		return 0, 0, fmt.Errorf("LTFS volume not mounted at %s", l.mountPoint)
	}

	// If no specific files requested, restore everything
	if len(filePaths) == 0 {
		entries, listErr := l.ListFiles(ctx)
		if listErr != nil {
			return 0, 0, fmt.Errorf("failed to list LTFS files: %w", listErr)
		}
		for _, e := range entries {
			filePaths = append(filePaths, e.Path)
		}
	}

	for _, relPath := range filePaths {
		select {
		case <-ctx.Done():
			return totalBytes, fileCount, ctx.Err()
		default:
		}

		srcPath := filepath.Join(l.mountPoint, relPath)
		dstPath := filepath.Join(destPath, relPath)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to create directory for %s: %w", relPath, err)
		}

		n, err := copyFile(srcPath, dstPath)
		if err != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to restore %s: %w", relPath, err)
		}

		totalBytes += n
		fileCount++
	}

	return totalBytes, fileCount, nil
}

// Check runs ltfsck (LTFS consistency check) on the tape device.
// This is useful for verifying tape integrity after unexpected unmounts
// or after formatting.
//
// If the check fails because the tape has no valid LTFS labels, the error
// message includes guidance to reformat the tape.
func (l *LTFSService) Check(ctx context.Context) error {
	if _, err := exec.LookPath("ltfsck"); err != nil {
		return fmt.Errorf("ltfsck not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ltfsck", l.devicePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(output))
		if isLabelReadFailure(outStr) {
			return fmt.Errorf("ltfsck failed: tape does not contain valid LTFS partition labels — "+
				"reformat the tape with LTFS: %s: %w", outStr, err)
		}
		return fmt.Errorf("ltfsck failed: %s: %w", outStr, err)
	}
	return nil
}

// ltfsckMaxRetries is the maximum number of cartridge-reload retries
// attempted by CheckWithRetry when ltfsck fails due to transient SG_IO
// ioctl errors. Some drives (notably Tandberg/Quantum LTO) need more
// than one reload cycle for the firmware to fully clear cached state
// after mkltfs.
const ltfsckMaxRetries = 2

// CheckWithRetry runs ltfsck and, if it fails due to transient SG_IO
// ioctl errors, performs a cartridge reload and retries up to
// ltfsckMaxRetries times. Some LTO drives (notably Tandberg/Quantum)
// cache stale mode parameters that persist across a single cartridge
// reload, causing SG_IO READ ioctl failures (EINVAL / -21700) during
// the full medium consistency check. An additional reload cycle clears
// the residual state.
//
// Non-transient failures (e.g. missing partition labels) are returned
// immediately without retrying.
func (l *LTFSService) CheckWithRetry(ctx context.Context) error {
	var lastErr error
	for attempt := 0; attempt <= ltfsckMaxRetries; attempt++ {
		if attempt > 0 {
			// Reload the cartridge before retrying to clear stale drive state.
			if err := l.reloadCartridge(ctx); err != nil {
				return fmt.Errorf("cartridge reload before ltfsck retry failed: %w", err)
			}
		}

		lastErr = l.Check(ctx)
		if lastErr == nil {
			return nil
		}

		// Only retry on transient SG_IO ioctl errors. Label-read
		// failures and other structural errors are not transient.
		if !isSGIOError(lastErr.Error()) {
			return lastErr
		}
	}
	return lastErr
}

// isSGIOError returns true when the output contains indicators of a
// transient SG_IO ioctl failure or its consequences. These errors occur
// when the drive firmware has stale cached state after mkltfs and are
// resolved by ejecting, reloading the cartridge, and resetting the block
// size to variable mode.
//
// Typical indicators:
//   - "SG_IO ioctl" — the kernel-level SCSI generic ioctl failed
//   - "ioctl error" — LTFS backend reports an ioctl-level failure
//   - "-21700" — LTFS internal error code for an ioctl error
//   - "no index found in the medium" — consequence of failed reads due to
//     stale drive state; ltfsck cannot read the index that mkltfs wrote
//   - "medium consistency check failed" — ltfsck summary when read errors
//     prevent it from verifying the freshly formatted medium
func isSGIOError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "sg_io ioctl") ||
		strings.Contains(lower, "ioctl error") ||
		strings.Contains(lower, "-21700") ||
		strings.Contains(lower, "no index found in the medium") ||
		strings.Contains(lower, "medium consistency check failed")
}

// FormatAndLabel formats the tape with LTFS and writes a TapeBackarr-compatible
// metadata file to the root of the volume so the tape can be identified by both
// LTFS tools and the TapeBackarr label system.
//
// After formatting, an ltfsck verification is performed to ensure the LTFS
// structures were written correctly before attempting to mount the volume.
func (l *LTFSService) FormatAndLabel(ctx context.Context, label string, uuid string, pool string) error {
	// Format the tape with LTFS
	if err := l.Format(ctx, label); err != nil {
		return fmt.Errorf("LTFS format failed: %w", err)
	}

	// Verify the freshly formatted tape with ltfsck before mounting.
	// This catches cases where mkltfs appeared to succeed but did not
	// write valid partition labels (e.g. drive quirks, interrupted I/O).
	// Uses CheckWithRetry because some drives need additional cartridge
	// reload cycles to clear stale firmware state after mkltfs.
	if err := l.CheckWithRetry(ctx); err != nil {
		return fmt.Errorf("LTFS post-format verification failed: %w", err)
	}

	// Mount the freshly formatted tape
	if err := l.Mount(ctx); err != nil {
		return fmt.Errorf("LTFS mount after format failed: %w", err)
	}

	// Write TapeBackarr metadata file to LTFS root
	metaObj := LTFSLabel{
		Magic:     "TAPEBACKARR_LTFS",
		Version:   1,
		Label:     label,
		UUID:      uuid,
		Pool:      pool,
		Format:    "ltfs",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	metadata, err := json.MarshalIndent(metaObj, "", "  ")
	if err != nil {
		l.Unmount(ctx)
		return fmt.Errorf("failed to marshal LTFS metadata: %w", err)
	}

	metadataPath := filepath.Join(l.mountPoint, LTFSMetadataFile)
	if err := os.WriteFile(metadataPath, metadata, 0644); err != nil {
		l.Unmount(ctx)
		return fmt.Errorf("failed to write metadata to LTFS: %w", err)
	}

	// Unmount to finalize
	if err := l.Unmount(ctx); err != nil {
		return fmt.Errorf("LTFS unmount after labeling failed: %w", err)
	}

	return nil
}

// ReadLTFSLabel reads the TapeBackarr metadata from a mounted LTFS volume.
// Returns nil if no metadata file is found.
func (l *LTFSService) ReadLTFSLabel(ctx context.Context) (*LTFSLabel, error) {
	if !l.IsMounted() {
		return nil, fmt.Errorf("LTFS volume not mounted at %s", l.mountPoint)
	}

	metadataPath := filepath.Join(l.mountPoint, LTFSMetadataFile)
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read LTFS label: %w", err)
	}

	var label LTFSLabel
	if err := json.Unmarshal(data, &label); err != nil {
		return nil, fmt.Errorf("failed to parse LTFS label: %w", err)
	}

	return &label, nil
}

// LTFSLabel is the TapeBackarr metadata written to LTFS volumes.
type LTFSLabel struct {
	Magic     string `json:"magic"`
	Version   int    `json:"version"`
	Label     string `json:"label"`
	UUID      string `json:"uuid"`
	Pool      string `json:"pool"`
	Format    string `json:"format"`
	CreatedAt string `json:"created_at"`
}

// EncryptedFileSuffix is appended to files written with per-file encryption
// on LTFS volumes so they can be identified during restore.
const EncryptedFileSuffix = ".enc"

// WriteFilesEncrypted copies files to the mounted LTFS volume with per-file
// AES-256-GCM encryption. Each file is encrypted individually so that single
// files can be decrypted during restore without needing the full volume.
// Encrypted files are stored with the ".enc" suffix.
func (l *LTFSService) WriteFilesEncrypted(ctx context.Context, sourcePath string, files []string, encryptionKey []byte, progressCb func(bytesWritten int64)) (totalBytes int64, fileCount int64, err error) {
	if !l.IsMounted() {
		return 0, 0, fmt.Errorf("LTFS volume not mounted at %s", l.mountPoint)
	}

	for _, filePath := range files {
		select {
		case <-ctx.Done():
			return totalBytes, fileCount, ctx.Err()
		default:
		}

		relPath, relErr := filepath.Rel(sourcePath, filePath)
		if relErr != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to compute relative path for %s: %w", filePath, relErr)
		}

		destPath := filepath.Join(l.mountPoint, relPath+EncryptedFileSuffix)

		destDir := filepath.Dir(destPath)
		if mkErr := os.MkdirAll(destDir, 0755); mkErr != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to create directory %s: %w", destDir, mkErr)
		}

		n, copyErr := copyFileEncrypted(filePath, destPath, encryptionKey)
		if copyErr != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to encrypt and copy %s to LTFS: %w", relPath, copyErr)
		}

		totalBytes += n
		fileCount++

		if progressCb != nil {
			progressCb(totalBytes)
		}
	}

	return totalBytes, fileCount, nil
}

// RestoreFilesDecrypted copies files from the mounted LTFS volume to a
// destination directory, decrypting per-file encrypted files (those ending
// with ".enc") using the provided key.
func (l *LTFSService) RestoreFilesDecrypted(ctx context.Context, destPath string, filePaths []string, encryptionKey []byte) (totalBytes int64, fileCount int64, err error) {
	if !l.IsMounted() {
		return 0, 0, fmt.Errorf("LTFS volume not mounted at %s", l.mountPoint)
	}

	if len(filePaths) == 0 {
		entries, listErr := l.ListFiles(ctx)
		if listErr != nil {
			return 0, 0, fmt.Errorf("failed to list LTFS files: %w", listErr)
		}
		for _, e := range entries {
			filePaths = append(filePaths, e.Path)
		}
	}

	for _, relPath := range filePaths {
		select {
		case <-ctx.Done():
			return totalBytes, fileCount, ctx.Err()
		default:
		}

		srcPath := filepath.Join(l.mountPoint, relPath)

		// Determine the output file name — strip .enc suffix if present
		outRelPath := relPath
		if strings.HasSuffix(relPath, EncryptedFileSuffix) {
			outRelPath = strings.TrimSuffix(relPath, EncryptedFileSuffix)
		}
		dstPath := filepath.Join(destPath, outRelPath)

		if mkErr := os.MkdirAll(filepath.Dir(dstPath), 0755); mkErr != nil {
			return totalBytes, fileCount, fmt.Errorf("failed to create directory for %s: %w", outRelPath, mkErr)
		}

		if strings.HasSuffix(relPath, EncryptedFileSuffix) && encryptionKey != nil {
			n, decErr := copyFileDecrypted(srcPath, dstPath, encryptionKey)
			if decErr != nil {
				return totalBytes, fileCount, fmt.Errorf("failed to decrypt and restore %s: %w", relPath, decErr)
			}
			totalBytes += n
		} else {
			n, cpErr := copyFile(srcPath, dstPath)
			if cpErr != nil {
				return totalBytes, fileCount, fmt.Errorf("failed to restore %s: %w", relPath, cpErr)
			}
			totalBytes += n
		}

		fileCount++
	}

	return totalBytes, fileCount, nil
}

// copyFileEncrypted encrypts src with AES-256-GCM and writes the ciphertext
// to dst. The format is: [12-byte nonce][ciphertext+tag]. Returns the number
// of bytes in the original plaintext file.
func copyFileEncrypted(src, dst string, key []byte) (int64, error) {
	plaintext, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(cryptoRand.Reader, nonce); err != nil {
		return 0, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	if err := os.WriteFile(dst, ciphertext, 0600); err != nil {
		return 0, err
	}

	return int64(len(plaintext)), nil
}

// copyFileDecrypted reads an AES-256-GCM encrypted file and writes the
// plaintext to dst. Returns the number of plaintext bytes written.
func copyFileDecrypted(src, dst string, key []byte) (int64, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return 0, fmt.Errorf("encrypted file too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, fmt.Errorf("decryption failed: %w", err)
	}

	if err := os.WriteFile(dst, plaintext, 0600); err != nil {
		return 0, err
	}

	return int64(len(plaintext)), nil
}

// copyFile copies a single file from src to dst. Returns bytes copied.
func copyFile(src, dst string) (int64, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer dstFile.Close()

	// Use a 1MB buffer for efficient copying, matching the LTO optimal block size.
	buf := make([]byte, 1024*1024)
	total, err := io.CopyBuffer(dstFile, srcFile, buf)
	if err != nil {
		return total, err
	}

	// Preserve file mode from source; chmod failure is non-fatal.
	if info, statErr := srcFile.Stat(); statErr == nil {
		_ = os.Chmod(dst, info.Mode())
	}

	return total, nil
}
