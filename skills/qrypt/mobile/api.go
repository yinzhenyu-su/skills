// Package mobile is the gomobile entry point for Android / iOS.
//
// Build:
//
//	gomobile bind -target=android -o libqrypt.aar ./mobile/
//	gomobile bind -target=ios     -o QryptCore.xcframework ./mobile/
//
// All exported methods use gomobile-compatible types only:
// string / []byte / int64 / bool / error and named interfaces.
// Compound types ([]FileEntry, *ProgressEntry, ...) cross the JNI boundary as JSON strings.
package mobile

import (
	"context"
	"encoding/json"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// MobileAPI wraps qrypt.FileAPI with a JNI-safe surface.
// Constructed via NewQryptAPI.
type MobileAPI struct {
	inner   *qrypt.FileAPI
	creds   qrypt.CredentialStore
	streams *streamRegistry
}

// Inner exposes the underlying FileAPI for advanced callers (e.g. unit tests
// inside the same Go module). NOT visible across the gomobile boundary.
func (m *MobileAPI) Inner() *qrypt.FileAPI { return m.inner }

// Shutdown releases all driver sessions, stops worker goroutines, and closes streams.
// Call from Android's Application.onTerminate or iOS app delegate.
func (m *MobileAPI) Shutdown() {
	if m.streams != nil {
		m.streams.closeAll()
	}
	m.inner.Shutdown()
}

// ---------------------------------------------------------------------------
// Read-only ops
// ---------------------------------------------------------------------------

// List returns the JSON-encoded array of FileEntry for the given mount path.
func (m *MobileAPI) List(mount, path string) (string, error) {
	entries, err := m.inner.List(context.Background(), mount, path)
	if err != nil {
		return "", err
	}
	return marshalJSON(entries)
}

// Stat returns the JSON-encoded FileEntry for the given path, or empty string if not found.
func (m *MobileAPI) Stat(mount, path string) (string, error) {
	entry, err := m.inner.Stat(context.Background(), mount, path)
	if err != nil {
		return "", err
	}
	return marshalJSON(entry)
}

// Find returns JSON array of matching FileEntry. Empty pattern matches all.
func (m *MobileAPI) Find(mount, path, pattern string, maxDepth, maxMatches int, caseSensitive bool) (string, error) {
	results, err := m.inner.Find(context.Background(), mount, path, pattern, maxDepth, maxMatches, caseSensitive)
	if err != nil {
		return "", err
	}
	return marshalJSON(results)
}

// ---------------------------------------------------------------------------
// Mutation ops
// ---------------------------------------------------------------------------

// Mkdir creates a directory at the given path (intermediate dirs created on demand by FileAPI).
func (m *MobileAPI) Mkdir(mount, path string) error {
	return m.inner.Mkdir(context.Background(), mount, path)
}

// Remove deletes a file or directory. recursive=true allows non-empty directory removal.
func (m *MobileAPI) Remove(mount, path string, recursive bool) error {
	return m.inner.Remove(context.Background(), mount, path, recursive)
}

// Move renames or moves a file/directory. Set noClobber=true to fail if destination exists.
func (m *MobileAPI) Move(mount, oldPath, newPath string, noClobber bool) error {
	return m.inner.Move(context.Background(), mount, oldPath, newPath, qrypt.MoveOptions{NoClobber: noClobber})
}

// ---------------------------------------------------------------------------
// File transfer ops (streaming via local filesystem paths)
// ---------------------------------------------------------------------------

// PushFile uploads a local file to the remote path. Streaming: O(1) memory.
// Android: copy content:// Uri to app cache dir first, then pass that path.
func (m *MobileAPI) PushFile(mount, localPath, remotePath string) error {
	return m.inner.Push(context.Background(), mount, localPath, remotePath)
}

// PullFile downloads a remote file to the given local path. Streaming: O(1) memory.
func (m *MobileAPI) PullFile(mount, remotePath, localPath string) error {
	return m.inner.Pull(context.Background(), mount, remotePath, localPath)
}

// ---------------------------------------------------------------------------
// Small data convenience (full buffer in memory — only for small files)
// ---------------------------------------------------------------------------

// ReadAll downloads the full file into a single []byte. Avoid for files > a few MB;
// use OpenRead + ReadChunk for streaming.
func (m *MobileAPI) ReadAll(mount, path string) ([]byte, error) {
	rc, err := m.inner.Read(context.Background(), mount, path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var out []byte
	buf := make([]byte, 32*1024)
	for {
		n, rerr := rc.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Status / observation
// ---------------------------------------------------------------------------

// ActiveTransfersJSON returns the JSON array of in-flight upload/download tasks.
func (m *MobileAPI) ActiveTransfersJSON() string {
	entries := m.inner.Progress().Active()
	data, _ := json.Marshal(entries)
	return string(data)
}

func marshalJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
