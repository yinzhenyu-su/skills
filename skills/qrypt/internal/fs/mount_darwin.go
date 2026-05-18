//go:build !nofuse && darwin

package fs

func MountOptions(allowOther bool) []string {
	opts := []string{
		"-o", "rw",
		"-o", "noappledouble",
		"-o", "defer_permissions",
		"-o", "volname=QuarkDrive",
		"-o", "iosize=1048576",
		"-o", "attr_timeout=0",
		"-o", "entry_timeout=0",
	}
	if allowOther {
		opts = append(opts, "-o", "allow_other")
	}
	return opts
}
