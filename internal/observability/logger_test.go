package observability

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestInitLoggerValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, l := range levels {
		t.Run(l, func(t *testing.T) {
			var buf bytes.Buffer
			logger, level := InitLogger(&buf, l)
			if logger == nil {
				t.Fatal("logger should not be nil")
			}
			want, _ := parseLevel(l)
			if level != want {
				t.Errorf("level = %v, want %v", level, want)
			}
		})
	}
}

func TestInitLoggerInvalidLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, level := InitLogger(&buf, "unknown")
	if logger == nil {
		t.Fatal("logger should not be nil")
	}
	if level != slog.LevelInfo {
		t.Errorf("level = %v, want Info", level)
	}
}

func TestInitLoggerOutput(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := InitLogger(&buf, "info")
	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Errorf("output missing message: %s", output)
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Errorf("output missing key/value: %s", output)
	}
}

func TestInitLoggerFiltersByLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := InitLogger(&buf, "warn")
	logger.Info("should not appear")
	logger.Warn("should appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Error("info should be filtered at warn level")
	}
	if !strings.Contains(output, "should appear") {
		t.Error("warn should pass at warn level")
	}
}
