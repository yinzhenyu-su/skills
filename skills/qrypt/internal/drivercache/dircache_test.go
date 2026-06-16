package drivercache

import (
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

func TestDirCacheHit(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	entries := []drivers.Entry{
		{ID: "1", Name: "foo", IsDir: true},
		{ID: "2", Name: "bar.txt", Size: 100},
	}
	c.SetDir("parent1", entries)

	got, ok := c.GetDir("parent1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Name != "foo" {
		t.Errorf("expected entry 'foo', got %s", got[0].Name)
	}
}

func TestDirCacheMiss(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	_, ok := c.GetDir("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestDirCacheExpiry(t *testing.T) {
	c := NewDirCache(50 * time.Millisecond)

	entries := []drivers.Entry{{ID: "1", Name: "expire_me"}}
	c.SetDir("exp", entries)

	// Should be a hit initially
	if _, ok := c.GetDir("exp"); !ok {
		t.Fatal("expected hit before expiry")
	}

	time.Sleep(60 * time.Millisecond)

	// Should be a miss after expiry
	if _, ok := c.GetDir("exp"); ok {
		t.Fatal("expected miss after expiry")
	}
}

func TestDirCacheRemove(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	entries := []drivers.Entry{{ID: "1", Name: "remove_me"}}
	c.SetDir("del", entries)
	c.RemoveDir("del")

	if _, ok := c.GetDir("del"); ok {
		t.Fatal("expected miss after remove")
	}
}

func TestDirCacheSetReplaces(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	c.SetDir("parent", []drivers.Entry{{ID: "1", Name: "old"}})
	c.SetDir("parent", []drivers.Entry{{ID: "2", Name: "new"}})

	got, ok := c.GetDir("parent")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 1 || got[0].Name != "new" {
		t.Errorf("expected [new], got %v", got)
	}
}

func TestNegCacheHit(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	c.SetNeg("parent1", "missing.txt")

	if !c.GetNeg("parent1", "missing.txt") {
		t.Fatal("expected neg cache hit")
	}
}

func TestNegCacheMiss(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	if c.GetNeg("parent1", "never_set") {
		t.Fatal("expected neg cache miss")
	}
}

func TestNegCacheExpiry(t *testing.T) {
	c := NewDirCache(60 * time.Second)
	c.negCacheTTL = 50 * time.Millisecond

	c.SetNeg("p", "f")

	if !c.GetNeg("p", "f") {
		t.Fatal("expected hit before expiry")
	}

	time.Sleep(70 * time.Millisecond)

	if c.GetNeg("p", "f") {
		t.Fatal("expected miss after expiry")
	}
}

func TestNegCacheDelete(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	c.SetNeg("p", "f")
	c.DeleteNeg("p", "f")

	if c.GetNeg("p", "f") {
		t.Fatal("expected miss after delete")
	}
}

func TestNegCacheIndependentKeys(t *testing.T) {
	c := NewDirCache(60 * time.Second)

	c.SetNeg("p1", "a")
	c.SetNeg("p2", "b")

	if !c.GetNeg("p1", "a") {
		t.Error("expected hit for p1:a")
	}
	if !c.GetNeg("p2", "b") {
		t.Error("expected hit for p2:b")
	}
	if c.GetNeg("p1", "b") {
		t.Error("expected miss for p1:b")
	}
	c.DeleteNeg("p1", "a")
	if c.GetNeg("p1", "a") {
		t.Error("expected miss for p1:a after delete")
	}
	if !c.GetNeg("p2", "b") {
		t.Error("expected hit for p2:b after unrelated delete")
	}
}

func TestNewDirCacheDefaultTTL(t *testing.T) {
	c := NewDirCache(0)
	if c.dirCacheTTL != 60*time.Second {
		t.Errorf("expected default dirCacheTTL 60s, got %v", c.dirCacheTTL)
	}
	if c.negCacheTTL != 60*time.Second {
		t.Errorf("expected default negCacheTTL 60s, got %v", c.negCacheTTL)
	}
}
