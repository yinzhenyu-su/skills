package daemon

import (
	"net"
	"os"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

// FindSocketPath returns the qryptd socket path from env or default.
func FindSocketPath() string {
	if p := os.Getenv("QRYPTD_SOCKET"); p != "" {
		return p
	}
	return config.WorkDir() + "/qryptd.sock"
}

// IsDaemonRunning checks whether qryptd is listening on the given socket.
func IsDaemonRunning(socketPath string) bool {
	if socketPath == "" {
		socketPath = FindSocketPath()
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
