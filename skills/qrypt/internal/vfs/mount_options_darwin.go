//go:build darwin

package vfs

// MountOptions returns platform-specific FUSE mount options for macOS.
func MountOptions(allowOther bool) []string {
	opts := []string{
		"-o", "rw",
		"-o", "noappledouble",   // 减少 AppleDouble 文件
		"-o", "defer_permissions", // macOS 推荐
		"-o", "volname=QuarkDrive",
		"-o", "iosize=1048576",   // 提高 I/O 步长至 1MB
		"-o", "attr_timeout=5",    // 内核缓存文件属性 5s
		"-o", "entry_timeout=5",   // 内核缓存目录条目 5s
	}
	if allowOther {
		opts = append(opts, "-o", "allow_other")
	}
	return opts
}
