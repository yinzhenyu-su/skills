package backend

import (
	"context"
	"time"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

type Entry = qrypt.Entry
type Driver = qrypt.Driver
type Writer = qrypt.Writer
type Uploader = qrypt.Uploader
type PathResolver = qrypt.PathResolver

// Deprecated: use qrypt.WithMtime.
func WithMtime(ctx context.Context, mtime time.Time) context.Context {
	return qrypt.WithMtime(ctx, mtime)
}

// Deprecated: use qrypt.MtimeFromContext.
func MtimeFromContext(ctx context.Context) (time.Time, bool) {
	return qrypt.MtimeFromContext(ctx)
}
