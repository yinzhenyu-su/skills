package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountLines(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		content string
		want    int
	}{
		{"", 0},
		{"a\n", 2},    // 1 newline → 2 lines
		{"a\nb\n", 3}, // 2 newlines → 3 lines
		{"a\nb\nc", 3},
		{"line1\nline2\nline3\n", 4}, // 3 newlines → 4 lines
	}
	for _, tt := range tests {
		path := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
			t.Fatal(err)
		}
		got := countLines(path)
		if got != tt.want {
			t.Errorf("countLines(%q) = %d, want %d", tt.content, got, tt.want)
		}
	}
}

func TestCountLinesFileNotFound(t *testing.T) {
	got := countLines("/nonexistent/path/file.txt")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestFormatBytesEdgeCases(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{-1, "-1 B"},
		{1 << 40, "1.0 TB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.n)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestParseMountPath(t *testing.T) {
	tests := []struct {
		input       string
		wantMount   string
		wantPath    string
	}{
		{"personal:/docs/file.txt", "personal", "/docs/file.txt"},
		{"my-mount:/path", "my-mount", "/path"},
		{"a:/", "a", "/"},
		{"/abs/path", "", "/abs/path"},
		{"./relative", "", "./relative"},
		{"~/home/path", "", "~/home/path"},
		{"just-a-path", "", "just-a-path"},
		{"C:/Windows", "", "C:/Windows"}, // uppercase C doesn't match mount name
	}
	for _, tt := range tests {
		mount, path := ParseMountPath(tt.input)
		if mount != tt.wantMount {
			t.Errorf("ParseMountPath(%q) mount = %q, want %q", tt.input, mount, tt.wantMount)
		}
		if path != tt.wantPath {
			t.Errorf("ParseMountPath(%q) path = %q, want %q", tt.input, path, tt.wantPath)
		}
	}
}
