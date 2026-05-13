package main

import (
	"errors"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type mockLister struct {
	dirs   map[string][]drive.Entry
	err    error
	errMap map[string]error
}

func (m *mockLister) List(parentID string) ([]drive.Entry, error) {
	if err, ok := m.errMap[parentID]; ok {
		return nil, err
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.dirs[parentID], nil
}

type mockCipher2 struct {
	decryptMap map[string]string
}

func (m *mockCipher2) DecryptSegment(name string) (string, error) {
	if d, ok := m.decryptMap[name]; ok {
		return d, nil
	}
	return name, errors.New("no decrypt mapping")
}

func (m *mockCipher2) EncryptSegment(name string) string {
	return name
}

func TestFindRecursiveEmptyDir(t *testing.T) {
	mock := &mockLister{
		dirs: map[string][]drive.Entry{
			"0": {},
		},
	}
	mockCiph := &mockCipher2{decryptMap: map[string]string{}}

	f := newFinder(mock, mockCiph, &findOptions{pattern: ""})
	matched := f.run("0", "/")
	if matched != 0 {
		t.Errorf("expected 0 matches, got %d", matched)
	}
}

func TestFindRecursiveMatchAll(t *testing.T) {
	mock := &mockLister{
		dirs: map[string][]drive.Entry{
			"0": {
				{ID: "f1", Name: "enc_photo.jpg", IsDir: true, Size: 0},
				{ID: "f2", Name: "enc_doc.txt", IsDir: false, Size: 100},
			},
		},
	}
	mockCiph := &mockCipher2{
		decryptMap: map[string]string{
			"enc_photo.jpg": "photo.jpg",
			"enc_doc.txt":   "doc.txt",
		},
	}

	f := newFinder(mock, mockCiph, &findOptions{pattern: ""})
	matched := f.run("0", "/")
	if matched != 2 {
		t.Errorf("expected 2 matches, got %d", matched)
	}
}

func TestFindRecursivePatternMatch(t *testing.T) {
	mock := &mockLister{
		dirs: map[string][]drive.Entry{
			"0": {
				{ID: "f1", Name: "enc_photo.jpg", IsDir: true, Size: 100},
				{ID: "f2", Name: "enc_doc.txt", IsDir: true, Size: 200},
				{ID: "f3", Name: "enc_other", IsDir: true, Size: 300},
			},
		},
	}
	mockCiph := &mockCipher2{
		decryptMap: map[string]string{
			"enc_photo.jpg": "photo.jpg",
			"enc_doc.txt":   "doc.txt",
			"enc_other":     "other",
		},
	}

	f := newFinder(mock, mockCiph, &findOptions{pattern: "jpg"})
	matched := f.run("0", "/")
	if matched != 1 {
		t.Errorf("expected 1 match, got %d", matched)
	}
}

func TestFindWorkerCount(t *testing.T) {
	f := newFinder(nil, nil, &findOptions{workers: 5})
	if cap(f.sem) != 5 {
		t.Errorf("expected 5 workers, got %d", cap(f.sem))
	}

	f2 := newFinder(nil, nil, &findOptions{workers: 10})
	if cap(f2.sem) != 8 {
		t.Errorf("expected 8 workers (clamped), got %d", cap(f2.sem))
	}

	f3 := newFinder(nil, nil, &findOptions{workers: 0})
	if cap(f3.sem) != 1 {
		t.Errorf("expected 1 worker (default), got %d", cap(f3.sem))
	}
}
