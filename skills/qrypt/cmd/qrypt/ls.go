package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

type ListEntry struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	DecName   string    `json:"dec_name"`
	IsDir     bool      `json:"is_dir"`
	Size      int64     `json:"size"`
	PlainSize int64     `json:"plain_size"`
	ModTime   time.Time `json:"mod_time"`
}

func runList(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
		runListViaDaemon(cmd, args, socketPath)
	} else {
		runListDirect(cmd, args)
	}
}

func runListViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	path := "/"
	if len(args) > 0 {
		path = args[0]
	}
	mountName := resolveMount(cmd, &path)
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	client, err := daemon.DialClient(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, rpcErr := client.Call("list_dir", protocol.ListDirParams{
		MountName: mountName,
		Path:      path,
		Password:  password,
		Salt:      salt,
	})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", err)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("列出目录失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}

	data, _ := json.Marshal(resp.Result)
	var result protocol.ListDirResult
	json.Unmarshal(data, &result)

	allEntries := make([]ListEntry, 0, len(result.Entries))
	for _, e := range result.Entries {
		allEntries = append(allEntries, ListEntry{
			Path:      e.DecName,
			Name:      e.Name,
			DecName:   e.DecName,
			IsDir:     e.IsDir,
			Size:      e.Size,
			PlainSize: e.PlainSize,
			ModTime:   time.UnixMilli(e.ModTime),
		})
	}

	sortTime, _ := cmd.Flags().GetBool("sort-time")
	sortSize, _ := cmd.Flags().GetBool("sort-size")
	if sortTime {
		sort.Slice(allEntries, func(i, j int) bool {
			return allEntries[i].ModTime.After(allEntries[j].ModTime)
		})
	} else if sortSize {
		sort.Slice(allEntries, func(i, j int) bool {
			return allEntries[i].PlainSize > allEntries[j].PlainSize
		})
	} else {
		sort.Slice(allEntries, func(i, j int) bool {
			return allEntries[i].Path < allEntries[j].Path
		})
	}

	outputJson, _ := cmd.Flags().GetBool("json")
	if outputJson {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(allEntries)
		return
	}

	showLong, _ := cmd.Flags().GetBool("long")
	showEnc, _ := cmd.Flags().GetBool("encrypted")
	humanReadable, _ := cmd.Flags().GetBool("human-readable")

	for _, le := range allEntries {
		if showLong {
			sizeStr := fmt.Sprintf("%10d", le.PlainSize)
			if humanReadable && !le.IsDir {
				sizeStr = fmt.Sprintf("%10s", formatBytes(le.PlainSize))
			} else if le.IsDir {
				sizeStr = fmt.Sprintf("%10s", "-")
			}
			displayPath := le.Path
			if le.IsDir {
				displayPath += "/"
			}
			if showEnc && le.Name != le.DecName {
				fmt.Printf("%s %s  %s  %s  [%s]\n", func() string {
					if le.IsDir {
						return "d"
					}
					return "-"
				}(), sizeStr, le.ModTime.Format("01-02 15:04"), displayPath, le.Name)
			} else {
				fmt.Printf("%s %s  %s  %s\n", func() string {
					if le.IsDir {
						return "d"
					}
					return "-"
				}(), sizeStr, le.ModTime.Format("01-02 15:04"), displayPath)
			}
		} else {
			displayPath := le.Path
			if le.IsDir {
				displayPath += "/"
			}
			fmt.Println(displayPath)
		}
	}
}

func runListDirect(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)

	path := "/"
	if len(args) > 0 {
		path = args[0]
	}
	mountName := resolveMount(cmd, &path)
	drv := loadToolDriverForMount(cfg, cipher, mountName)
	resolver, _ := drv.(drive.PathResolver)
	fullPath := config.ResolveFullPath(cfg.RootPath(), path)

	fid, err := resolver.ResolvePath(context.Background(), fullPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	recursive, _ := cmd.Flags().GetBool("recursive")

	var allEntries []ListEntry
	var listDir func(currentPath string, currentFid string) error
	listDir = func(currentPath string, currentFid string) error {
		entries, err := drv.List(context.Background(), currentFid)
		if err != nil {
			return err
		}
		for _, e := range entries {
			decName, decErr := cipher.DecryptSegment(e.Name)
			if decErr != nil {
				decName = e.Name
			}
			plainSize, err := cipher.DecryptedSize(e.Size)
			if err != nil {
				plainSize = e.Size
			}
			entryPath := decName
			if currentPath != "" && currentPath != "/" {
				entryPath = currentPath + "/" + decName
			}
			le := ListEntry{
				Path:      entryPath,
				Name:      e.Name,
				DecName:   decName,
				IsDir:     e.IsDir,
				Size:      e.Size,
				PlainSize: plainSize,
				ModTime:   e.ModTime,
			}
			allEntries = append(allEntries, le)
			if recursive && e.IsDir {
				if err := listDir(entryPath, e.ID); err != nil {
					fmt.Printf("无法列出子目录 %s: %v\n", entryPath, err)
				}
			}
		}
		return nil
	}
	if err := listDir("", fid); err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	sortTime, _ := cmd.Flags().GetBool("sort-time")
	sortSize, _ := cmd.Flags().GetBool("sort-size")
	if sortTime {
		sort.Slice(allEntries, func(i, j int) bool {
			return allEntries[i].ModTime.After(allEntries[j].ModTime)
		})
	} else if sortSize {
		sort.Slice(allEntries, func(i, j int) bool {
			return allEntries[i].PlainSize > allEntries[j].PlainSize
		})
	} else {
		sort.Slice(allEntries, func(i, j int) bool {
			return allEntries[i].Path < allEntries[j].Path
		})
	}

	outputJson, _ := cmd.Flags().GetBool("json")
	if outputJson {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(allEntries)
		return
	}

	showLong, _ := cmd.Flags().GetBool("long")
	showEnc, _ := cmd.Flags().GetBool("encrypted")
	humanReadable, _ := cmd.Flags().GetBool("human-readable")

	for _, le := range allEntries {
		if showLong {
			sizeStr := fmt.Sprintf("%10d", le.PlainSize)
			if humanReadable && !le.IsDir {
				sizeStr = fmt.Sprintf("%10s", formatBytes(le.PlainSize))
			} else if le.IsDir {
				sizeStr = fmt.Sprintf("%10s", "-")
			}
			displayPath := le.Path
			if le.IsDir {
				displayPath += "/"
			}
			if showEnc && le.Name != le.DecName {
				fmt.Printf("%s %s  %s  %s  [%s]\n", func() string {
					if le.IsDir {
						return "d"
					}
					return "-"
				}(), sizeStr, le.ModTime.Format("01-02 15:04"), displayPath, le.Name)
			} else {
				fmt.Printf("%s %s  %s  %s\n", func() string {
					if le.IsDir {
						return "d"
					}
					return "-"
				}(), sizeStr, le.ModTime.Format("01-02 15:04"), displayPath)
			}
		} else {
			displayPath := le.Path
			if le.IsDir {
				displayPath += "/"
			}
			fmt.Println(displayPath)
		}
	}
}
