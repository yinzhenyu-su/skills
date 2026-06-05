package localfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

type LocalDriver struct {
	root    string
	rootDir string
	cipher  cipher.Cipher
}

func (d *LocalDriver) SetCipher(c cipher.Cipher) { d.cipher = c }

var (
	_ drivers.Driver       = (*LocalDriver)(nil)
	_ drivers.Writer       = (*LocalDriver)(nil)
	_ drivers.Uploader     = (*LocalDriver)(nil)
	_ drivers.CipherSetter = (*LocalDriver)(nil)
)

func init() {
	drivers.Register("localfs", func(params drivers.Params) (drivers.Driver, error) {
		root := params["local_root"]
		if root == "" {
			root = params["root_path"]
		}
		if root == "" {
			return nil, fmt.Errorf("missing local_root for localfs driver")
		}
		return NewDriver(root), nil
	})
}

func NewDriver(root string) *LocalDriver {
	return &LocalDriver{root: filepath.Clean(root)}
}

func (d *LocalDriver) Init(ctx context.Context) error {
	info, err := os.Stat(d.root)
	if err != nil {
		return fmt.Errorf("localfs: root %s: %w", d.root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("localfs: root %s is not a directory", d.root)
	}
	d.rootDir = d.root
	return nil
}

func (d *LocalDriver) Drop(ctx context.Context) error {
	return nil
}

func (d *LocalDriver) List(ctx context.Context, parentID string) ([]drivers.Entry, error) {
	dir := d.resolve(parentID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("localfs: readdir %s: %w", dir, err)
	}
	result := make([]drivers.Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, drivers.Entry{
			ID:      filepath.Join(dir, e.Name()),
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return result, nil
}

func (d *LocalDriver) Read(ctx context.Context, entry drivers.Entry, offset, size int64) (io.ReadCloser, error) {
	f, err := os.Open(entry.ID)
	if err != nil {
		return nil, fmt.Errorf("localfs: open %s: %w", entry.ID, err)
	}
	if offset > 0 {
		_, err = f.Seek(offset, io.SeekStart)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("localfs: seek %s: %w", entry.ID, err)
		}
	}
	if size > 0 {
		return struct {
			io.Reader
			io.Closer
		}{io.LimitReader(f, size), f}, nil
	}
	return f, nil
}

func (d *LocalDriver) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	parent := d.resolve(parentID)
	path := filepath.Join(parent, name)
	err := os.Mkdir(path, 0755)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("localfs: mkdir %s: %w", path, err)
	}
	return drivers.Entry{ID: path, Name: name, IsDir: true, ModTime: time.Now()}, nil
}

func (d *LocalDriver) Move(ctx context.Context, entry drivers.Entry, dstParentID string) error {
	dst := filepath.Join(d.resolve(dstParentID), filepath.Base(entry.ID))
	return os.Rename(entry.ID, dst)
}

func (d *LocalDriver) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	parent := filepath.Dir(entry.ID)
	dst := filepath.Join(parent, newName)
	return os.Rename(entry.ID, dst)
}

func (d *LocalDriver) Remove(ctx context.Context, entry drivers.Entry) error {
	if entry.IsDir {
		return os.RemoveAll(entry.ID)
	}
	return os.Remove(entry.ID)
}

func (d *LocalDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	parent := d.resolve(parentID)
	path := filepath.Join(parent, name)
	f, err := os.Create(path)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("localfs: create %s: %w", path, err)
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("localfs: write %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		return drivers.Entry{ID: path, Name: name}, nil
	}
	return drivers.Entry{ID: path, Name: name, Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (d *LocalDriver) ResolvePath(ctx context.Context, path string) (string, error) {
	cleanPath := filepath.Clean(path)

	// Check if path is already under root (CLI prepends cfg.RootPath()).
	rel, err := filepath.Rel(d.root, cleanPath)
	if err == nil && !strings.HasPrefix(rel, "..") {
		if d.cipher != nil && rel != "." {
			return d.encryptRel(rel), nil
		}
		return cleanPath, nil
	}

	// Path is outside root — treat as relative virtual path.
	if d.cipher != nil {
		return d.encryptRel(path), nil
	}

	abs := filepath.Join(d.root, cleanPath)
	abs = filepath.Clean(abs)
	rel, err = filepath.Rel(d.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %s escapes root", path)
	}
	return abs, nil
}

// encryptRel encrypts each path segment and joins onto d.root.
// Example: "docs/file.txt" → d.root + "/" + E("docs") + "/" + E("file.txt")
func (d *LocalDriver) encryptRel(rel string) string {
	segs := strings.Split(filepath.ToSlash(rel), "/")
	parts := make([]string, 0, len(segs)+1)
	parts = append(parts, d.root)
	for _, s := range segs {
		if s == "" || s == "." {
			continue
		}
		parts = append(parts, d.cipher.EncryptSegment(s))
	}
	return filepath.Join(parts...)
}

func (d *LocalDriver) resolve(id string) string {
	if id == "" || id == "0" || id == "/" {
		return d.root
	}
	return id
}
