package tape

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/models"
)

// DefaultOperationTimeout is the default timeout for tape operations like status, rewind, and label read.
// This prevents the application from hanging indefinitely when a tape drive is unresponsive.
const DefaultOperationTimeout = 30 * time.Second

// ErrOperationTimeout is returned when a tape operation times out
var ErrOperationTimeout = errors.New("tape operation timed out")

// DriveStatus represents the current status of a tape drive
type DriveStatus struct {
	DevicePath   string    `json:"device_path"`
	Ready        bool      `json:"ready"`
	Online       bool      `json:"online"`
	WriteProtect bool      `json:"write_protect"`
	BOT          bool      `json:"bot"` // Beginning of Tape
	EOT          bool      `json:"eot"` // End of Tape
	EOF          bool      `json:"eof"` // End of File mark
	FileNumber   int64     `json:"file_number"`
	BlockNumber  int64     `json:"block_number"`
	Density      string    `json:"density"`
	BlockSize    int       `json:"block_size"`
	DriveType    string    `json:"drive_type"`
	LastChecked  time.Time `json:"last_checked"`
	Error        string    `json:"error,omitempty"`
}

// TapeInfo contains information about the loaded tape
type TapeInfo struct {
	Loaded     bool   `json:"loaded"`
	Label      string `json:"label,omitempty"`
	UUID       string `json:"uuid,omitempty"`
	Pool       string `json:"pool,omitempty"`
	WriteCount int    `json:"write_count"`
	Density    string `json:"density,omitempty"`
}

// TapeLabelData represents structured label data written to tape
type TapeLabelData struct {
	Label                    string `json:"label"`
	UUID                     string `json:"uuid"`
	Pool                     string `json:"pool"`
	Timestamp                int64  `json:"timestamp"`
	EncryptionKeyFingerprint string `json:"encryption_key_fingerprint,omitempty"`
	CompressionType          string `json:"compression_type,omitempty"`
}

// TapeContentEntry represents a single file entry from tape contents listing
type TapeContentEntry struct {
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Size        int64  `json:"size"`
	Date        string `json:"date"`
	Path        string `json:"path"`
}

// CachedLabel holds a cached tape label for a drive
type CachedLabel struct {
	Label       *TapeLabelData
	CachedAt    time.Time
	DriveOnline bool
}

// LabelCache provides thread-safe caching of tape labels per device
type LabelCache struct {
	mu    sync.RWMutex
	cache map[string]*CachedLabel
}

// NewLabelCache creates a new label cache
func NewLabelCache() *LabelCache {
	return &LabelCache{
		cache: make(map[string]*CachedLabel),
	}
}

// Get returns the cached label for a device, or nil if not cached or expired
func (lc *LabelCache) Get(devicePath string, maxAge time.Duration) *CachedLabel {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	entry, ok := lc.cache[devicePath]
	if !ok {
		return nil
	}
	if time.Since(entry.CachedAt) > maxAge {
		return nil
	}
	return entry
}

// Set stores a label in the cache
func (lc *LabelCache) Set(devicePath string, label *TapeLabelData, online bool) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.cache[devicePath] = &CachedLabel{
		Label:       label,
		CachedAt:    time.Now(),
		DriveOnline: online,
	}
}

// Invalidate removes the cached label for a device (call on eject/load)
func (lc *LabelCache) Invalidate(devicePath string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	delete(lc.cache, devicePath)
}

// InvalidateAll clears the entire cache
func (lc *LabelCache) InvalidateAll() {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.cache = make(map[string]*CachedLabel)
}

// Service provides tape drive operations
type Service struct {
	devicePath string
	blockSize  int
	labelCache *LabelCache
}

// GetBlockSize returns the configured block size
func (s *Service) GetBlockSize() int {
	return s.blockSize
}

// NewService creates a new tape service
func NewService(devicePath string, blockSize int) *Service {
	return &Service{
		devicePath: devicePath,
		blockSize:  blockSize,
		labelCache: NewLabelCache(),
	}
}

// NewServiceForDevice creates a tape service for a specific device path
func NewServiceForDevice(devicePath string, blockSize int) *Service {
	return &Service{
		devicePath: devicePath,
		blockSize:  blockSize,
		labelCache: NewLabelCache(),
	}
}

// DevicePath returns the configured device path
func (s *Service) DevicePath() string {
	return s.devicePath
}

// GetLabelCache returns the label cache for external use
func (s *Service) GetLabelCache() *LabelCache {
	return s.labelCache
}

