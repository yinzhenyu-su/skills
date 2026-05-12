package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

type mockFileLister struct {
	dirs   map[string][]quark.File
	err    error
	errMap map[string]error
}

func (m *mockFileLister) ListFiles(parentFid string) ([]quark.File, error) {
	if err, ok := m.errMap[parentFid]; ok {
		return nil, err
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.dirs[parentFid], nil
}

type mockCipher struct {
	decryptMap map[string]string
}

func (m *mockCipher) DecryptSegment(name string) (string, error) {
	if d, ok := m.decryptMap[name]; ok {
		return d, nil
	}
	return name, errors.New("no decrypt mapping")
}

func (m *mockCipher) EncryptSegment(name string) string {
	return name
}

func makeFile(fid, name string, isFile bool, size int64) quark.File {
	b, _ := json.Marshal(size)
	return quark.File{
		Fid:      fid,
		FileName: name,
		File:     isFile,
		Size:     json.Number(string(b)),
	}
}

func TestFindRecursiveEmptyDir(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {},
		},
	}
	mockCiph := &mockCipher{decryptMap: map[string]string{}}

	matched := 0
	err := findRecursive(mock, mockCiph, "0", "/", "", &matched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != 0 {
		t.Errorf("expected 0 matches, got %d", matched)
	}
}

func TestFindRecursiveMatchAll(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("f1", "enc_photo.jpg", true, 0),
				makeFile("f2", "enc_doc.txt", false, 100),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_photo.jpg": "photo.jpg",
			"enc_doc.txt":   "doc.txt",
		},
	}

	matched := 0
	err := findRecursive(mock, mockCiph, "0", "/", "", &matched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != 2 {
		t.Errorf("expected 2 matches, got %d", matched)
	}
}

func TestFindRecursivePatternMatch(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("f1", "enc_photo.jpg", true, 100),
				makeFile("f2", "enc_doc.txt", true, 200),
				makeFile("f3", "enc_other", true, 300),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_photo.jpg": "photo.jpg",
			"enc_doc.txt":   "doc.txt",
			"enc_other":     "other",
		},
	}

	matched := 0
	err := findRecursive(mock, mockCiph, "0", "/", "jpg", &matched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != 1 {
		t.Errorf("expected 1 match, got %d", matched)
	}
}

func TestFindRecursiveDecryptFailure(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("f1", "enc_photo.jpg", true, 100),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{},
	}

	matched := 0
	err := findRecursive(mock, mockCiph, "0", "/", "enc_photo", &matched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != 1 {
		t.Errorf("expected 1 match (fallback to encrypted name), got %d", matched)
	}
}

func TestFindRecursiveNestedDirs(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("d1", "enc_subdir", false, 0),
			},
			"d1": {
				makeFile("f1", "enc_nested.txt", true, 100),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_subdir":      "subdir",
			"enc_nested.txt": "nested.txt",
		},
	}

	matched := 0
	err := findRecursive(mock, mockCiph, "0", "/", "nested", &matched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != 1 {
		t.Errorf("expected 1 match (nested.txt), got %d", matched)
	}
}

func TestFindRecursiveNestedListErrorSkips(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("d1", "enc_broken", false, 0),
			},
		},
		errMap: map[string]error{
			"d1": errors.New("list error"),
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_broken": "broken",
		},
	}

	matched := 0
	err := findRecursive(mock, mockCiph, "0", "/", "broken", &matched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != 1 {
		t.Errorf("expected 1 match (the parent dir), got %d", matched)
	}
}

func TestFindRecursiveCaseInsensitive(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("f1", "enc_Photo.JPG", true, 100),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_Photo.JPG": "Photo.JPG",
		},
	}

	matched := 0
	err := findRecursive(mock, mockCiph, "0", "/", "photo", &matched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != 1 {
		t.Errorf("expected 1 match (case insensitive), got %d", matched)
	}
}

func TestNodeMatcherSubstring(t *testing.T) {
	m := newMatcher("JPG", &findOptions{matchMode: matchSubstring, caseSensitive: false})
	if !m.matchName("photo.jpg") {
		t.Error("expected substring match")
	}
}

func TestNodeMatcherGlob(t *testing.T) {
	m := newMatcher("*.jpg", &findOptions{matchMode: matchGlob, caseSensitive: false})
	if !m.matchName("photo.jpg") {
		t.Error("expected glob match *.jpg")
	}
	if m.matchName("photo.txt") {
		t.Error("expected no match for .txt")
	}
	m2 := newMatcher("photo_*", &findOptions{matchMode: matchGlob, caseSensitive: true})
	if !m2.matchName("photo_001.jpg") {
		t.Error("expected glob match photo_*")
	}
}

func TestNodeMatcherRegex(t *testing.T) {
	m := newMatcher(`\.jpg$`, &findOptions{matchMode: matchRegex, caseSensitive: false})
	if !m.matchName("photo.jpg") {
		t.Error("expected regex match")
	}
	if m.matchName("photo.txt") {
		t.Error("expected no match for .txt")
	}
	m2 := newMatcher(`photo\.\w+`, &findOptions{matchMode: matchRegex, caseSensitive: true})
	if m2.matchName("Photo.jpg") {
		t.Error("expected no match for case-sensitive regex")
	}
}

func TestNodeMatcherExact(t *testing.T) {
	m := newMatcher("photo.jpg", &findOptions{matchMode: matchExact, caseSensitive: false})
	if !m.matchName("photo.jpg") {
		t.Error("expected exact match")
	}
	if m.matchName("photo2.jpg") {
		t.Error("expected no match for different name")
	}
}

