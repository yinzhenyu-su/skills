//go:build !nofuse && linux

package fusefs

import (
	"os/exec"
)

func UnmountCommand(mountPoint string) *exec.Cmd {
	return exec.Command("fusermount", "-u", mountPoint)
}
