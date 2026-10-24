package logging_test

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/logging"
)

func TestNonTTYOutputsJSON(t *testing.T) {
	var buf bytes.Buffer
	if logging.IsTTY(&buf) {
		t.Fatalf("expected bytes.Buffer to not be a TTY")
	}
	logger := logging.NewLogger(&buf, slog.LevelInfo)
	logger.Info("server starting", "addr", ":8080")

	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "{") || !strings.Contains(out, `"msg":"server starting"`) {
		t.Fatalf("expected JSON log output for non-TTY writer, got: %s", out)
	}
}

func TestTTYDetectionWithCharDevice(t *testing.T) {
	// /dev/null on Linux is a character device (ModeCharDevice)
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()

	if !logging.IsTTY(devNull) {
		t.Fatalf("expected character device %s to be detected as TTY", os.DevNull)
	}

	// Regular file is NOT a character device
	tmpFile, err := os.CreateTemp(t.TempDir(), "log-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	if logging.IsTTY(tmpFile) {
		t.Fatalf("expected regular file to NOT be detected as TTY")
	}
}