// ScanDrives scans the system for available tape drives
func ScanDrives(ctx context.Context) ([]map[string]string, error) {
	drives := make([]map[string]string, 0)

	// Check common tape device paths
	devicePaths := []string{
		"/dev/nst0", "/dev/nst1", "/dev/nst2", "/dev/nst3",
		"/dev/st0", "/dev/st1", "/dev/st2", "/dev/st3",
	}

	for _, path := range devicePaths {
		if _, err := os.Stat(path); err == nil {
			drive := map[string]string{
				"device_path": path,
				"status":      "detected",
			}

			// Try to get drive info
			svc := NewServiceForDevice(path, 65536)
			if info, err := svc.GetDriveInfo(ctx); err == nil {
				if v, ok := info["Vendor identification"]; ok {
					drive["vendor"] = v
				}
				if v, ok := info["Product identification"]; ok {
					drive["model"] = v
				}
				if v, ok := info["Unit serial number"]; ok {
					drive["serial_number"] = v
				}
			}

			// Check if drive is online
			if status, err := svc.GetStatus(ctx); err == nil {
				if status.Online {
					drive["status"] = "online"
				}
			}

			drives = append(drives, drive)
		}
	}

	// Also try lsscsi for more comprehensive detection
	cmd := exec.CommandContext(ctx, "lsscsi", "-g")
	if output, err := cmd.CombinedOutput(); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "tape") || strings.Contains(line, "mediumx") {
				// Parse lsscsi output: [H:C:T:L] type vendor model rev device
				fields := strings.Fields(line)
				if len(fields) >= 6 {
					devPath := fields[len(fields)-1]
					if devPath != "-" {
						// Check if we already have this drive
						found := false
						for _, d := range drives {
							if d["device_path"] == devPath {
								found = true
								break
							}
						}
						if !found {
							drive := map[string]string{
								"device_path": devPath,
								"status":      "detected",
							}
							if len(fields) >= 4 {
								drive["vendor"] = fields[2]
								drive["model"] = fields[3]
							}
							drives = append(drives, drive)
						} else {
							// Merge lsscsi data into existing drive if fields are missing
							for _, d := range drives {
								if d["device_path"] == devPath {
									if d["vendor"] == "" && len(fields) >= 3 {
										d["vendor"] = fields[2]
									}
									if d["model"] == "" && len(fields) >= 4 {
										d["model"] = fields[3]
									}
									break
								}
							}
						}
					}
				}
			}
		}
	}

	return drives, nil
}

// GetStatus returns the current status of the tape drive.
// It enforces a timeout to prevent indefinite blocking when the drive is unresponsive.
func (s *Service) GetStatus(ctx context.Context) (*DriveStatus, error) {
	status := &DriveStatus{
		DevicePath:  s.devicePath,
		LastChecked: time.Now(),
	}

	// Create a context with timeout to prevent indefinite blocking
	opCtx, cancel := context.WithTimeout(ctx, DefaultOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(opCtx, "mt", "-f", s.devicePath, "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error was due to context timeout/cancellation
		if opCtx.Err() == context.DeadlineExceeded {
			status.Error = fmt.Sprintf("tape status operation timed out after %v", DefaultOperationTimeout)
			return status, ErrOperationTimeout
		}
		if opCtx.Err() == context.Canceled {
			status.Error = "tape status operation cancelled"
			return status, ctx.Err()
		}
		status.Error = fmt.Sprintf("failed to get tape status: %s", string(output))
		return status, nil
	}

	// Parse mt status output
	outputStr := string(output)
	status.Online = !strings.Contains(outputStr, "offline")
	status.Ready = strings.Contains(outputStr, "ONLINE") || strings.Contains(outputStr, "DR_OPEN")
	status.WriteProtect = strings.Contains(outputStr, "WR_PROT")
	status.BOT = strings.Contains(outputStr, "BOT")
	status.EOT = strings.Contains(outputStr, "EOT")
	status.EOF = strings.Contains(outputStr, "EOF")

	// Parse file and block numbers
	fileNumRe := regexp.MustCompile(`File number=(\d+)`)
	if matches := fileNumRe.FindStringSubmatch(outputStr); len(matches) > 1 {
		status.FileNumber, _ = strconv.ParseInt(matches[1], 10, 64)
	}

	blockNumRe := regexp.MustCompile(`block number=(\d+)`)
	if matches := blockNumRe.FindStringSubmatch(outputStr); len(matches) > 1 {
		status.BlockNumber, _ = strconv.ParseInt(matches[1], 10, 64)
	}

	// Parse density
	densityRe := regexp.MustCompile(`Tape block size (\d+) bytes\. Density code (0x[0-9a-fA-F]+)`)
	if matches := densityRe.FindStringSubmatch(outputStr); len(matches) > 2 {
		status.BlockSize, _ = strconv.Atoi(matches[1])
		status.Density = matches[2]
	}

	// Parse LTO type from density description (e.g., "Density code 0x58 (LTO-5).")
	ltoDescRe := regexp.MustCompile(`Density code 0x[0-9a-fA-F]+ \((LTO-\d+)\)`)
	if matches := ltoDescRe.FindStringSubmatch(outputStr); len(matches) > 1 {
		status.DriveType = matches[1]
	}

	return status, nil
}

// Rewind rewinds the tape to the beginning.
// It enforces a timeout to prevent indefinite blocking when the drive is unresponsive.
func (s *Service) Rewind(ctx context.Context) error {
	// Create a context with timeout to prevent indefinite blocking
	opCtx, cancel := context.WithTimeout(ctx, DefaultOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(opCtx, "mt", "-f", s.devicePath, "rewind")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error was due to context timeout/cancellation
		if opCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("rewind timed out after %v: %w", DefaultOperationTimeout, ErrOperationTimeout)
		}
		if opCtx.Err() == context.Canceled {
			return fmt.Errorf("rewind cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("rewind failed: %s", string(output))
	}
	return nil
}

// Eject ejects the tape from the drive
func (s *Service) Eject(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "eject")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("eject failed: %s", string(output))
	}
	if s.labelCache != nil {
		s.labelCache.Invalidate(s.devicePath)
	}
	return nil
}

