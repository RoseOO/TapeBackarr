package tape

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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
	BOT          bool      `json:"bot"`          // Beginning of Tape
	EOT          bool      `json:"eot"`          // End of Tape
	EOF          bool      `json:"eof"`          // End of File mark
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
	WriteCount int    `json:"write_count"`
	Density    string `json:"density,omitempty"`
}

// Service provides tape drive operations
type Service struct {
	devicePath string
	blockSize  int
}

// NewService creates a new tape service
func NewService(devicePath string, blockSize int) *Service {
	return &Service{
		devicePath: devicePath,
		blockSize:  blockSize,
	}
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
func (s *Service) ReadTapeLabel(ctx context.Context) (string, error) {
	// Rewind to beginning
	if err := s.Rewind(ctx); err != nil {
		return "", err
	}

	// Read first block which should contain the label
	cmd := exec.CommandContext(ctx, "dd", fmt.Sprintf("if=%s", s.devicePath), "bs=512", "count=1")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read label: %w", err)
	}

	// Parse label (assuming our custom format: "TAPEBACKARR|label|timestamp")
	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) >= 2 && parts[0] == "TAPEBACKARR" {
		return parts[1], nil
	}

	return "", nil
}

// WriteTapeLabel writes a label to the beginning of the tape
func (s *Service) WriteTapeLabel(ctx context.Context, label string) error {
	// Rewind to beginning
	if err := s.Rewind(ctx); err != nil {
		return err
	}

	// Create label block
	labelData := fmt.Sprintf("TAPEBACKARR|%s|%d", label, time.Now().Unix())
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
