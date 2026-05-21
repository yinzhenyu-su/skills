package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	factory "github.com/yinzhenyu/skills/qrypt/internal/drive/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/drive/localfs"
	quark "github.com/yinzhenyu/skills/qrypt/internal/drive/quark"
)

type cipherHelper interface {
	DecryptSegment(string) (string, error)
	EncryptSegment(string) string
}

const (
	matchSubstring = iota
	matchGlob
	matchRegex
	matchExact
)

type findOptions struct {
	pattern       string
	matchMode     int
	caseSensitive bool
	fileType      int
	maxDepth      int
	maxMatches    int
	jsonOutput    bool
	countOnly     bool
	workers       int
	sizeOp        int
	sizeBytes     int64
}

type nodeMatcher struct {
	pattern       string
	re            *regexp.Regexp
	mode          int
	caseSensitive bool
	fileType      int
	sizeOp        int
	sizeBytes     int64
}

func newMatcher(pattern string, opts *findOptions) *nodeMatcher {
	m := &nodeMatcher{
		pattern:       pattern,
		mode:          opts.matchMode,
		caseSensitive: opts.caseSensitive,
		fileType:      opts.fileType,
		sizeOp:        opts.sizeOp,
		sizeBytes:     opts.sizeBytes,
	}
	if opts.matchMode == matchRegex && pattern != "" {
		var expr string
		if opts.caseSensitive {
			expr = pattern
		} else {
			expr = "(?i)" + pattern
		}
		m.re = regexp.MustCompile(expr)
	}
	return m
}

func (m *nodeMatcher) matchName(name string) bool {
	if m.pattern == "" {
		return true
	}
	test := name
	if !m.caseSensitive && m.mode != matchRegex {
		test = strings.ToLower(name)
	}
	switch m.mode {
	case matchGlob:
		return matchGlobPattern(m.pattern, name, m.caseSensitive)
	case matchRegex:
		return m.re.MatchString(test)
	case matchExact:
		if m.caseSensitive {
			return test == m.pattern
		}
		return strings.EqualFold(test, m.pattern)
	default:
		return strings.Contains(test, m.pattern)
	}
}

func (m *nodeMatcher) matchEntry(entry drive.Entry) bool {
	if m.fileType != 0 {
		isDir := entry.IsDir
		if m.fileType == 1 && isDir {
			return false
		}
		if m.fileType == 2 && !isDir {
			return false
		}
	}

	if m.sizeOp != 0 && !entry.IsDir {
		sz := entry.Size
		switch m.sizeOp {
		case 1:
			if sz <= m.sizeBytes {
				return false
			}
		case -1:
			if sz >= m.sizeBytes {
				return false
			}
		}
	}

	return m.matchName(entry.Name)
}

func matchGlobPattern(pattern, name string, caseSensitive bool) bool {
	if !caseSensitive {
		pattern = strings.ToLower(pattern)
		name = strings.ToLower(name)
	}
	matched, _ := path.Match(pattern, name)
	return matched
}

type lister interface {
	List(parentID string) ([]drive.Entry, error)
}

type finder struct {
	lister  lister
	cipher  cipherHelper
	opts    *findOptions
	matches atomic.Int32
	sem     chan struct{}
}

func newFinder(l lister, c cipherHelper, opts *findOptions) *finder {
	if opts.workers <= 0 {
		opts.workers = 1
	} else if opts.workers > 8 {
		opts.workers = 8
	}
	return &finder{
		lister: l,
		cipher: c,
		opts:   opts,
		sem:    make(chan struct{}, opts.workers),
	}
}

func (f *finder) run(fid string, displayPath string) int {
	f.matches.Store(0)
	f.scan(fid, displayPath, 0)
	return int(f.matches.Load())
}

func (f *finder) scan(fid string, displayPath string, depth int) {
	if f.opts.maxDepth >= 0 && depth > f.opts.maxDepth {
		return
	}

	entries, err := f.lister.List(fid)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	for _, e := range entries {
		childPath := filepath.Join(displayPath, e.Name)
		isDir := e.IsDir

		matcher := newMatcher(f.opts.pattern, f.opts)
		if matcher.matchEntry(e) {
			if f.opts.maxMatches > 0 && f.matches.Load() >= int32(f.opts.maxMatches) {
				return
			}
			f.printResult(childPath, isDir)
			f.matches.Add(1)
		}

		if isDir {
			if f.opts.maxDepth >= 0 && depth >= f.opts.maxDepth {
				continue
			}
			wg.Add(1)
			f.sem <- struct{}{}
			go func(childFid string, childDisplay string) {
				defer func() { <-f.sem }()
				defer wg.Done()
				f.scan(childFid, childDisplay, depth+1)
			}(e.ID, childPath)
		}
	}
	wg.Wait()
}

