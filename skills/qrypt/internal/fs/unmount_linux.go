//go:build linux

package fs

import (
	"os/exec"
)

func UnmountCommand(mountPoint string) *exec.Cmd {
	return exec.Command("fusermount", "-u", mountPoint)
}
