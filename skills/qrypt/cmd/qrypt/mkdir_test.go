package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type mockMkdirDriver struct {
	drive.Driver
	entries map[string][]drive.Entry // fid -> entries
}

func (m *mockMkdirDriver) List(ctx context.Context, parentID string) ([]drive.Entry, error) {
	return m.entries[parentID], nil
}

func (m *mockMkdirDriver) Mkdir(ctx context.Context, parentID, name string) (drive.Entry, error) {
	id := fmt.Sprintf("fid_%s_%s", parentID, name)
	entry := drive.Entry{ID: id, Name: name, IsDir: true}
	m.entries[parentID] = append(m.entries[parentID], entry)
	return entry, nil
}

func (m *mockMkdirDriver) Move(ctx context.Context, entry drive.Entry, dstParentID string) error { return nil }
func (m *mockMkdirDriver) Rename(ctx context.Context, entry drive.Entry, newName string) error {
	return nil
}
func (m *mockMkdirDriver) Remove(ctx context.Context, entry drive.Entry) error { return nil }

func (m *mockMkdirDriver) ResolvePath(ctx context.Context, path string) (string, error) {
	if path == "/" || path == "" {
		return "0", nil
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	currentFid := "0"
	for _, seg := range segments {
		entries := m.entries[currentFid]
		found := false
		for _, e := range entries {
			if e.Name == seg {
				currentFid = e.ID
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("not found")
		}
	}
	return currentFid, nil
}

func TestCreateDirectory(t *testing.T) {
	cipher, err := crypt.NewRcloneCipher("test", "test")
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}
	enc := func(s string) string { return cipher.EncryptSegment(s) }
	ctx := context.Background()

	t.Run("Simple Creation", func(t *testing.T) {
		m := &mockMkdirDriver{
			entries: make(map[string][]drive.Entry),
		}
		err := createDirectory(ctx, m, m, cipher, "/newdir", "/newdir", false)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		want := enc("newdir")
		if len(m.entries["0"]) != 1 || m.entries["0"][0].Name != want {
			t.Errorf("directory not created correctly: %v", m.entries["0"])
		}
	})

	t.Run("Recursive Creation parents=true", func(t *testing.T) {
		m := &mockMkdirDriver{
			entries: make(map[string][]drive.Entry),
		}
		err := createDirectory(ctx, m, m, cipher, "/a/b/c", "/a/b/c", true)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		wantA := enc("a")
		if len(m.entries["0"]) != 1 || m.entries["0"][0].Name != wantA {
			t.Fatalf("dir 'a' not created, got %s", m.entries["0"][0].Name)
		}
		fidA := m.entries["0"][0].ID
		wantB := enc("b")
		if len(m.entries[fidA]) != 1 || m.entries[fidA][0].Name != wantB {
			t.Fatalf("dir 'b' not created")
		}
		fidB := m.entries[fidA][0].ID
		wantC := enc("c")
		if len(m.entries[fidB]) != 1 || m.entries[fidB][0].Name != wantC {
			t.Fatalf("dir 'c' not created")
		}
	})

	t.Run("Simple Creation Fails without parents", func(t *testing.T) {
		m := &mockMkdirDriver{
			entries: make(map[string][]drive.Entry),
		}
		err := createDirectory(ctx, m, m, cipher, "/a/b", "/a/b", false)
		if err == nil {
			t.Errorf("expected error for missing parent, got nil")
		}
	})

	t.Run("Existing directory with parents=true", func(t *testing.T) {
		wantA := enc("a")
		m := &mockMkdirDriver{
			entries: map[string][]drive.Entry{
				"0": {{ID: "fid_a", Name: wantA, IsDir: true}},
			},
		}
		err := createDirectory(ctx, m, m, cipher, "/a", "/a", true)
		if err != nil {
			t.Errorf("expected no error for existing dir with parents=true, got %v", err)
		}
		if len(m.entries["0"]) != 1 {
			t.Errorf("should not have created a duplicate directory")
		}
	})

	t.Run("Conflict with file", func(t *testing.T) {
		wantA := enc("a")
		m := &mockMkdirDriver{
			entries: map[string][]drive.Entry{
				"0": {{ID: "fid_file", Name: wantA, IsDir: false}},
			},
		}
		err := createDirectory(ctx, m, m, cipher, "/a/b", "/a/b", true)
		if err == nil || !strings.Contains(err.Error(), "路径冲突") {
			t.Errorf("expected conflict error, got %v", err)
		}
	})
}
