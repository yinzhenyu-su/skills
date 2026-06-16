package drivercache

import (
	"context"
	"io"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/mockdrive"
)

// cipherSetterMock wraps MockDriver and implements CipherSetter for testing.
type cipherSetterMock struct {
	*mockdrive.MockDriver
	cipherSet bool
}

func (m *cipherSetterMock) SetCipher(c cipher.Cipher) {
	m.cipherSet = true
}

func TestCachedDriver_List_CacheHit(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// First call — cache miss, delegates to inner.
	_, err := cd.List(ctx, "0")
	if err != nil {
		t.Fatal(err)
	}
	if n := inner.ListCallCount(); n != 1 {
		t.Fatalf("expected 1 List call after first access, got %d", n)
	}

	// Second call — cache hit, no inner call.
	_, err = cd.List(ctx, "0")
	if err != nil {
		t.Fatal(err)
	}
	if n := inner.ListCallCount(); n != 1 {
		t.Fatalf("expected 1 List call (cached), got %d", n)
	}
}

func TestCachedDriver_List_CacheMiss_DifferentParent(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// Warm cache for "0".
	_, err := cd.List(ctx, "0")
	if err != nil {
		t.Fatal(err)
	}
	if n := inner.ListCallCount(); n != 1 {
		t.Fatalf("expected 1 List call, got %d", n)
	}

	// Different parentID should miss cache.
	_, err = cd.List(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}
	if n := inner.ListCallCount(); n != 2 {
		t.Fatalf("expected 2 List calls (different parent), got %d", n)
	}
}

func TestCachedDriver_Mkdir_InvalidatesCache(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// Warm cache for "0".
	cd.List(ctx, "0")
	before := inner.ListCallCount()

	// Mkdir under "0" should invalidate its cache.
	_, err := cd.Mkdir(ctx, "0", "newdir")
	if err != nil {
		t.Fatal(err)
	}

	// List "0" again — should miss cache and call inner.
	_, err = cd.List(ctx, "0")
	if err != nil {
		t.Fatal(err)
	}
	if n := inner.ListCallCount(); n != before+1 {
		t.Fatalf("expected %d List calls after Mkdir+List, got %d", before+1, n)
	}
}

func TestCachedDriver_Move_InvalidatesCache(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// Create two directories under root.
	dir1, err := cd.Mkdir(ctx, "0", "dir1")
	if err != nil {
		t.Fatal(err)
	}
	dir2, err := cd.Mkdir(ctx, "0", "dir2")
	if err != nil {
		t.Fatal(err)
	}

	// Warm cache for both dirs.
	cd.List(ctx, dir1.ID)
	cd.List(ctx, dir2.ID)
	before := inner.ListCallCount()

	// Move dir1 into dir2.
	// dir1.ParentID is "" because MockDriver's Mkdir doesn't set ParentID on returned Entry.
	// Set it so cache invalidation for the old parent is tested properly.
	dir1.ParentID = "0"
	err = cd.Move(ctx, dir1, dir2.ID)
	if err != nil {
		t.Fatal(err)
	}

	// List dir2 — should be invalidated (dstParentID).
	cd.List(ctx, dir2.ID)
	if n := inner.ListCallCount(); n != before+1 {
		t.Fatalf("expected %d List calls after Move (dst), got %d", before+1, n)
	}
	before = inner.ListCallCount()

	// List "0" — should also be invalidated (old parent).
	cd.List(ctx, "0")
	if n := inner.ListCallCount(); n != before+1 {
		t.Fatalf("expected %d List calls after Move (old parent), got %d", before+1, n)
	}
}

func TestCachedDriver_Rename_InvalidatesCache(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// Create a directory under root.
	dir, err := cd.Mkdir(ctx, "0", "mydir")
	if err != nil {
		t.Fatal(err)
	}

	// Warm cache for the parent.
	cd.List(ctx, "0")
	before := inner.ListCallCount()

	dir.ParentID = "0"
	err = cd.Rename(ctx, dir, "renamed")
	if err != nil {
		t.Fatal(err)
	}

	// List parent — should be invalidated.
	cd.List(ctx, "0")
	if n := inner.ListCallCount(); n != before+1 {
		t.Fatalf("expected %d List calls after Rename+List, got %d", before+1, n)
	}
}

