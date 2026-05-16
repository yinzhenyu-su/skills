package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrintCacheMetrics_noDir(t *testing.T) {
	dir := t.TempDir()
	printCacheMetrics(filepath.Join(dir, "nonexistent"))
}

func TestPrintCacheMetrics_emptyDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "staging"), 0o755)
	os.MkdirAll(filepath.Join(dir, "reading"), 0o755)
	printCacheMetrics(dir)
}

func TestPrintCacheMetrics_withData(t *testing.T) {
	dir := t.TempDir()

	// pending journal
	journalPath := filepath.Join(dir, "pending.jsonl")
	os.WriteFile(journalPath, []byte("line1\nline2\n"), 0o644)

	// staging files
	stagingDir := filepath.Join(dir, "staging")
	os.MkdirAll(stagingDir, 0o755)
	os.WriteFile(filepath.Join(stagingDir, "f1.staging"), []byte("data1"), 0o644)
	os.WriteFile(filepath.Join(stagingDir, "f2.staging"), []byte("data22"), 0o644)

	// reading cache
	readingDir := filepath.Join(dir, "reading")
	os.MkdirAll(readingDir, 0o755)
	os.WriteFile(filepath.Join(readingDir, "a_batch_0.dec.batch"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(readingDir, "a_batch_1.dec.batch"), []byte("y"), 0o644)

	printCacheMetrics(dir)
}
