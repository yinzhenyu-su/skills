package drivercache

import (
	"context"
	"io"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

// CachedDriver wraps a drivers.Driver and adds TTL-based caching for List
// results using DirCache. Mutation operations (Mkdir, Move, Rename, Remove,
// BatchRemove) invalidate the cache for affected parent directories.
//
// CachedDriver implements drivers.Driver unconditionally. It also implements
// drivers.Writer, drivers.BatchRemover, drivers.PathResolver, and
// drivers.CipherSetter by delegating to the inner driver when the inner
// implements those interfaces.
type CachedDriver struct {
	inner drivers.Driver
	cache *DirCache
}

// NewCachedDriver creates a CachedDriver that wraps the given driver with a
// default DirCache (60-second TTL).
func NewCachedDriver(inner drivers.Driver) *CachedDriver {
	return &CachedDriver{
		inner: inner,
		cache: NewDirCache(0), // default 60s TTL
	}
}

// ---------------------------------------------------------------------------
// drivers.Driver
// ---------------------------------------------------------------------------

// Init delegates to the inner driver.
func (d *CachedDriver) Init(ctx context.Context) error {
	return d.inner.Init(ctx)
}

// Drop clears the directory cache, then delegates to the inner driver.
func (d *CachedDriver) Drop(ctx context.Context) error {
	d.cache = NewDirCache(0)
	return d.inner.Drop(ctx)
}

// List returns directory entries, using DirCache when possible. On a cache
// miss it delegates to the inner driver and caches the result.
func (d *CachedDriver) List(ctx context.Context, parentID string) ([]drivers.Entry, error) {
	if entries, ok := d.cache.GetDir(parentID); ok {
		return entries, nil
	}
	entries, err := d.inner.List(ctx, parentID)
	if err != nil {
		return nil, err
	}
	d.cache.SetDir(parentID, entries)
	return entries, nil
}

// Read delegates to the inner driver.
func (d *CachedDriver) Read(ctx context.Context, entry drivers.Entry, offset, size int64) (io.ReadCloser, error) {
	return d.inner.Read(ctx, entry, offset, size)
}

// ---------------------------------------------------------------------------
// drivers.Writer (optional — delegated to inner)
// ---------------------------------------------------------------------------

// Mkdir creates a new directory and invalidates the cache for parentID.
func (d *CachedDriver) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	w, ok := d.inner.(drivers.Writer)
	if !ok {
		return drivers.Entry{}, drivers.ErrNotImplemented
	}
	entry, err := w.Mkdir(ctx, parentID, name)
	if err != nil {
		return drivers.Entry{}, err
	}
	d.cache.RemoveDir(parentID)
	return entry, nil
}

// Move moves an entry to a different parent and invalidates the cache for
// both the old and new parent directories.
func (d *CachedDriver) Move(ctx context.Context, entry drivers.Entry, dstParentID string) error {
	w, ok := d.inner.(drivers.Writer)
	if !ok {
		return drivers.ErrNotImplemented
	}
	err := w.Move(ctx, entry, dstParentID)
	if err != nil {
		return err
	}
	d.cache.RemoveDir(entry.ParentID)
	d.cache.RemoveDir(dstParentID)
	return nil
}

// Rename renames an entry and invalidates the cache for its parent directory.
func (d *CachedDriver) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	w, ok := d.inner.(drivers.Writer)
	if !ok {
		return drivers.ErrNotImplemented
	}
	err := w.Rename(ctx, entry, newName)
	if err != nil {
		return err
	}
	d.cache.RemoveDir(entry.ParentID)
	return nil
}

// Remove removes an entry and invalidates the cache for its parent directory.
func (d *CachedDriver) Remove(ctx context.Context, entry drivers.Entry) error {
	w, ok := d.inner.(drivers.Writer)
	if !ok {
		return drivers.ErrNotImplemented
	}
	err := w.Remove(ctx, entry)
	if err != nil {
		return err
	}
	d.cache.RemoveDir(entry.ParentID)
	return nil
}

// ---------------------------------------------------------------------------
// drivers.BatchRemover (optional — delegated to inner)
// ---------------------------------------------------------------------------

// BatchRemove removes multiple entries and invalidates the cache for each
// affected parent directory.
func (d *CachedDriver) BatchRemove(ctx context.Context, entries []drivers.Entry) error {
	br, ok := d.inner.(drivers.BatchRemover)
	if !ok {
		return drivers.ErrNotImplemented
	}
	err := br.BatchRemove(ctx, entries)
	if err != nil {
		return err
	}
	for _, e := range entries {
		d.cache.RemoveDir(e.ParentID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// drivers.Uploader (optional — delegated to inner)
// ---------------------------------------------------------------------------

// Put uploads a file and invalidates the cache for parentID on success.
func (d *CachedDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	u, ok := d.inner.(drivers.Uploader)
	if !ok {
		return drivers.Entry{}, drivers.ErrNotImplemented
	}
	entry, err := u.Put(ctx, parentID, name, size, body)
	if err != nil {
		return drivers.Entry{}, err
	}
	d.cache.RemoveDir(parentID)
	return entry, nil
}

// ---------------------------------------------------------------------------
// drivers.PathResolver (optional — delegated to inner)
// ---------------------------------------------------------------------------

// ResolvePath resolves a logical path to a backend-specific file identifier.
func (d *CachedDriver) ResolvePath(ctx context.Context, path string) (string, error) {
	pr, ok := d.inner.(drivers.PathResolver)
	if !ok {
		return "", drivers.ErrNotImplemented
	}
	return pr.ResolvePath(ctx, path)
}

// ---------------------------------------------------------------------------
// drivers.CipherSetter (optional — delegated to inner)
// ---------------------------------------------------------------------------

// SetCipher injects the cipher into the inner driver when it supports it.
func (d *CachedDriver) SetCipher(c cipher.Cipher) {
	if s, ok := d.inner.(drivers.CipherSetter); ok {
		s.SetCipher(c)
	}
}
