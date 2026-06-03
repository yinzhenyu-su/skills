//go:build !nofuse && linux

package fusefs

func MountOptions(allowOther bool, _ string) []string {
	opts := []string{
		"-o", "rw",
		"-o", "nonempty",
		"-o", "attr_timeout=0",
		"-o", "entry_timeout=0",
	}
	if allowOther {
		opts = append(opts, "-o", "allow_other")
	}
	return opts
}
