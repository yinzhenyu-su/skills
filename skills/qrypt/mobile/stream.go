package mobile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync"
)

// ReadStream is a JNI-safe handle for chunked reads. Returned as an opaque string ID.
//
// Usage from Kotlin:
//
//	val id = api.openRead("quark", "/movie.mp4")
//	while (true) {
//	    val chunk = api.readChunk(id, 1024 * 1024)
//	    if (chunk.isEmpty()) break
//	    output.write(chunk)
//	}
//	api.closeRead(id)
//
// IDs are random 16-byte hex strings, scoped to this MobileAPI instance.
type streamRegistry struct {
	mu      sync.Mutex
	streams map[string]io.ReadCloser
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{streams: make(map[string]io.ReadCloser)}
}

func (r *streamRegistry) put(rc io.ReadCloser) string {
	id := newStreamID()
	r.mu.Lock()
	r.streams[id] = rc
	r.mu.Unlock()
	return id
}

func (r *streamRegistry) get(id string) (io.ReadCloser, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rc, ok := r.streams[id]
	return rc, ok
}

func (r *streamRegistry) delete(id string) (io.ReadCloser, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rc, ok := r.streams[id]
	if ok {
		delete(r.streams, id)
	}
	return rc, ok
}

func (r *streamRegistry) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, rc := range r.streams {
		_ = rc.Close()
		delete(r.streams, id)
	}
}

func newStreamID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// OpenRead opens a streaming read handle. Returns an opaque ID for ReadChunk/CloseRead.
func (m *MobileAPI) OpenRead(mount, path string) (string, error) {
	rc, err := m.inner.Read(context.Background(), mount, path)
	if err != nil {
		return "", err
	}
	return m.streams.put(rc), nil
}

// ReadChunk reads up to maxBytes from the stream. Returns an empty slice on EOF.
// Callers should keep calling until they get an empty slice, then CloseRead.
func (m *MobileAPI) ReadChunk(id string, maxBytes int) ([]byte, error) {
	rc, ok := m.streams.get(id)
	if !ok {
		return nil, errors.New("stream not found: " + id)
	}
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}
	buf := make([]byte, maxBytes)
	n, err := rc.Read(buf)
	if err == io.EOF {
		return buf[:n], nil
	}
	if err != nil {
		return buf[:n], err
	}
	return buf[:n], nil
}

// CloseRead releases the stream handle. Safe to call multiple times.
func (m *MobileAPI) CloseRead(id string) error {
	rc, ok := m.streams.delete(id)
	if !ok {
		return nil
	}
	return rc.Close()
}
