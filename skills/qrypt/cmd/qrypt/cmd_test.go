package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/drive"
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

func TestNewFinderWithDefaults(t *testing.T) {
	f := newFinder(nil, nil, &findOptions{workers: -1})
	if cap(f.sem) != 1 {
		t.Errorf("expected 1 worker for invalid value, got %d", cap(f.sem))
	}
}

func TestFindOutputNoop(t *testing.T) {
	_ = newFinder(nil, nil, &findOptions{countOnly: true})
	_ = newFinder(nil, nil, &findOptions{countOnly: true, maxMatches: 10})
}

func TestFindMatchEntryDefaults(t *testing.T) {
	m := &nodeMatcher{fileType: 0, sizeOp: 0, sizeBytes: 0}
	if !m.matchEntry(drive.Entry{IsDir: false, Size: 0}) {
		t.Error("default matcher should match any entry")
	}
}

func TestFindListError(t *testing.T) {
	mock := &mockLister{
		err: nil,
		errMap: map[string]error{
			"0": os.ErrNotExist,
		},
		dirs: map[string][]drive.Entry{
			"0": {
				{ID: "f1", Name: "file.txt"},
			},
		},
	}
	mockCiph := &mockCipher2{decryptMap: map[string]string{}}

	f := newFinder(mock, mockCiph, &findOptions{pattern: ""})
	matched := f.run("0", "/")
	if matched != 0 {
		t.Errorf("expected 0 matches on list error, got %d", matched)
	}
}

func TestFindListRecursive(t *testing.T) {
	mock := &mockLister{
		errMap: map[string]error{},
		dirs: map[string][]drive.Entry{
			"0": {
				{ID: "d1", Name: "enc_dir", IsDir: true},
			},
			"d1": {
				{ID: "f1", Name: "enc_nested.txt", IsDir: false, Size: 50},
			},
		},
	}
	mockCiph := &mockCipher2{
		decryptMap: map[string]string{
			"enc_dir":         "dir",
			"enc_nested.txt":  "nested.txt",
		},
	}

	f := newFinder(mock, mockCiph, &findOptions{pattern: "nested", maxDepth: -1})
	matched := f.run("0", "/")
	if matched != 1 {
		t.Errorf("expected 1 nested match, got %d", matched)
	}
}

func TestFindMaxMatches(t *testing.T) {
	mock := &mockLister{
		dirs: map[string][]drive.Entry{
			"0": {
				{ID: "f1", Name: "enc_a.txt", IsDir: false},
				{ID: "f2", Name: "enc_b.txt", IsDir: false},
				{ID: "f3", Name: "enc_c.txt", IsDir: false},
			},
		},
	}
	mockCiph := &mockCipher2{
		decryptMap: map[string]string{
			"enc_a.txt": "a.txt",
			"enc_b.txt": "b.txt",
			"enc_c.txt": "c.txt",
		},
	}

	f := newFinder(mock, mockCiph, &findOptions{pattern: ".txt", maxMatches: 2})
	matched := f.run("0", "/")
	if matched > 2 {
		t.Errorf("expected max 2 matches, got %d", matched)
	}
}

func TestFindCountOnly(t *testing.T) {
	mock := &mockLister{
		dirs: map[string][]drive.Entry{
			"0": {
				{ID: "f1", Name: "enc_a.go", IsDir: false},
				{ID: "f2", Name: "enc_b.go", IsDir: false},
				{ID: "f3", Name: "enc_c.go", IsDir: false},
			},
		},
	}
	mockCiph := &mockCipher2{
		decryptMap: map[string]string{
			"enc_a.go": "a.go",
			"enc_b.go": "b.go",
			"enc_c.go": "c.go",
		},
	}

	f := newFinder(mock, mockCiph, &findOptions{pattern: ".go", countOnly: true})
	matched := f.run("0", "/")
	if matched != 3 {
		t.Errorf("expected 3 matches with countOnly, got %d", matched)
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
