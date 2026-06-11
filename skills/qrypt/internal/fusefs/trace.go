//go:build !nofuse

package fusefs

import (
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

func logFuseResult(op, path string, start time.Time, errc *int) {
	if errc == nil {
		return
	}
	if *errc != 0 {
		logging.L.Warnf("[FUSE] %s done: path=%s errc=%d took=%v\n", op, path, *errc, time.Since(start))
		return
	}
	logging.L.Debugf("[FUSE] %s done: path=%s errc=0 took=%v\n", op, path, time.Since(start))
}
