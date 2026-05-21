package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"nhooyr.io/websocket"
)

func runCat(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以使用查看功能")
		os.Exit(1)
	}
	runCatViaDaemon(cmd, args, socketPath)
}

func runCatViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	path := args[0]
	mountName := resolveMount(cmd, &path)
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Send cat request
	resp, err := client.Call("cat_file", map[string]interface{}{
		"mount_name": mountName,
		"path":       path,
		"password":   password,
		"salt":       salt,
	})
	if err != nil {
		fmt.Printf("RPC 错误: %v\n", err)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("读取文件失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}

	// The daemon responds with binary frames (decrypted content) followed by EOF text frame.
	for {
		msgType, data, err := client.Conn().Read(client.Ctx())
		if err != nil {
			break
		}
		switch msgType {
		case websocket.MessageBinary:
			os.Stdout.Write(data)
		case websocket.MessageText:
			var eofResp struct {
				Result *struct {
					EOF bool `json:"eof"`
				} `json:"result,omitempty"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error,omitempty"`
			}
			json.Unmarshal(data, &eofResp)
			if eofResp.Error != nil {
				fmt.Fprintf(os.Stderr, "错误: %s\n", eofResp.Error.Message)
				os.Exit(1)
			}
			return
		}
	}
}


