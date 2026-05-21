package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

const (
	matchSubstring = iota
	matchGlob
	matchRegex
	matchExact
)

func runFind(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

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

	resp, rpcErr := client.Call("find", protocol.FindParams{
		MountName:     mountName,
		Path:          rootPath,
		Pattern:       pattern,
		CaseSensitive: caseSensitive,
		MaxDepth:      maxDepth,
		MaxMatches:    maxMatches,
	})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("搜索失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}

	data, _ := json.Marshal(resp.Result)
	var result protocol.FindResult
	json.Unmarshal(data, &result)

	// Client-side filtering for advanced matching modes
	globMode, _ := cmd.Flags().GetBool("glob")
	regexMode, _ := cmd.Flags().GetBool("regex")
	exactMode, _ := cmd.Flags().GetBool("exact")
	typeFilter, _ := cmd.Flags().GetString("type")
	sizeFilter, _ := cmd.Flags().GetString("size")

	useFilter := globMode || regexMode || exactMode || typeFilter != "" || sizeFilter != ""

	if useFilter && pattern != "" {
		filtered := result.Entries[:0]
		for _, e := range result.Entries {
			if matchPath(e.Path, pattern, globMode, regexMode, exactMode, caseSensitive) {
				filtered = append(filtered, e)
			}
		}
		result.Entries = filtered
		result.Count = len(filtered)
	}

	// File type filter
	if typeFilter != "" {
		filtered := result.Entries[:0]
		for _, e := range result.Entries {
			if typeFilter == "f" && e.IsDir {
				continue
			}
			if typeFilter == "d" && !e.IsDir {
				continue
			}
			filtered = append(filtered, e)
		}
		result.Entries = filtered
		result.Count = len(filtered)
	}

	// Size filter
	if sizeFilter != "" {
		sizeOp, sizeBytes := parseSizeFilter(sizeFilter)
		if sizeOp != 0 {
			filtered := result.Entries[:0]
			for _, e := range result.Entries {
				if e.IsDir {
					filtered = append(filtered, e)
					continue
				}
				switch sizeOp {
				case 1: // greater than
					if e.Size > sizeBytes {
						filtered = append(filtered, e)
					}
				case -1: // less than
					if e.Size < sizeBytes {
						filtered = append(filtered, e)
					}
				}
			}
			result.Entries = filtered
			result.Count = len(filtered)
		}
	}

	// Max matches post-filter
	if maxMatches > 0 && len(result.Entries) > maxMatches {
		result.Entries = result.Entries[:maxMatches]
		result.Count = maxMatches
	}

	if countOnly {
		fmt.Println(result.Count)
		return
	}

	for _, e := range result.Entries {
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
