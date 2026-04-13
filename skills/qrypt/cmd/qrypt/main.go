package main

import (
	"fmt"
	"os"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	"github.com/yinzhenyu/skills/qrypt/internal/vfs"
)

func main() {
	var cookie string
	var cacheDir string
	var mountPoint string
	var password string
	var salt string
	var rootPath string

	var rootCmd = &cobra.Command{
		Use:   "qrypt",
		Short: "Qrypt - Quark Drive Rclone-Compatible Crypt Mount Tool",
	}

	var mountCmd = &cobra.Command{
		Use:   "mount",
		Short: "Mount Quark Drive to a local directory",
		Run: func(cmd *cobra.Command, args []string) {
			if cookie == "" || mountPoint == "" || password == "" {
				fmt.Println("Error: cookie, mount, and password are required")
				os.Exit(1)
			}

			// 1. 初始化加密引擎 (rclone 兼容)
			cipher, err := crypt.NewRcloneCipher(password, salt)
			if err != nil {
				fmt.Printf("Cipher init failed: %v\n", err)
				os.Exit(1)
			}

			// 2. 初始化驱动并验证
			d := driver.NewQuarkDriver(cookie)
			if err := d.Auth(); err != nil {
				fmt.Printf("Auth failed: %v\n", err)
				os.Exit(1)
			}

			// 3. 解析根目录 FID (使用明文路径解析，因为 rclone 的 remote 路径通常是明文)
			rootFid := "0"
			if rootPath != "/" && rootPath != "" {
				fmt.Printf("Resolving plaintext path: %s...\n", rootPath)
				
				// 注意：这里使用 ResolvePath 直接解析明文路径
				fid, err := d.ResolvePath(rootPath)
				if err != nil {
					fmt.Printf("Failed to resolve root path: %v\n", err)
					os.Exit(1)
				}
				rootFid = fid
			}
			fmt.Printf("Using Root FID: %s\n", rootFid)

			// 4. 初始化缓存
			dbPath := "qrypt_cache.db"
			cm, err := cache.NewCacheManager(cacheDir, dbPath, 10*1024*1024*1024)
			if err != nil {
				fmt.Printf("Cache init failed: %v\n", err)
				os.Exit(1)
			}

			// 5. 挂载
			fs := vfs.NewQryptFS(d, cm, rootFid, cipher)
			host := fuse.NewFileSystemHost(fs)
			host.Mount(mountPoint, nil)
		},
	}

	mountCmd.Flags().StringVarP(&cookie, "cookie", "c", "", "Quark Drive Cookie")
	mountCmd.Flags().StringVarP(&cacheDir, "cache", "a", "./cache", "Local cache directory")
	mountCmd.Flags().StringVarP(&mountPoint, "mount", "m", "", "Local mount point")
	mountCmd.Flags().StringVarP(&password, "password", "p", "", "Rclone password")
	mountCmd.Flags().StringVarP(&salt, "salt", "s", "", "Rclone salt (optional)")
	mountCmd.Flags().StringVarP(&rootPath, "root-path", "r", "/", "Quark Drive path to mount")

	rootCmd.AddCommand(mountCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
