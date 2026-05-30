//go:build !nofuse && darwin

package fs

func MountOptions(allowOther bool, volName string) []string {
	if volName == "" {
		volName = "QryptDrive"
	}
	opts := []string{
		"-o", "rw",
		"-o", "noappledouble",
		"-o", "defer_permissions",
		"-o", "volname=" + volName,
		"-o", "iosize=1048576",
		"-o", "attr_timeout=0",
		"-o", "entry_timeout=0",
	}
	if allowOther {
		opts = append(opts, "-o", "allow_other")
	}
	return opts
}
