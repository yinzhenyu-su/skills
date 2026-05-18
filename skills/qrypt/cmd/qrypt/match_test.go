package main

import (
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

func TestNewMatcherRegexCaseSensitive(t *testing.T) {
	m := newMatcher("test", &findOptions{matchMode: matchRegex, caseSensitive: true})
	if m.re == nil {
		t.Fatal("regex not compiled")
	}
	if !m.matchName("test") {
		t.Error("should match 'test'")
	}
	if m.matchName("Test") {
		t.Error("should NOT match 'Test' (case sensitive)")
	}
}

func TestNewMatcherRegexCaseInsensitive(t *testing.T) {
	m := newMatcher("test", &findOptions{matchMode: matchRegex, caseSensitive: false})
	if !m.matchName("TEST") {
		t.Error("should match 'TEST' (case insensitive)")
	}
}

func TestNewMatcherGlob(t *testing.T) {
	m := newMatcher("*.go", &findOptions{matchMode: matchGlob})
	if !m.matchName("main.go") {
		t.Error("should match 'main.go'")
	}
	if m.matchName("main.rs") {
		t.Error("should NOT match 'main.rs'")
	}
}

func TestNewMatcherExact(t *testing.T) {
	m := newMatcher("target.txt", &findOptions{matchMode: matchExact})
	if !m.matchName("target.txt") {
		t.Error("should match 'target.txt'")
	}
	if m.matchName("target.txt.bak") {
		t.Error("should NOT match 'target.txt.bak'")
	}
}

func TestNewMatcherSubstring(t *testing.T) {
	m := newMatcher("photo", &findOptions{matchMode: matchSubstring})
	if !m.matchName("my_photo_2024.jpg") {
		t.Error("should match 'my_photo_2024.jpg'")
	}
	if !m.matchName("photography") {
		t.Error("should match 'photography' (substring match)")
	}
	// Default caseSensitive=false enables case-insensitive matching
	if !m.matchName("Photo") {
		t.Error("should match 'Photo' (case insensitive by default)")
	}
}

func TestNewMatcherSubstringCaseSensitive(t *testing.T) {
	m := newMatcher("Photo", &findOptions{matchMode: matchSubstring, caseSensitive: true})
	if !m.matchName("Photo2024") {
		t.Error("should match 'Photo2024' (case sensitive)")
	}
	if m.matchName("photo2024") {
		t.Error("should NOT match 'photo2024' (case sensitive)")
	}
}

func TestNewMatcherEmptyPattern(t *testing.T) {
	m := newMatcher("", &findOptions{})
	if !m.matchName("anything") {
		t.Error("empty pattern should match everything")
	}
}

func TestMatchEntryFileType(t *testing.T) {
	m := &nodeMatcher{fileType: 1} // files only
	if m.matchEntry(drive.Entry{IsDir: true, Size: 100}) {
		t.Error("should NOT match directory when fileType=1")
	}
	if !m.matchEntry(drive.Entry{IsDir: false, Size: 100}) {
		t.Error("should match file when fileType=1")
	}

	m2 := &nodeMatcher{fileType: 2} // dirs only
	if !m2.matchEntry(drive.Entry{IsDir: true, Size: 0}) {
		t.Error("should match directory when fileType=2")
	}
	if m2.matchEntry(drive.Entry{IsDir: false, Size: 100}) {
		t.Error("should NOT match file when fileType=2")
	}
}

func TestMatchEntrySizeGt(t *testing.T) {
	m := &nodeMatcher{sizeOp: 1, sizeBytes: 100}
	if m.matchEntry(drive.Entry{Size: 50}) {
		t.Error("50 should NOT be > 100")
	}
	if !m.matchEntry(drive.Entry{Size: 150}) {
		t.Error("150 should be > 100")
	}
}

func TestMatchEntrySizeLt(t *testing.T) {
	m := &nodeMatcher{sizeOp: -1, sizeBytes: 100}
	if m.matchEntry(drive.Entry{Size: 150}) {
		t.Error("150 should NOT be < 100")
	}
	if !m.matchEntry(drive.Entry{Size: 50}) {
		t.Error("50 should be < 100")
	}
}

func TestMatchEntrySizeEq(t *testing.T) {
	// sizeOp=0 means "no size filter" — always match
	m := &nodeMatcher{sizeOp: 0, sizeBytes: 100}
	if !m.matchEntry(drive.Entry{Size: 100}) {
		t.Error("should match when sizeOp=0 (no filter)")
	}
	if !m.matchEntry(drive.Entry{Size: 101}) {
		t.Error("should match when sizeOp=0 regardless of size")
	}
}

func TestMatchEntryDirSizeOp(t *testing.T) {
	// Directories should not have size checks applied
	m := &nodeMatcher{sizeOp: 1, sizeBytes: 100}
	if !m.matchEntry(drive.Entry{IsDir: true, Size: 50}) {
		t.Error("directories should bypass size check")
	}
}

func TestMatchNameGlobCaseInsensitive(t *testing.T) {
	m := newMatcher("*.TXT", &findOptions{matchMode: matchGlob, caseSensitive: false})
	if !m.matchName("readme.txt") {
		t.Error("should match 'readme.txt' (case insensitive glob)")
	}
}

func TestMatchNameRegexEdgeCase(t *testing.T) {
	m := newMatcher("^[a-z]+\\.txt$", &findOptions{matchMode: matchRegex})
	if !m.matchName("file.txt") {
		t.Error("should match 'file.txt'")
	}
	if m.matchName("file.txt.bak") {
		t.Error("should NOT match 'file.txt.bak'")
	}
}