func TestNodeMatcherCaseSensitive(t *testing.T) {
	m := newMatcher("Photo", &findOptions{matchMode: matchSubstring, caseSensitive: true})
	if m.matchName("photo.jpg") {
		t.Error("expected no match for case-sensitive")
	}
	if !m.matchName("Photo.jpg") {
		t.Error("expected match for exact case")
	}
}

func TestNodeMatcherEmptyPattern(t *testing.T) {
	m := newMatcher("", &findOptions{matchMode: matchSubstring})
	if !m.matchName("anything") {
		t.Error("expected empty pattern to match everything")
	}
}

func TestNodeMatcherTypeFilter(t *testing.T) {
	dir := makeFile("d1", "dir", false, 0)
	file := makeFile("f1", "file.txt", true, 100)

	m := &nodeMatcher{pattern: "", fileType: 1}
	if !m.matchFile(file) {
		t.Error("expected file to match file-only filter")
	}
	if m.matchFile(dir) {
		t.Error("expected dir to NOT match file-only filter")
	}

	m2 := &nodeMatcher{pattern: "", fileType: 2}
	if !m2.matchFile(dir) {
		t.Error("expected dir to match dir-only filter")
	}
	if m2.matchFile(file) {
		t.Error("expected file to NOT match dir-only filter")
	}
}

func TestNodeMatcherNoTypeFilter(t *testing.T) {
	m := &nodeMatcher{pattern: "", fileType: 0}
	dir := makeFile("d1", "dir", false, 0)
	file := makeFile("f1", "file.txt", true, 100)
	if !m.matchFile(dir) {
		t.Error("expected no type filter to match dirs")
	}
	if !m.matchFile(file) {
		t.Error("expected no type filter to match files")
	}
}

func TestNodeMatcherSizeFilter(t *testing.T) {
	small := makeFile("f1", "small.txt", true, 100)
	exact := makeFile("f2", "exact.txt", true, 1000)
	large := makeFile("f3", "large.txt", true, 5000)

	m := &nodeMatcher{pattern: "", sizeOp: 1, sizeBytes: 1000}
	if !m.matchFile(large) {
		t.Error("expected 5000 > 1000 to match")
	}
	if m.matchFile(small) {
		t.Error("expected 100 > 1000 to NOT match")
	}
	if m.matchFile(exact) {
		t.Error("expected 1000 > 1000 to NOT match")
	}

	m2 := &nodeMatcher{pattern: "", sizeOp: -1, sizeBytes: 1000}
	if !m2.matchFile(small) {
		t.Error("expected 100 < 1000 to match")
	}
	if m2.matchFile(exact) {
		t.Error("expected 1000 < 1000 to NOT match")
	}
}

func TestFinderMaxDepth(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("d1", "enc_sub", false, 0),
			},
			"d1": {
				makeFile("f1", "enc_deep.txt", true, 100),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_sub":      "sub",
			"enc_deep.txt": "deep.txt",
		},
	}
	f := &finder{
		fileSvc: mock,
		cipher:  mockCiph,
		opts:    &findOptions{maxDepth: 0},
		matcher: &nodeMatcher{pattern: "", caseSensitive: true},
		sem:     make(chan struct{}, 1),
	}
	matched := f.run("0", "/")
	if matched != 1 {
		t.Errorf("expected 1 match (dir at depth 0), got %d", matched)
	}
}

func TestFinderMaxMatches(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("f1", "enc_a.txt", true, 100),
				makeFile("f2", "enc_b.txt", true, 200),
				makeFile("f3", "enc_c.txt", true, 300),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_a.txt": "a.txt",
			"enc_b.txt": "b.txt",
			"enc_c.txt": "c.txt",
		},
	}
	f := &finder{
		fileSvc: mock,
		cipher:  mockCiph,
		opts:    &findOptions{maxMatches: 2, maxDepth: -1},
		matcher: &nodeMatcher{pattern: "", caseSensitive: true},
		sem:     make(chan struct{}, 1),
	}
	matched := f.run("0", "/")
	if matched > 2 {
		t.Errorf("expected at most 2 matches, got %d", matched)
	}
}

func TestFinderCountOnly(t *testing.T) {
	mock := &mockFileLister{
		dirs: map[string][]quark.File{
			"0": {
				makeFile("f1", "enc_a.txt", true, 100),
				makeFile("f2", "enc_b.txt", true, 200),
			},
		},
	}
	mockCiph := &mockCipher{
		decryptMap: map[string]string{
			"enc_a.txt": "a.txt",
			"enc_b.txt": "b.txt",
		},
	}
	f := &finder{
		fileSvc: mock,
		cipher:  mockCiph,
		opts:    &findOptions{countOnly: true, maxDepth: -1},
		matcher: &nodeMatcher{pattern: "", caseSensitive: true},
		sem:     make(chan struct{}, 1),
	}
	matched := f.run("0", "/")
	if matched != 2 {
		t.Errorf("expected 2 matches, got %d", matched)
	}
}

func TestNodeMatcherGlobCaseInsensitive(t *testing.T) {
	m := newMatcher("*.JPG", &findOptions{matchMode: matchGlob, caseSensitive: false})
	if !m.matchName("photo.jpg") {
		t.Error("expected case-insensitive glob match *.JPG against photo.jpg")
	}
	m2 := newMatcher("*.JPG", &findOptions{matchMode: matchGlob, caseSensitive: true})
	if m2.matchName("photo.jpg") {
		t.Error("expected case-sensitive glob to NOT match")
	}
}
