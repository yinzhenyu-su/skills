//go:build linux

package fs

func MountOptions(allowOther bool) []string {
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
