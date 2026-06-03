package coreadapter

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
)

func TestToCoreEntry_Roundtrip(t *testing.T) {
	be := backend.Entry{
		ID:      "id1",
		Name:    "n",
		IsDir:   true,
		Size:    42,
		ModTime: time.Unix(123, 0),
	}
	ce := ToCoreEntry(be)
	back := ToBackendEntry(ce)
	if back != be {
		t.Errorf("roundtrip lost data: got %+v want %+v", back, be)
	}
}

func TestDriverAdapter_List_Conversion(t *testing.T) {
	mb := &fullMockBackend{}
	a := NewDriverAdapter(mb)
	entries, err := a.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "foo" {
		t.Errorf("got %+v", entries)
	}
}

func TestDriverAdapter_UnsupportedOps(t *testing.T) {
	mb := &fullMockBackend{}
	a := NewDriverAdapter(mb)

	if _, err := a.Mkdir(context.Background(), "p", "n"); err == nil {
		t.Error("Mkdir should fail")
	}
	if err := a.Remove(context.Background(), qrypt.Entry{}); err == nil {
		t.Error("Remove should fail")
	}
	if _, err := a.Put(context.Background(), "p", "n", 0, nil); err == nil {
		t.Error("Put should fail")
	}
	if _, err := a.ResolvePath(context.Background(), "/"); err == nil {
		t.Error("ResolvePath should fail")
	}
}

func TestDriverAdapter_Inner(t *testing.T) {
	mb := &fullMockBackend{}
	a := NewDriverAdapter(mb)
	if a.Inner() != backend.Driver(mb) {
		t.Error("Inner mismatch")
	}
}

func TestNewSingleDriverFactory(t *testing.T) {
	mb := &fullMockBackend{}
	f := NewSingleDriverFactory(mb)
	d, err := f.CreateDriver(context.Background(), qrypt.SessionConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("nil driver")
	}
}

type fullMockBackend struct{}

func (f *fullMockBackend) Init(context.Context) error { return nil }
func (f *fullMockBackend) Drop(context.Context) error { return nil }
func (f *fullMockBackend) List(context.Context, string) ([]backend.Entry, error) {
	return []backend.Entry{{ID: "x", Name: "foo"}}, nil
}
func (f *fullMockBackend) Read(ctx context.Context, e backend.Entry, off, sz int64) (io.ReadCloser, error) {
	return nil, nil
}
