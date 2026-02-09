package cmdutil

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrorDetail returns a human-readable description of a command failure.
// It extracts the exit code and stderr output from the error when available.
// An explicit stderr buffer is preferred; when nil the function falls back
// to the Stderr field of exec.ExitError (populated by Output/CombinedOutput).
func ErrorDetail(err error, stderr *bytes.Buffer) string {
	if err == nil {
		return ""
	}

	var detail strings.Builder
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		fmt.Fprintf(&detail, "exit code %d", exitErr.ExitCode())
		stderrText := ""
		if stderr != nil && stderr.Len() > 0 {
			stderrText = strings.TrimSpace(stderr.String())
		} else if len(exitErr.Stderr) > 0 {
			stderrText = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderrText != "" {
			fmt.Fprintf(&detail, ": %s", stderrText)
		}
	} else {
		detail.WriteString(err.Error())
	}

	return detail.String()
}
