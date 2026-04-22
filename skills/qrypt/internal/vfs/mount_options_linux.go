//go:build linux

package vfs

// MountOptions returns platform-specific FUSE mount options for Linux.
func MountOptions(allowOther bool) []string {
	opts := []string{
		"-o", "rw",
		"-o", "nonempty",     // 允许挂载到非空目录
		"-o", "attr_timeout=5",    // 内核缓存文件属性 5s（远程删除更快可见）
		"-o", "entry_timeout=5",   // 内核缓存目录条目 5s（远程删除更快可见）
	}
	if allowOther {
		opts = append(opts, "-o", "allow_other") // 允许其他用户访问（需 /etc/fuse.conf 中 user_allow_other）
	}
	return opts
}
