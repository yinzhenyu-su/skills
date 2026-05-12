//go:build darwin

package fs

import (
	"os/exec"
)

func UnmountCommand(mountPoint string) *exec.Cmd {
	return exec.Command("umount", "-f", mountPoint)
}
