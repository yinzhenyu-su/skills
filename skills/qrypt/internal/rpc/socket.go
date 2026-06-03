package rpc

import (
	"net"
	"os"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func FindSocketPath() string {
	if p := os.Getenv("QRYPTD_SOCKET"); p != "" {
		return p
	}
	return config.WorkDir() + "/qryptd.sock"
}

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