func (f *finder) printResult(childPath string, isDir bool) {
	if f.opts.countOnly {
		return
	}
	if f.opts.jsonOutput {
		typ := "file"
		if isDir {
			typ = "dir"
		}
		b, _ := json.Marshal(struct {
			Type string `json:"type"`
			Path string `json:"path"`
		}{Type: typ, Path: childPath})
		fmt.Println(string(b))
	} else {
		if isDir {
			fmt.Printf("%s/\n", childPath)
		} else {
			fmt.Println(childPath)
		}
	}
}

func runFind(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	_, cfg, _, _ := config.LoadConfigAuto(configPath)
	if cfg == nil {
		fmt.Println("未找到配置文件，请使用 --config 指定")
		os.Exit(1)
	}

	path := ""
	if len(args) >= 2 {
		path = args[0]
	}
	mountName := resolveMount(cmd, &path)

	m := config.FindMount(cfg, mountName)
	if m == nil {
		if mountName != "" {
			fmt.Printf("挂载实例 %q 未找到\n", mountName)
		} else {
			fmt.Printf("未指定挂载实例 (配置中有 %d 个，使用 --mount 或 mount_name:path 选择)\n", len(cfg.Mounts))
		}
		os.Exit(1)
	}

	rc := cfg.MergeInstanceConfig(*m)

	pwd, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	mountCipher, cerr := config.MakeCipher(rc.Encryption, cfg.Defaults.Encryption, pwd, salt)
	if cerr != nil {
		fmt.Printf("加密引擎初始化失败: %v\n", cerr)
		os.Exit(1)
	}

	drv, err := factory.NewDriverFromType(rc.Type, rc.Params)
	if err != nil {
		fmt.Printf("创建驱动失败: %v\n", err)
		os.Exit(1)
	}
	if err := drv.Init(context.Background()); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}
	switch d := drv.(type) {
	case *quark.QuarkDriver:
		if mountCipher != nil {
			d.SetCipher(mountCipher)
		}
	case *localfs.LocalDriver:
		if mountCipher != nil {
			d.SetCipher(mountCipher)
		}
	}

	resolver, ok := drv.(drive.PathResolver)
	if !ok {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}

	globMode, _ := cmd.Flags().GetBool("glob")
	regexMode, _ := cmd.Flags().GetBool("regex")
	exactMode, _ := cmd.Flags().GetBool("exact")
	caseSensitive, _ := cmd.Flags().GetBool("case-sensitive")
	typeFilter, _ := cmd.Flags().GetString("type")
	maxDepth, _ := cmd.Flags().GetInt("maxdepth")
	maxMatches, _ := cmd.Flags().GetInt("max")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	countOnly, _ := cmd.Flags().GetBool("count")
	workers, _ := cmd.Flags().GetInt("workers")
	sizeFilter, _ := cmd.Flags().GetString("size")

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

	fullRootPath := config.ResolveFullPath(config.RootPathForMount(*m), rootPath)
	rootFid, err := resolver.ResolvePath(context.Background(), fullRootPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	matchMode := matchSubstring
	if globMode {
		matchMode = matchGlob
	}
	if regexMode {
		matchMode = matchRegex
	}
	if exactMode {
		matchMode = matchExact
	}

	ft := 0
	switch typeFilter {
	case "f":
		ft = 1
	case "d":
		ft = 2
	}

	sizeOp := 0
	sizeBytes := int64(0)
	if sizeFilter != "" {
		if sizeFilter[0] == '+' {
			sizeOp = 1
			sizeFilter = sizeFilter[1:]
		} else if sizeFilter[0] == '-' {
			sizeOp = -1
			sizeFilter = sizeFilter[1:]
		}
		parsed, err := config.ParseSize(sizeFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "无效的大小格式: %s\n", sizeFilter)
			os.Exit(1)
		}
		sizeBytes = parsed
	}

	opts := &findOptions{
		pattern:       pattern,
		matchMode:     matchMode,
		caseSensitive: caseSensitive,
		fileType:      ft,
		maxDepth:      maxDepth,
		maxMatches:    maxMatches,
		jsonOutput:    jsonOutput,
		countOnly:     countOnly,
		workers:       workers,
		sizeOp:        sizeOp,
		sizeBytes:     sizeBytes,
	}

	f := newFinder(listAdapter{drv: drv}, mountCipher, opts)
	matched := f.run(rootFid, rootPath)

	if opts.countOnly {
		fmt.Println(matched)
	}
}

type listAdapter struct {
	drv drive.Reader
}

func (a listAdapter) List(parentID string) ([]drive.Entry, error) {
	return a.drv.List(context.Background(), parentID)
}