func TestCachedDriver_Remove_InvalidatesCache(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// Create a directory under root.
	dir, err := cd.Mkdir(ctx, "0", "toremove")
	if err != nil {
		t.Fatal(err)
	}

	// Warm cache for the parent.
	cd.List(ctx, "0")
	before := inner.ListCallCount()

	dir.ParentID = "0"
	err = cd.Remove(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// List parent — should be invalidated.
	cd.List(ctx, "0")
	if n := inner.ListCallCount(); n != before+1 {
		t.Fatalf("expected %d List calls after Remove+List, got %d", before+1, n)
	}
}

func TestCachedDriver_BatchRemove_InvalidatesCache(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// Create directories under root.
	d1, _ := cd.Mkdir(ctx, "0", "batch1")
	d2, _ := cd.Mkdir(ctx, "0", "batch2")

	// Warm cache for parents.
	cd.List(ctx, "0")
	before := inner.ListCallCount()

	// BatchRemove with proper parent IDs.
	d1.ParentID = "0"
	d2.ParentID = "0"
	err := cd.BatchRemove(ctx, []drivers.Entry{d1, d2})
	if err != nil {
		t.Fatal(err)
	}

	// List "0" — should be invalidated.
	cd.List(ctx, "0")
	if n := inner.ListCallCount(); n != before+1 {
		t.Fatalf("expected %d List calls after BatchRemove+List, got %d", before+1, n)
	}
}

func TestCachedDriver_Drop_ClearsCache(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	// Warm cache.
	cd.List(ctx, "0")
	before := inner.ListCallCount()

	// Drop clears cache.
	if err := cd.Drop(ctx); err != nil {
		t.Fatal(err)
	}

	// List should miss cache (cache was reset).
	cd.List(ctx, "0")
	if n := inner.ListCallCount(); n != before+1 {
		t.Fatalf("expected %d List calls after Drop+List, got %d", before+1, n)
	}
}

func TestCachedDriver_Read_Delegates(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	_, err := cd.Read(ctx, drivers.Entry{ID: "0"}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCachedDriver_Init_Delegates(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	if err := cd.Init(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCachedDriver_CipherSetter_Passthrough(t *testing.T) {
	inner := &cipherSetterMock{MockDriver: mockdrive.NewDriver()}
	cd := NewCachedDriver(inner)

	var iface any = cd
	setter, ok := iface.(drivers.CipherSetter)
	if !ok {
		t.Fatal("CachedDriver should satisfy drivers.CipherSetter")
	}

	setter.SetCipher(nil)
	if !inner.cipherSet {
		t.Fatal("expected inner SetCipher to be called")
	}
}

func TestCachedDriver_CipherSetter_Noop_WhenInnerMissing(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)

	var iface any = cd
	setter, ok := iface.(drivers.CipherSetter)
	if !ok {
		t.Fatal("CachedDriver should always satisfy drivers.CipherSetter")
	}
	setter.SetCipher(nil) // must not panic
}

func TestCachedDriver_PathResolver_Passthrough(t *testing.T) {
	inner := mockdrive.NewDriver()
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	fid, err := cd.ResolvePath(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if fid != "0" {
		t.Fatalf("expected fid '0', got %q", fid)
	}
}

func TestCachedDriver_Writer_NotImplemented(t *testing.T) {
	// Use a minimal driver that only implements Driver, not Writer.
	inner := &minimalDriver{}
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	_, err := cd.Mkdir(ctx, "0", "x")
	if err != drivers.ErrNotImplemented {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}

	err = cd.Move(ctx, drivers.Entry{}, "0")
	if err != drivers.ErrNotImplemented {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}

	err = cd.Rename(ctx, drivers.Entry{}, "x")
	if err != drivers.ErrNotImplemented {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}

	err = cd.Remove(ctx, drivers.Entry{})
	if err != drivers.ErrNotImplemented {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func TestCachedDriver_BatchRemove_NotImplemented(t *testing.T) {
	inner := &minimalDriver{}
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	err := cd.BatchRemove(ctx, nil)
	if err != drivers.ErrNotImplemented {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func TestCachedDriver_PathResolver_NotImplemented(t *testing.T) {
	inner := &minimalDriver{}
	cd := NewCachedDriver(inner)
	ctx := context.Background()

	_, err := cd.ResolvePath(ctx, "/")
	if err != drivers.ErrNotImplemented {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

// minimalDriver implements only drivers.Driver with no optional interfaces.
type minimalDriver struct{}

func (m *minimalDriver) Init(ctx context.Context) error                    { return nil }
func (m *minimalDriver) Drop(ctx context.Context) error                    { return nil }
func (m *minimalDriver) List(ctx context.Context, p string) ([]drivers.Entry, error) { return nil, nil }
func (m *minimalDriver) Read(ctx context.Context, e drivers.Entry, o, s int64) (io.ReadCloser, error) {
	panic("not expected")
}