// Load loads a tape (if autoloader is available)
func (s *Service) Load(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "load")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("load failed: %s", string(output))
	}
	if s.labelCache != nil {
		s.labelCache.Invalidate(s.devicePath)
	}
	return nil
}

// Retension runs a tape retension pass (full wind/rewind cycle)
func (s *Service) Retension(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "retension")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("retension failed: %s", string(output))
	}
	return nil
}

// SeekToFileNumber positions the tape at the specified file mark
func (s *Service) SeekToFileNumber(ctx context.Context, fileNum int64) error {
	// First rewind
	if err := s.Rewind(ctx); err != nil {
		return err
	}

	if fileNum == 0 {
		return nil
	}

	// Forward space to file number
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "fsf", strconv.FormatInt(fileNum, 10))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("seek failed: %s", string(output))
	}
	return nil
}

// GetTapePosition returns the current file number and block number of the tape head
// by querying the drive status via mt.
func (s *Service) GetTapePosition(ctx context.Context) (fileNumber, blockNumber int64, err error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get tape status: %w", err)
	}
	if status.Error != "" {
		return 0, 0, fmt.Errorf("tape status error: %s", status.Error)
	}
	return status.FileNumber, status.BlockNumber, nil
}

// SeekToBlock positions the tape at the specified block
func (s *Service) SeekToBlock(ctx context.Context, blockNum int64) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "seek", strconv.FormatInt(blockNum, 10))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("seek to block failed: %s", string(output))
	}
	return nil
}

// WriteFileMark writes a file mark on the tape
func (s *Service) WriteFileMark(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "weof", "1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write file mark failed: %s", string(output))
	}
	return nil
}

const (
	// vpdHeaderSize is the number of bytes to skip in VPD page 80 before the serial number
	vpdHeaderSize = 4
)

// SetBlockSize sets the tape block size
func (s *Service) SetBlockSize(ctx context.Context, size int) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "setblk", strconv.Itoa(size))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("set block size failed: %s", string(output))
	}
	return nil
}

const (
	// labelMagic is the identifier prefix for TapeBackarr labels
	labelMagic = "TAPEBACKARR"
	// labelDelimiter separates fields in the label block
	labelDelimiter = "|"
)

const (
	// tocMagic is the identifier for TapeBackarr TOC entries
	tocMagic = "TAPEBACKARR_TOC"
	// tocVersion is the current TOC format version
	tocVersion = 1
	// tocBlockSize is the block size for TOC reads/writes (64KB)
	tocBlockSize = 65536
)

