package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

func runFind(cmd *cobra.Command, args []string) {
	api, err := apiFromCmd(cmd)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	rootPath := "/"
	pattern := ""
	if len(args) >= 1 {
		rootPath = args[0]
	}
	if len(args) >= 2 {
		pattern = args[1]
	} else if len(args) == 1 {
		if !strings.Contains(rootPath, "/") && rootPath != "." {
			pattern = rootPath
			rootPath = "/"
		}
	}

	mountName := resolveMount(cmd, &rootPath)
	caseSensitive, _ := cmd.Flags().GetBool("case-sensitive")
	maxDepth, _ := cmd.Flags().GetInt("maxdepth")
	maxMatches, _ := cmd.Flags().GetInt("max")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	countOnly, _ := cmd.Flags().GetBool("count")

	entries, err := api.Find(context.Background(), mountName, rootPath, pattern, maxDepth, maxMatches, caseSensitive)
	if err != nil {
		fmt.Printf("搜索失败: %v\n", err)
		os.Exit(1)
	}

	// Convert to findEntries for client-side filtering
	findEntries := make([]findEntry, len(entries))
	for i, e := range entries {
		findEntries[i] = findEntry{
			Path:  e.DecName,
			IsDir: e.IsDir,
			Size:  e.Size,
		}
	}

	// Client-side filtering for advanced matching modes
	globMode, _ := cmd.Flags().GetBool("glob")
	regexMode, _ := cmd.Flags().GetBool("regex")
	exactMode, _ := cmd.Flags().GetBool("exact")
	typeFilter, _ := cmd.Flags().GetString("type")
	sizeFilter, _ := cmd.Flags().GetString("size")

	useFilter := globMode || regexMode || exactMode || typeFilter != "" || sizeFilter != ""

	if useFilter && pattern != "" {
		var filtered []findEntry
		for _, e := range findEntries {
			if matchPath(e.Path, pattern, globMode, regexMode, exactMode, caseSensitive) {
				filtered = append(filtered, e)
			}
		}
		findEntries = filtered
	}

	// File type filter
	if typeFilter != "" {
		var filtered []findEntry
		for _, e := range findEntries {
			if typeFilter == "f" && e.IsDir {
				continue
			}
			if typeFilter == "d" && !e.IsDir {
				continue
			}
			filtered = append(filtered, e)
		}
		findEntries = filtered
	}

	// Size filter
	if sizeFilter != "" {
		sizeOp, sizeBytes := parseSizeFilter(sizeFilter)
		if sizeOp != 0 {
			var filtered []findEntry
			for _, e := range findEntries {
				if e.IsDir {
					filtered = append(filtered, e)
					continue
				}
				switch sizeOp {
				case 1:
					if e.Size > sizeBytes {
						filtered = append(filtered, e)
					}
				case -1:
					if e.Size < sizeBytes {
						filtered = append(filtered, e)
					}
				}
			}
			findEntries = filtered
		}
	}

	if countOnly {
		fmt.Println(len(findEntries))
		return
	}

	for _, e := range findEntries {
		if jsonOutput {
			typ := "file"
			if e.IsDir {
				typ = "dir"
			}
			b, _ := json.Marshal(struct {
				Type string `json:"type"`
				Path string `json:"path"`
			}{Type: typ, Path: e.Path})
			fmt.Println(string(b))
		} else {
			if e.IsDir {
				fmt.Printf("%s/\n", e.Path)
			} else {
				fmt.Println(e.Path)
			}
		}
	}
}

type findEntry struct {
	Path  string
	IsDir bool
	Size  int64
}

func matchPath(path, pattern string, globMode, regexMode, exactMode, caseSensitive bool) bool {
	name := filepath.Base(path)
	test := name
	if !caseSensitive {
		test = strings.ToLower(name)
		pattern = strings.ToLower(pattern)
	}
	switch {
	case globMode:
		matched, _ := filepath.Match(pattern, test)
		return matched
	case regexMode:
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(test)
	case exactMode:
		return test == pattern
	default:
		return strings.Contains(test, pattern)
	}
}

func parseSizeFilter(s string) (op int, bytes int64) {
	if s == "" {
		return 0, 0
	}
	if s[0] == '+' {
		op = 1
		s = s[1:]
	} else if s[0] == '-' {
		op = -1
		s = s[1:]
	}
	parsed, err := parseSizeBytes(s)
	if err != nil {
		return 0, 0
	}
	return op, parsed
}

func parseSizeBytes(s string) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty size")
	}
	s = strings.ToUpper(s)
	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	var val int64
	if _, err := fmt.Sscanf(s, "%d", &val); err != nil {
		return 0, err
	}
	return val * multiplier, nil
}
