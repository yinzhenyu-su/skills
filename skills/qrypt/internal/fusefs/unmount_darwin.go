//go:build !nofuse && darwin

package fusefs

import (
	"os/exec"
)

func UnmountCommand(mountPoint string) *exec.Cmd {
	return exec.Command("umount", "-f", mountPoint)
}