// ReadTapeLabel reads the label from the beginning of the tape.
// It enforces a timeout to prevent indefinite blocking when the drive is unresponsive.
func (s *Service) ReadTapeLabel(ctx context.Context) (*TapeLabelData, error) {
	// Rewind to beginning (already has its own timeout)
	if err := s.Rewind(ctx); err != nil {
		return nil, err
	}

	// Set variable block size mode so the 512-byte label block can be read
	// regardless of the drive's configured fixed block size.
	if err := s.SetBlockSize(ctx, 0); err != nil {
		return nil, fmt.Errorf("failed to set variable block size for label read: %w", err)
	}
	defer s.SetBlockSize(ctx, s.blockSize)

	// Create a context with timeout for the dd read operation
	opCtx, cancel := context.WithTimeout(ctx, DefaultOperationTimeout)
	defer cancel()

	// Read first block which should contain the label
	cmd := exec.CommandContext(opCtx, "dd", fmt.Sprintf("if=%s", s.devicePath), "bs=512", "count=1")
	output, err := cmd.Output()
	if err != nil {
		// Check if the error was due to context timeout/cancellation
		if opCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("label read timed out after %v: %w", DefaultOperationTimeout, ErrOperationTimeout)
		}
		if opCtx.Err() == context.Canceled {
			return nil, fmt.Errorf("label read cancelled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("failed to read label: %w", err)
	}

	// Strip null bytes from padded block
	raw := strings.TrimRight(string(output), "\x00")
	if raw == "" {
		return nil, nil
	}

	// Parse label (format: "TAPEBACKARR|label|uuid|pool|timestamp")
	parts := strings.Split(raw, labelDelimiter)
	if len(parts) < 2 || parts[0] != labelMagic {
		return nil, nil
	}

	data := &TapeLabelData{
		Label: parts[1],
	}
	if len(parts) >= 3 {
		data.UUID = parts[2]
	}
	if len(parts) >= 4 {
		data.Pool = parts[3]
	}
	if len(parts) >= 5 {
		data.Timestamp, _ = strconv.ParseInt(parts[4], 10, 64)
	}
	if len(parts) >= 6 {
		data.EncryptionKeyFingerprint = parts[5]
	}
	if len(parts) >= 7 {
		data.CompressionType = parts[6]
	}
	return data, nil
}

// WriteTapeLabel writes a label to the beginning of the tape
// Optional metadata parameters: encFingerprint, compressionType
func (s *Service) WriteTapeLabel(ctx context.Context, label string, uuid string, pool string, metadata ...string) error {
	// Rewind to beginning
	if err := s.Rewind(ctx); err != nil {
		return err
	}

	// Set variable block size mode so the 512-byte label block can be written
	// regardless of the drive's configured fixed block size.
	if err := s.SetBlockSize(ctx, 0); err != nil {
		return fmt.Errorf("failed to set variable block size for label write: %w", err)
	}
	defer s.SetBlockSize(ctx, s.blockSize)

	// Create label block with UUID and pool info
	fields := []string{labelMagic, label, uuid, pool, strconv.FormatInt(time.Now().Unix(), 10)}
	if len(metadata) > 0 && metadata[0] != "" {
		fields = append(fields, metadata[0])
	} else if len(metadata) > 1 {
		fields = append(fields, "")
	}
	if len(metadata) > 1 && metadata[1] != "" {
		fields = append(fields, metadata[1])
	}
	labelData := strings.Join(fields, labelDelimiter)
	// Pad to 512 bytes
	padded := make([]byte, 512)
	copy(padded, []byte(labelData))

	// Write label
	cmd := exec.CommandContext(ctx, "dd", fmt.Sprintf("of=%s", s.devicePath), "bs=512", "count=1")
	cmd.Stdin = bytes.NewReader(padded)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to write label: %s", string(output))
	}

	// Write file mark after label
	if err := s.WriteFileMark(ctx); err != nil {
		return err
	}
	// Update cache with newly written label
	if s.labelCache != nil {
		cachedLabel := &TapeLabelData{
			Label:     label,
			UUID:      uuid,
			Pool:      pool,
			Timestamp: time.Now().Unix(),
		}
		if len(metadata) > 0 && metadata[0] != "" {
			cachedLabel.EncryptionKeyFingerprint = metadata[0]
		}
		if len(metadata) > 1 && metadata[1] != "" {
			cachedLabel.CompressionType = metadata[1]
		}
		s.labelCache.Set(s.devicePath, cachedLabel, true)
	}
	return nil
}

// EraseTape erases/formats the tape, removing all data including labels
func (s *Service) EraseTape(ctx context.Context) error {
	// Rewind first
	if err := s.Rewind(ctx); err != nil {
		return err
	}

	// Write end-of-data mark at beginning to effectively erase
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "weof", "1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("erase failed: %s", string(output))
	}

	if s.labelCache != nil {
		s.labelCache.Invalidate(s.devicePath)
	}
	// Rewind again after erase
	return s.Rewind(ctx)
}

// GetDriveInfo returns drive information using sg_inq
func (s *Service) GetDriveInfo(ctx context.Context) (map[string]string, error) {
	info := make(map[string]string)

	// Try to get device info using sg_inq
	cmd := exec.CommandContext(ctx, "sg_inq", s.devicePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// sg_inq might not be available, return empty info
		return info, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			info[key] = value
		}
	}

	// Fallback: try sysfs for vendor/model/serial info
	if info["Vendor identification"] == "" {
		// Try reading from sysfs - extract device name from path
		devName := filepath.Base(s.devicePath)
		// Try common sysfs paths for tape devices
		sysfsBase := fmt.Sprintf("/sys/class/scsi_tape/%s/device", devName)
		if _, err := os.Stat(sysfsBase); err != nil {
			// Try without 'n' prefix (nst0 -> st0)
			if strings.HasPrefix(devName, "n") {
				sysfsBase = fmt.Sprintf("/sys/class/scsi_tape/%s/device", devName[1:])
			}
		}
		if vendor, err := os.ReadFile(filepath.Join(sysfsBase, "vendor")); err == nil {
			info["Vendor identification"] = strings.TrimSpace(string(vendor))
		}
		if model, err := os.ReadFile(filepath.Join(sysfsBase, "model")); err == nil {
			info["Product identification"] = strings.TrimSpace(string(model))
		}
		// Try to find serial from vpd_pg80
		serialPath := filepath.Join(sysfsBase, "vpd_pg80")
		if serial, err := os.ReadFile(serialPath); err == nil {
			// VPD page 80 contains unit serial number in binary - extract printable chars
			serialStr := strings.Map(func(r rune) rune {
				if r >= 32 && r < 127 {
					return r
				}
				return -1
			}, string(serial))
			serialStr = strings.TrimSpace(serialStr)
			if len(serialStr) > vpdHeaderSize {
				// Skip the VPD header bytes
				info["Unit serial number"] = serialStr[vpdHeaderSize:]
			}
		}
	}

	return info, nil
}

// IsTapeLoaded checks if a tape is loaded in the drive
func (s *Service) IsTapeLoaded(ctx context.Context) (bool, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return false, err
	}
	return status.Online && status.Ready && status.Error == "", nil
}

