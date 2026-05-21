package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"nhooyr.io/websocket"
)

func runCat(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	path := args[0]
	mountName := resolveMount(cmd, &path)
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

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


