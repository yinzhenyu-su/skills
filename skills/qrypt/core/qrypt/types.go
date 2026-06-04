package qrypt

import "time"

type FileEntry struct {
	ID        string
	Path      string
	Name      string
	DecName   string
	IsDir     bool
	Size      int64
	PlainSize int64
	ModTime   time.Time
}

type FileAPIStats struct {
	FileCount int
	DirCount  int
	TotalSize int64
}
