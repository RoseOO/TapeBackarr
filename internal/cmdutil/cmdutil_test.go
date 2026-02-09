package cmdutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestErrorDetail(t *testing.T) {
	t.Run("nil error returns empty string", func(t *testing.T) {
		result := ErrorDetail(nil, nil)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("non-exec error returns error message", func(t *testing.T) {
		err := fmt.Errorf("some generic error")
		result := ErrorDetail(err, nil)
		if result != "some generic error" {
			t.Errorf("expected 'some generic error', got %q", result)
		}
	})

	t.Run("exit error includes exit code", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "exit 2")
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected error from command")
		}
		result := ErrorDetail(err, nil)
		if !strings.Contains(result, "exit code 2") {
			t.Errorf("expected 'exit code 2' in result, got %q", result)
		}
	})

	t.Run("exit error with stderr buffer", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "echo 'something went wrong' >&2; exit 1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected error from command")
		}
		result := ErrorDetail(err, &stderr)
		if !strings.Contains(result, "exit code 1") {
			t.Errorf("expected 'exit code 1' in result, got %q", result)
		}
		if !strings.Contains(result, "something went wrong") {
			t.Errorf("expected stderr content in result, got %q", result)
		}
	})

	t.Run("exit error with empty stderr buffer", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "exit 3")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected error from command")
		}
		result := ErrorDetail(err, &stderr)
		if result != "exit code 3" {
			t.Errorf("expected 'exit code 3', got %q", result)
		}
	})
}
