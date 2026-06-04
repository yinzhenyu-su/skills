package mockdrive

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestDriverFileLifecycle(t *testing.T) {
	ctx := context.Background()
	drv := NewDriver()

	dir, err := drv.Mkdir(ctx, "0", "docs")
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	entry, err := drv.Put(ctx, dir.ID, "note.txt", 5, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	r, err := drv.Read(ctx, entry, 1, 3)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != "ell" {
		t.Fatalf("Read returned %q, want %q", data, "ell")
	}

	resolved, err := drv.ResolvePath(ctx, "/docs/note.txt")
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}
	if resolved != entry.ID {
		t.Fatalf("ResolvePath returned %q, want %q", resolved, entry.ID)
	}

	if err := drv.Remove(ctx, dir); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
}
