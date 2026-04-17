//go:build darwin

package vfs

// MountOptions returns platform-specific FUSE mount options for macOS.
func MountOptions() []string {
	return []string{
		"-o", "rw",
		"-o", "noappledouble",   // 减少 AppleDouble 文件
		"-o", "defer_permissions", // macOS 推荐
		"-o", "volname=QuarkDrive",
		"-o", "attr_timeout=60",   // 内核缓存文件属性 60s（匹配 MetadataTTL）
		"-o", "entry_timeout=60",  // 内核缓存目录条目 60s
	}
}
