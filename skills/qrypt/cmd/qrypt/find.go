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
)

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
		if opts.caseSensitive {
			m.re = regexp.MustCompile(pattern)
		} else {
			m.re = regexp.MustCompile("(?i)" + pattern)
		}
	}
	return m
}

func (m *nodeMatcher) matchName(name string) bool {
	if m.pattern == "" {
		return true
	}
	test := name
	if !m.caseSensitive && m.mode != matchRegex {
		test = strings.ToLower(test)
	}
	p := m.pattern
	if !m.caseSensitive && m.mode != matchRegex {
		p = strings.ToLower(p)
	}
	switch m.mode {
	case matchGlob:
		ok, _ := path.Match(p, test)
		return ok
	case matchRegex:
		return m.re.MatchString(name)
	case matchExact:
		return test == p
	default:
		return strings.Contains(test, p)
	}
}

func (m *nodeMatcher) matchEntry(e drive.Entry) bool {
	switch m.fileType {
	case 1:
		if e.IsDir {
			return false
		}
	case 2:
		if !e.IsDir {
			return false
		}
	}
	if m.sizeOp != 0 && !e.IsDir {
		s := e.Size
		switch m.sizeOp {
		case 1:
			if s <= m.sizeBytes {
				return false
			}
		case -1:
			if s >= m.sizeBytes {
				return false
			}
		case 0:
			if s != m.sizeBytes {
				return false
			}
		}
	}
	return true
}

type fileLister interface {
	List(parentID string) ([]drive.Entry, error)
}

type cipherHelper interface {
	DecryptSegment(string) (string, error)
	EncryptSegment(string) string
}

type finder struct {
	lister  fileLister
	cipher  cipherHelper
	opts    *findOptions
	matcher *nodeMatcher

	stopped atomic.Bool
	count   atomic.Int32
	wg      sync.WaitGroup
	sem     chan struct{}
	outMu   sync.Mutex
}

func newFinder(lister fileLister, cipher cipherHelper, opts *findOptions) *finder {
	workers := opts.workers
	if workers <= 0 {
		workers = 1
	}
	if workers > 8 {
		workers = 8
	}
	return &finder{
		lister:  lister,
		cipher:  cipher,
		opts:    opts,
		matcher: newMatcher(opts.pattern, opts),
		sem:     make(chan struct{}, workers),
	}
}

func (f *finder) run(rootFid, rootPath string) int {
	f.sem <- struct{}{}
	f.wg.Add(1)
	go func() {
		defer func() { <-f.sem; f.wg.Done() }()
		f.visitDir(rootFid, rootPath, 0)
	}()
	f.wg.Wait()
	if !f.opts.countOnly {
		n := f.count.Load()
		if n == 0 {
			fmt.Println("未找到匹配的文件")
		}
	}
	return int(f.count.Load())
}

func (f *finder) visitDir(fid, displayPath string, depth int) {
	if f.stopped.Load() {
		return
	}
	entries, err := f.lister.List(fid)
	if err != nil {
		return
	}
	for _, e := range entries {
		if f.stopped.Load() {
			return
		}
		decName, decErr := f.cipher.DecryptSegment(e.Name)
		if decErr != nil {
			decName = e.Name
		}
		childPath := filepath.Join(displayPath, decName)

		if f.matcher.matchName(decName) && f.matcher.matchEntry(e) {
			f.count.Add(1)
			if !f.opts.countOnly {
				f.output(childPath, e.IsDir)
			}
			if f.opts.maxMatches > 0 && f.count.Load() >= int32(f.opts.maxMatches) {
				f.stopped.Store(true)
				return
			}
		}
		if e.IsDir && (f.opts.maxDepth < 0 || depth < f.opts.maxDepth) {
			select {
			case f.sem <- struct{}{}:
				f.wg.Add(1)
				go func(cfid, cpath string, cdepth int) {
					defer func() { <-f.sem; f.wg.Done() }()
					f.visitDir(cfid, cpath, cdepth)
				}(e.ID, childPath, depth+1)
			default:
				f.visitDir(e.ID, childPath, depth+1)
			}
		}
	}
}

func (f *finder) output(childPath string, isDir bool) {
	f.outMu.Lock()
	defer f.outMu.Unlock()
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
	cfg, cipher := loadToolCfg(cmd)
	drv := loadToolDriver(cfg, cipher)

	resolver, ok := drv.(pathResolver)
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

	fullRootPath := resolveFullPath(cfg.RootPath(), rootPath)
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

	f := newFinder(listAdapter{drv: drv}, cipher, opts)
	matched := f.run(rootFid, fullRootPath)

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