// DetectTapeType detects the LTO type of the tape currently loaded in the drive
// by reading the density code from the mt status output.
// Returns the LTO type string (e.g., "LTO-5") or empty string if detection fails.
func (s *Service) DetectTapeType(ctx context.Context) (string, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return "", err
	}
	if status.Error != "" {
		return "", fmt.Errorf("drive error: %s", status.Error)
	}

	// First try the LTO type parsed from the description in mt output (e.g., "(LTO-5)")
	if status.DriveType != "" {
		return status.DriveType, nil
	}

	// Fall back to looking up the density code in our mapping
	if status.Density != "" {
		if ltoType, ok := models.LTOTypeFromDensity(status.Density); ok {
			return ltoType, nil
		}
	}

	return "", nil
}

// WaitForTape waits for a tape to be loaded
func (s *Service) WaitForTape(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		loaded, err := s.IsTapeLoaded(ctx)
		if err != nil {
			return err
		}
		if loaded {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			// Check again
		}
	}

	return fmt.Errorf("timeout waiting for tape to be loaded")
}

// ListTapeContents lists the contents of a tape using tar, starting from current position.
// It reads at most maxEntries files. If encrypted is true, returns an indicator instead.
func (s *Service) ListTapeContents(ctx context.Context, maxEntries int) ([]TapeContentEntry, error) {
	if maxEntries <= 0 {
		maxEntries = 1000
	}

	// Seek past the label to file number 1
	if err := s.SeekToFileNumber(ctx, 1); err != nil {
		return nil, fmt.Errorf("failed to seek past label: %w", err)
	}

	// Run tar with a timeout
	tarCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tarCtx, "tar", "-tvf", s.devicePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Could be encrypted data or not a tar archive - return empty list
		return []TapeContentEntry{}, nil
	}

	entries := make([]TapeContentEntry, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() && len(entries) < maxEntries {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		permissions := fields[0]
		owner := fields[1]
		size, _ := strconv.ParseInt(fields[2], 10, 64)
		date := fields[3] + " " + fields[4]
		path := strings.Join(fields[5:], " ")

		entries = append(entries, TapeContentEntry{
			Permissions: permissions,
			Owner:       owner,
			Size:        size,
			Date:        date,
			Path:        path,
		})
	}

	return entries, nil
}

// DriveStatisticsData holds parsed drive statistics from tapeinfo/sg_logs
type DriveStatisticsData struct {
	TotalBytesRead      int64   `json:"total_bytes_read"`
	TotalBytesWritten   int64   `json:"total_bytes_written"`
	ReadErrors          int64   `json:"read_errors"`
	WriteErrors         int64   `json:"write_errors"`
	TotalLoadCount      int64   `json:"total_load_count"`
	CleaningRequired    bool    `json:"cleaning_required"`
	PowerOnHours        int64   `json:"power_on_hours"`
	TapeMotionHours     float64 `json:"tape_motion_hours"`
	TemperatureC        int64   `json:"temperature_c"`
	LifetimePowerCycles int64   `json:"lifetime_power_cycles"`
	ReadCompressionPct  int64   `json:"read_compression_pct"`
	WriteCompressionPct int64   `json:"write_compression_pct"`
	TapeAlertFlags      string  `json:"tape_alert_flags"`
}

// ForceClean sends a rewind-offline command to eject the current tape from the drive,
// preparing it for a cleaning cartridge to be loaded. Once a cleaning tape is inserted,
// the drive automatically detects it and initiates a cleaning cycle.
func (s *Service) ForceClean(ctx context.Context) error {
	// rewoffl (rewind-offline) ejects the tape, which is the preparatory step for
	// loading a cleaning cartridge. LTO drives auto-detect cleaning tapes on load.
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "rewoffl")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("force clean failed: %s", string(output))
	}
	if s.labelCache != nil {
		s.labelCache.Invalidate(s.devicePath)
	}
	return nil
}

