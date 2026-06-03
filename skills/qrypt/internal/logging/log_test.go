package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"off", LevelOff},
		{"none", LevelOff},
		{"unknown", LevelInfo},
	}
	for _, tt := range tests {
		result := ParseLevel(tt.input)
		if result != tt.expected {
			t.Errorf("ParseLevel(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelOff, "OFF"},
	}
	for _, tt := range tests {
		if tt.level.String() != tt.expected {
			t.Errorf("Level(%d).String() = %s, want %s", tt.level, tt.level.String(), tt.expected)
		}
	}
}

func TestLoggerStdout(t *testing.T) {
	l, err := New("debug", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if l.writer != os.Stdout {
		t.Errorf("expected stdout writer")
	}
}

func TestLoggerFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	l, err := New("debug", logPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l.lj == nil {
		t.Fatal("expected lumberjack logger")
	}
	l.Debugf("hello %s", "world")
	l.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("log missing message: %s", string(data))
	}
}

func TestLoggerLevelFilter(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{level: LevelWarn, writer: &buf}

	l.Debugf("debug msg")
	l.Infof("info msg")
	l.Warnf("warn msg")
	l.Errorf("error msg")

	output := buf.String()
	if strings.Contains(output, "debug msg") {
		t.Error("debug msg should be filtered")
	}
	if strings.Contains(output, "info msg") {
		t.Error("info msg should be filtered")
	}
	if !strings.Contains(output, "warn msg") {
		t.Error("warn msg should appear")
	}
	if !strings.Contains(output, "error msg") {
		t.Error("error msg should appear")
	}
}

func TestRotateConfig(t *testing.T) {
	rc := &RotateConfig{MaxSize: 50, MaxBackups: 3, MaxAge: 7}
	l, err := New("info", t.TempDir()+"/rotate.log", rc)
	if err != nil {
		t.Fatal(err)
	}
	if l.lj.MaxSize != 50 {
		t.Errorf("expected 50, got %d", l.lj.MaxSize)
	}
	if l.lj.MaxBackups != 3 {
		t.Errorf("expected 3, got %d", l.lj.MaxBackups)
	}
	l.Close()
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ctoken=abc123", "ctoken=***"},
		{"__puus=secret_token", "__puus=***"},
		{"password=\"mysecret\"", "password=\"***\""},
		{"normal text", "normal text"},
		{"salt=\"\"", "salt=\"\""},
	}
	for _, tt := range tests {
		result := sanitize(tt.input)
		if result != tt.expected {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestSanitizeViaLogger(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{level: LevelInfo, writer: &buf}

	l.Infof("Cookie: ctoken=secret123; __puus=token456")

	output := buf.String()
	if strings.Contains(output, "secret123") {
		t.Error("ctoken value should be sanitized")
	}
	if strings.Contains(output, "token456") {
		t.Error("__puus value should be sanitized")
	}
	if !strings.Contains(output, "Cookie: ***") {
		t.Error("Cookie should be fully masked")
	}
}
