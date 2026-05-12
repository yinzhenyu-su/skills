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