// TapeTOC represents the Table of Contents written to tape after backup data.
// The TOC is written as the last file section on the tape, after all backup data,
// allowing the tape to be self-describing even without access to the database.
//
// Each tape in a multi-tape backup receives its own TOC containing only the
// files written to that specific tape. The SpanningSetID, SequenceNumber, and
// TotalTapes fields link the tape to the broader spanning set so the full
// backup can be reconstructed from individual tapes.
//
// Tape layout:
//
//	[Label (512B)] [FM] [Backup Data (tar)] [FM] [TOC (JSON)] [FM] [EOD]
//	  File #0             File #1                  File #2
//
// The TOC is written after a rewind to the end of the backup data, and uses a
// fixed 64KB block size. The TOC size depends on the number of files cataloged
// (typically a few KB to several MB for large backup sets). The TOC is padded
// to the nearest 64KB boundary.
type TapeTOC struct {
	Magic          string         `json:"magic"`
	Version        int            `json:"version"`
	TapeLabel      string         `json:"tape_label"`
	TapeUUID       string         `json:"tape_uuid"`
	Pool           string         `json:"pool,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	SpanningSetID  int64          `json:"spanning_set_id,omitempty"`
	SequenceNumber int            `json:"sequence_number,omitempty"`
	TotalTapes     int            `json:"total_tapes,omitempty"`
	BackupSets     []TOCBackupSet `json:"backup_sets"`
}

// TOCBackupSet represents a single backup set entry in the TOC
type TOCBackupSet struct {
	FileNumber      int            `json:"file_number"`
	JobName         string         `json:"job_name,omitempty"`
	BackupType      string         `json:"backup_type"`
	StartTime       time.Time      `json:"start_time"`
	EndTime         time.Time      `json:"end_time"`
	FileCount       int64          `json:"file_count"`
	TotalBytes      int64          `json:"total_bytes"`
	Encrypted       bool           `json:"encrypted"`
	HwEncrypted     bool           `json:"hw_encrypted,omitempty"`
	Compressed      bool           `json:"compressed"`
	CompressionType string         `json:"compression_type,omitempty"`
	Files           []TOCFileEntry `json:"files"`
}

// TOCFileEntry represents a single file entry in the TOC
type TOCFileEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Mode     int    `json:"mode,omitempty"`
	ModTime  string `json:"mod_time,omitempty"`
	Checksum string `json:"checksum,omitempty"`
}

// NewTapeTOC creates a new TapeTOC with the given tape metadata
func NewTapeTOC(tapeLabel, tapeUUID, pool string) *TapeTOC {
	return &TapeTOC{
		Magic:      tocMagic,
		Version:    tocVersion,
		TapeLabel:  tapeLabel,
		TapeUUID:   tapeUUID,
		Pool:       pool,
		CreatedAt:  time.Now(),
		BackupSets: []TOCBackupSet{},
	}
}

// MarshalTOC serializes a TapeTOC to JSON bytes
func MarshalTOC(toc *TapeTOC) ([]byte, error) {
	return json.Marshal(toc)
}

// UnmarshalTOC deserializes JSON bytes into a TapeTOC
func UnmarshalTOC(data []byte) (*TapeTOC, error) {
	var toc TapeTOC
	if err := json.Unmarshal(data, &toc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal TOC: %w", err)
	}
	if toc.Magic != tocMagic {
		return nil, fmt.Errorf("invalid TOC magic: expected %q, got %q", tocMagic, toc.Magic)
	}
	return &toc, nil
}

// WriteTOC writes the Table of Contents to the tape at the current position.
// The TOC is written as raw JSON padded to 64KB block boundaries, followed by a file mark.
// This should be called after writing all backup data and its trailing file mark.
func (s *Service) WriteTOC(ctx context.Context, toc *TapeTOC) error {
	tocData, err := json.MarshalIndent(toc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal TOC: %w", err)
	}

	// Pad to 64KB block boundary
	padSize := tocBlockSize - (len(tocData) % tocBlockSize)
	if padSize < tocBlockSize {
		tocData = append(tocData, make([]byte, padSize)...)
	}

	// Write TOC data to tape using dd
	cmd := exec.CommandContext(ctx, "dd",
		fmt.Sprintf("of=%s", s.devicePath),
		fmt.Sprintf("bs=%d", tocBlockSize),
		fmt.Sprintf("count=%d", len(tocData)/tocBlockSize),
	)
	cmd.Stdin = bytes.NewReader(tocData)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to write TOC to tape: %s", string(output))
	}

	// Write a file mark after the TOC data
	if err := s.WriteFileMark(ctx); err != nil {
		return fmt.Errorf("failed to write file mark after TOC: %w", err)
	}

	return nil
}

// GetDriveStatistics collects drive statistics from tapeinfo and sg_logs
func (s *Service) GetDriveStatistics(ctx context.Context) (*DriveStatisticsData, error) {
	stats := &DriveStatisticsData{}

	// Try tapeinfo first
	cmd := exec.CommandContext(ctx, "tapeinfo", "-f", s.devicePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		s.parseTapeInfoStats(string(output), stats)
	}

	// Try sg_logs for temperature page
	cmd = exec.CommandContext(ctx, "sg_logs", "-p", "0x0d", s.devicePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		s.parseTemperaturePage(string(output), stats)
	}

	// Try sg_logs for device statistics page
	cmd = exec.CommandContext(ctx, "sg_logs", "-p", "0x14", s.devicePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		s.parseDeviceStatisticsPage(string(output), stats)
	}

	// Try sg_logs for data compression page
	cmd = exec.CommandContext(ctx, "sg_logs", "-p", "0x1b", s.devicePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		s.parseDataCompressionPage(string(output), stats)
	}

	// Try sg_logs for tape alert page
	cmd = exec.CommandContext(ctx, "sg_logs", "-p", "0x2e", s.devicePath)
	output, err = cmd.CombinedOutput()
	if err == nil {
		s.parseTapeAlertPage(string(output), stats)
	}

	return stats, nil
}

// parseTapeInfoStats parses tapeinfo output for drive statistics
func (s *Service) parseTapeInfoStats(output string, stats *DriveStatisticsData) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch {
		case key == "Total Loads" || key == "LoadCount":
			stats.TotalLoadCount, _ = strconv.ParseInt(value, 10, 64)
		case key == "Total Written" || strings.Contains(key, "TotalWritten"):
			stats.TotalBytesWritten, _ = strconv.ParseInt(value, 10, 64)
		case key == "Total Read" || strings.Contains(key, "TotalRead"):
			stats.TotalBytesRead, _ = strconv.ParseInt(value, 10, 64)
		case key == "Write Errors" || strings.Contains(key, "WriteErrors"):
			stats.WriteErrors, _ = strconv.ParseInt(value, 10, 64)
		case key == "Read Errors" || strings.Contains(key, "ReadErrors"):
			stats.ReadErrors, _ = strconv.ParseInt(value, 10, 64)
		case key == "CleaningRequired" || strings.Contains(key, "Cleaning"):
			stats.CleaningRequired = strings.Contains(strings.ToLower(value), "yes") || value == "1"
		case key == "PowerOnHours" || strings.Contains(key, "Power On"):
			stats.PowerOnHours, _ = strconv.ParseInt(value, 10, 64)
		}
	}

}

// ReadTOC reads the Table of Contents from the tape at the current position.
// The caller must position the tape to the TOC file section before calling this method.
// Typically, the TOC is at file #2 (after the label at #0 and backup data at #1).
func (s *Service) ReadTOC(ctx context.Context) (*TapeTOC, error) {
	// Read TOC data from tape using dd with a reasonable max size (16MB)
	cmd := exec.CommandContext(ctx, "dd",
		fmt.Sprintf("if=%s", s.devicePath),
		fmt.Sprintf("bs=%d", tocBlockSize),
		"count=256", // Up to 16MB of TOC data
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read TOC from tape: %w", err)
	}

	// Trim null padding from block-aligned read
	trimmed := output
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] != 0 {
			trimmed = trimmed[:i+1]
			break
		}
	}

	return UnmarshalTOC(trimmed)
}

// parseTemperaturePage parses sg_logs temperature page (0x0d) output
func (s *Service) parseTemperaturePage(output string, stats *DriveStatisticsData) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "Current temperature") && !strings.Contains(line, "not available") {
			stats.TemperatureC = extractSgLogsValue(line)
		}
	}
}

// parseDeviceStatisticsPage parses sg_logs device statistics page (0x14) output
func (s *Service) parseDeviceStatisticsPage(output string, stats *DriveStatisticsData) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.Contains(line, "Lifetime media loads"):
			if v := extractSgLogsColonValue(line); v > 0 {
				stats.TotalLoadCount = v
			}
		case strings.Contains(line, "Lifetime power on hours"):
			if v := extractSgLogsColonValue(line); v > 0 {
				stats.PowerOnHours = v
			}
		case strings.Contains(line, "Lifetime power cycles"):
			if v := extractSgLogsColonValue(line); v > 0 {
				stats.LifetimePowerCycles = v
			}
		case strings.Contains(line, "Hard write errors"):
			if v := extractSgLogsColonValue(line); v > 0 {
				stats.WriteErrors = v
			}
		case strings.Contains(line, "Hard read errors"):
			if v := extractSgLogsColonValue(line); v > 0 {
				stats.ReadErrors = v
			}
		}
	}
}

// parseDataCompressionPage parses sg_logs data compression page (0x1b) output
func (s *Service) parseDataCompressionPage(output string, stats *DriveStatisticsData) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.Contains(line, "Read compression ratio"):
			if v := extractSgLogsColonValue(line); v > 0 {
				stats.ReadCompressionPct = v
			}
		case strings.Contains(line, "Write compression ratio"):
			if v := extractSgLogsColonValue(line); v > 0 {
				stats.WriteCompressionPct = v
			}
		}
	}
}

// parseTapeAlertPage parses sg_logs tape alert page (0x2e) output and collects active alert flags
func (s *Service) parseTapeAlertPage(output string, stats *DriveStatisticsData) {
	var activeAlerts []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Tape alert lines look like: "  Read warning: 0" or "  Media life: 1"
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			continue
		}
		label := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		// Skip reserved/obsolete entries and non-flag lines
		if strings.HasPrefix(label, "Reserved") || strings.HasPrefix(label, "Obsolete") {
			continue
		}
		if value == "1" {
			activeAlerts = append(activeAlerts, label)
		}
	}
	if len(activeAlerts) > 0 {
		stats.TapeAlertFlags = strings.Join(activeAlerts, ",")
	}
}

// extractSgLogsValue extracts an integer value from an sg_logs output line
func extractSgLogsValue(line string) int64 {
	// sg_logs output format: "  Description = value"
	eqIdx := strings.LastIndex(line, "=")
	if eqIdx < 0 {
		return 0
	}
	val := strings.TrimSpace(line[eqIdx+1:])
	// Remove any units suffix
	fields := strings.Fields(val)
	if len(fields) > 0 {
		v, _ := strconv.ParseInt(fields[0], 10, 64)
		return v
	}

	return 0
}

// extractSgLogsColonValue extracts an integer value from a colon-delimited sg_logs line
func extractSgLogsColonValue(line string) int64 {
	// sg_logs output format: "  Description: value"
	colonIdx := strings.LastIndex(line, ":")
	if colonIdx < 0 {
		return 0
	}
	val := strings.TrimSpace(line[colonIdx+1:])
	fields := strings.Fields(val)
	if len(fields) > 0 {
		v, _ := strconv.ParseInt(fields[0], 10, 64)
		return v
	}
	return 0
}

// HardwareEncryptionStatus represents the current hardware encryption state of a tape drive.
// LTO-4 and later drives support AES-256-GCM encryption at the drive firmware level.
type HardwareEncryptionStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Algorithm string `json:"algorithm,omitempty"` // e.g. "AES-256-GCM"
	Mode      string `json:"mode"`                // "on", "mixed", "off", "rawread"
	Error     string `json:"error,omitempty"`
}

// SetHardwareEncryption enables hardware AES-256-GCM encryption on the tape drive
// using the stenc utility. keyData is the raw 256-bit key (32 bytes).
// The key is passed via a temporary file that is securely removed after use.
func (s *Service) SetHardwareEncryption(ctx context.Context, keyData []byte) error {
	if len(keyData) != 32 {
		return fmt.Errorf("hardware encryption requires a 256-bit (32-byte) key, got %d bytes", len(keyData))
	}

	// Write key to a temporary file with restricted permissions (stenc reads from a key file)
	tmpDir := os.TempDir()
	keyFilePath := filepath.Join(tmpDir, fmt.Sprintf("tapebackarr-hwenc-%d.key", time.Now().UnixNano()))
	tmpFile, err := os.OpenFile(keyFilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temporary key file: %w", err)
	}
	defer os.Remove(keyFilePath)

	if _, err := tmpFile.Write(keyData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write key to temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary key file: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, DefaultOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(opCtx, "stenc", "-f", s.devicePath, "-e", "on", "-k", keyFilePath, "-a", "1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if opCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("set hardware encryption timed out after %v: %w", DefaultOperationTimeout, ErrOperationTimeout)
		}
		return fmt.Errorf("failed to set hardware encryption: %s", string(output))
	}

	return nil
}

// ClearHardwareEncryption disables hardware encryption on the tape drive.
func (s *Service) ClearHardwareEncryption(ctx context.Context) error {
	opCtx, cancel := context.WithTimeout(ctx, DefaultOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(opCtx, "stenc", "-f", s.devicePath, "-e", "off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if opCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("clear hardware encryption timed out after %v: %w", DefaultOperationTimeout, ErrOperationTimeout)
		}
		return fmt.Errorf("failed to clear hardware encryption: %s", string(output))
	}

	return nil
}

// GetHardwareEncryptionStatus returns the current hardware encryption status of the drive.
func (s *Service) GetHardwareEncryptionStatus(ctx context.Context) (*HardwareEncryptionStatus, error) {
	status := &HardwareEncryptionStatus{
		Mode: "off",
	}

	opCtx, cancel := context.WithTimeout(ctx, DefaultOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(opCtx, "stenc", "-f", s.devicePath, "--detail")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if opCtx.Err() == context.DeadlineExceeded {
			status.Error = fmt.Sprintf("hardware encryption status timed out after %v", DefaultOperationTimeout)
			return status, ErrOperationTimeout
		}
		// stenc not installed or drive doesn't support encryption
		outputStr := string(output)
		if strings.Contains(outputStr, "not found") || strings.Contains(outputStr, "No such file") {
			status.Error = "stenc utility not installed"
			return status, nil
		}
		status.Error = fmt.Sprintf("failed to get hardware encryption status: %s", outputStr)
		return status, nil
	}

	s.parseHardwareEncryptionStatus(string(output), status)
	return status, nil
}

// parseHardwareEncryptionStatus parses stenc --detail output into a HardwareEncryptionStatus.
func (s *Service) parseHardwareEncryptionStatus(output string, status *HardwareEncryptionStatus) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lower := strings.ToLower(line)

		if strings.Contains(lower, "encryption") && strings.Contains(lower, "capable") {
			status.Supported = !strings.Contains(lower, "not capable")
		}
		if strings.Contains(lower, "drive encryption") || strings.Contains(lower, "encryption mode") {
			// Extract the value portion after the colon or last equals
			var value string
			if idx := strings.LastIndex(lower, ":"); idx >= 0 {
				value = strings.TrimSpace(lower[idx+1:])
			} else if idx := strings.LastIndex(lower, "="); idx >= 0 {
				value = strings.TrimSpace(lower[idx+1:])
			} else {
				// No delimiter found; skip mode detection for this line
				continue
			}

			switch {
			case strings.Contains(value, "mixed"):
				status.Enabled = true
				status.Mode = "mixed"
			case strings.Contains(value, "raw"):
				status.Enabled = false
				status.Mode = "rawread"
			case strings.Contains(value, "off") || strings.Contains(value, "disabled"):
				status.Enabled = false
				status.Mode = "off"
			case strings.Contains(value, "on") || strings.Contains(value, "encrypt"):
				status.Enabled = true
				status.Mode = "on"
			}
		}
		if strings.Contains(lower, "algorithm") && strings.Contains(lower, "aes") {
			status.Algorithm = "AES-256-GCM"
		}
	}
}
