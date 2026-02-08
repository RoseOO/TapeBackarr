package tape

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

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
	Label     string `json:"label"`
	UUID      string `json:"uuid"`
	Pool      string `json:"pool"`
	Timestamp int64  `json:"timestamp"`
}

// Service provides tape drive operations
type Service struct {
	devicePath string
	blockSize  int
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
	}
}

// NewServiceForDevice creates a tape service for a specific device path
func NewServiceForDevice(devicePath string, blockSize int) *Service {
	return &Service{
		devicePath: devicePath,
		blockSize:  blockSize,
	}
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
						}
					}
				}
			}
		}
	}

	return drives, nil
}

// GetStatus returns the current status of the tape drive
func (s *Service) GetStatus(ctx context.Context) (*DriveStatus, error) {
	status := &DriveStatus{
		DevicePath:  s.devicePath,
		LastChecked: time.Now(),
	}

	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
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
	densityRe := regexp.MustCompile(`Tape block size (\d+) bytes\. Density code (0x[0-9a-f]+)`)
	if matches := densityRe.FindStringSubmatch(outputStr); len(matches) > 2 {
		status.BlockSize, _ = strconv.Atoi(matches[1])
		status.Density = matches[2]
	}

	return status, nil
}

// Rewind rewinds the tape to the beginning
func (s *Service) Rewind(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "rewind")
	output, err := cmd.CombinedOutput()
	if err != nil {
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
	return nil
}

// Load loads a tape (if autoloader is available)
func (s *Service) Load(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "load")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("load failed: %s", string(output))
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

// SetBlockSize sets the tape block size
func (s *Service) SetBlockSize(ctx context.Context, size int) error {
	cmd := exec.CommandContext(ctx, "mt", "-f", s.devicePath, "setblk", strconv.Itoa(size))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("set block size failed: %s", string(output))
	}
	return nil
}

// ReadTapeLabel reads the label from the beginning of the tape
func (s *Service) ReadTapeLabel(ctx context.Context) (*TapeLabelData, error) {
	// Rewind to beginning
	if err := s.Rewind(ctx); err != nil {
		return nil, err
	}

	// Read first block which should contain the label
	cmd := exec.CommandContext(ctx, "dd", fmt.Sprintf("if=%s", s.devicePath), "bs=512", "count=1")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read label: %w", err)
	}

	// Parse label (format: "TAPEBACKARR|label|uuid|pool|timestamp")
	parts := strings.Split(strings.TrimRight(string(output), "\x00"), "|")
	if len(parts) >= 2 && parts[0] == "TAPEBACKARR" {
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
		return data, nil
	}

	return nil, nil
}

// WriteTapeLabel writes a label to the beginning of the tape
func (s *Service) WriteTapeLabel(ctx context.Context, label string, uuid string, pool string) error {
	// Rewind to beginning
	if err := s.Rewind(ctx); err != nil {
		return err
	}

	// Create label block with UUID and pool info
	labelData := fmt.Sprintf("TAPEBACKARR|%s|%s|%s|%d", label, uuid, pool, time.Now().Unix())
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
	return s.WriteFileMark(ctx)
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
