package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
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
	api, err := apiFromCmd(cmd)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	path := "/"
	if len(args) > 0 {
		path = args[0]
	}
	mountName := resolveMount(cmd, &path)

	entries, err := api.List(context.Background(), mountName, path)
	if err != nil {
		fmt.Printf("列出目录失败: %v\n", err)
		os.Exit(1)
	}

	allEntries := make([]ListEntry, 0, len(entries))
	for _, e := range entries {
		allEntries = append(allEntries, ListEntry{
			Path:      e.DecName,
			Name:      e.Name,
			DecName:   e.DecName,
			IsDir:     e.IsDir,
			Size:      e.Size,
			PlainSize: e.PlainSize,
			ModTime:   e.ModTime,
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
		displayPath := le.Path
		if le.IsDir {
			displayPath += "/"
		}

		if showLong {
			sizeStr := fmt.Sprintf("%10d", le.PlainSize)
			if humanReadable && !le.IsDir {
				sizeStr = fmt.Sprintf("%10s", formatBytes(le.PlainSize))
			} else if le.IsDir {
				sizeStr = fmt.Sprintf("%10s", "-")
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
		} else if showEnc && le.Name != le.DecName {
			fmt.Printf("%s  [%s]\n", displayPath, le.Name)
		} else {
			fmt.Println(displayPath)
		}
	}
}


