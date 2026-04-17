//go:build linux

package vfs

// MountOptions returns platform-specific FUSE mount options for Linux.
func MountOptions() []string {
	return []string{
		"-o", "rw",
		"-o", "allow_other",  // 允许其他用户访问（需 /etc/fuse.conf 中 user_allow_other）
		"-o", "nonempty",     // 允许挂载到非空目录
		"-o", "attr_timeout=60",   // 内核缓存文件属性 60s（匹配 MetadataTTL）
		"-o", "entry_timeout=60",  // 内核缓存目录条目 60s
	}
}
