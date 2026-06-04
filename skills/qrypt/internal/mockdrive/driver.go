package mockdrive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

type memFile struct {
	id       string
	name     string
	isDir    bool
	data     []byte
	parentID string
	modTime  time.Time
}

type MockDriver struct {
	mu     sync.RWMutex
	files  map[string]*memFile
	nextID atomic.Int64
}

var (
	_ drivers.Driver   = (*MockDriver)(nil)
	_ drivers.Writer   = (*MockDriver)(nil)
	_ drivers.Uploader = (*MockDriver)(nil)
)

func NewDriver() *MockDriver {
	d := &MockDriver{files: make(map[string]*memFile)}
	d.nextID.Store(1)
	d.files["0"] = &memFile{
		id: "0", name: "", isDir: true, parentID: "",
		modTime: time.Now(),
	}
	return d
}

func (d *MockDriver) allocID() string {
	return fmt.Sprintf("mock_fid_%d", d.nextID.Add(1))
}

func (d *MockDriver) Init(ctx context.Context) error { return nil }

func (d *MockDriver) Drop(ctx context.Context) error { return nil }

func (d *MockDriver) List(ctx context.Context, parentID string) ([]drivers.Entry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var result []drivers.Entry
	for _, f := range d.files {
		if f.parentID == parentID {
			result = append(result, drivers.Entry{
				ID: f.id, Name: f.name, IsDir: f.isDir,
				Size: int64(len(f.data)), ModTime: f.modTime,
			})
		}
	}
	if result == nil {
		result = []drivers.Entry{}
	}
	return result, nil
}

func (d *MockDriver) Read(ctx context.Context, entry drivers.Entry, offset, size int64) (io.ReadCloser, error) {
	d.mu.RLock()
	f, ok := d.files[entry.ID]
	d.mu.RUnlock()
	if !ok {
		return nil, drivers.ErrNotFound
	}
	if offset >= int64(len(f.data)) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	end := offset + size
	if end > int64(len(f.data)) {
		end = int64(len(f.data))
	}
	return io.NopCloser(bytes.NewReader(f.data[offset:end])), nil
}

func (d *MockDriver) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	id := d.allocID()
	d.files[id] = &memFile{
		id: id, name: name, isDir: true, parentID: parentID,
		modTime: time.Now(),
	}
	return drivers.Entry{ID: id, Name: name, IsDir: true, ModTime: time.Now()}, nil
}

func (d *MockDriver) Move(ctx context.Context, entry drivers.Entry, dstParentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.files[entry.ID]
	if !ok {
		return drivers.ErrNotFound
	}
	f.parentID = dstParentID
	return nil
}

func (d *MockDriver) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.files[entry.ID]
	if !ok {
		return drivers.ErrNotFound
	}
	for _, other := range d.files {
		if other.id != entry.ID && other.parentID == f.parentID && other.name == newName {
			return drivers.ErrDirAlreadyExists
		}
	}
	f.name = newName
	return nil
}

func (d *MockDriver) Remove(ctx context.Context, entry drivers.Entry) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.files[entry.ID]; !ok {
		return drivers.ErrNotFound
	}
	d.removeRecursive(entry.ID)
	return nil
}

func (d *MockDriver) removeRecursive(id string) {
	delete(d.files, id)
	for _, f := range d.files {
		if f.parentID == id {
			d.removeRecursive(f.id)
		}
	}
}

func (d *MockDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return drivers.Entry{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	id := d.allocID()
	d.files[id] = &memFile{
		id: id, name: name, isDir: false, parentID: parentID,
		data: data, modTime: time.Now(),
	}
	return drivers.Entry{ID: id, Name: name, Size: int64(len(data)), ModTime: time.Now()}, nil
}

func (d *MockDriver) ResolvePath(ctx context.Context, path string) (string, error) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	currentID := "0"
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		d.mu.RLock()
		found := false
		for _, f := range d.files {
			if f.parentID == currentID && f.name == seg {
				currentID = f.id
				found = true
				break
			}
		}
		d.mu.RUnlock()
		if !found {
			return "", fmt.Errorf("path segment %s not found", seg)
		}
	}
	return currentID, nil
}
