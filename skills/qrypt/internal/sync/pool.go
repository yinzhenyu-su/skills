package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type JobType int

const (
	JobTypeUpload JobType = iota
	JobTypeDownload
)

type TransferJob struct {
	Type          JobType
	LocalPath     string
	RemoteName    string
	RemoteParent  string // Parent FID
	RemoteEntry   drive.Entry
	Size          int64
	IsDir         bool
}

type WorkerPool struct {
	concurrency int
	drv         drive.Driver
	cipher      *crypt.RcloneCipher
	uploader    *Uploader
	downloader  *Downloader
	jobs        chan TransferJob
	wg          sync.WaitGroup
	dryRun      bool
	update      bool
}

func NewWorkerPool(drv drive.Driver, cipher *crypt.RcloneCipher, concurrency int, dryRun, update bool) *WorkerPool {
	return &WorkerPool{
		concurrency: concurrency,
		drv:         drv,
		cipher:      cipher,
		uploader:    NewUploader(drv, cipher),
		downloader:  NewDownloader(drv, cipher),
		jobs:        make(chan TransferJob, concurrency*2),
		dryRun:      dryRun,
		update:      update,
	}
}

func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

func (p *WorkerPool) Submit(job TransferJob) {
	p.jobs <- job
}

func (p *WorkerPool) Wait() {
	close(p.jobs)
	p.wg.Wait()
}

func (p *WorkerPool) worker(ctx context.Context, id int) {
	defer p.wg.Done()
	for job := range p.jobs {
		if job.IsDir {
			continue // Skip directories for actual transfer, Mkdir is handled by scanners
		}

		if p.dryRun {
			if job.Type == JobTypeUpload {
				fmt.Printf("[Dry Run] Would upload %s -> %s\n", job.LocalPath, job.RemoteName)
			} else {
				fmt.Printf("[Dry Run] Would download %s -> %s\n", job.RemoteName, job.LocalPath)
			}
			continue
		}

		if job.Type == JobTypeUpload {
			p.handleUpload(ctx, job)
		} else {
			p.handleDownload(ctx, job)
		}
	}
}

func (p *WorkerPool) handleUpload(ctx context.Context, job TransferJob) {
	req := Request{
		Name:      job.RemoteName,
		ParentFid: job.RemoteParent,
		PlainSize: job.Size,
		DataReader: func() (io.ReadCloser, error) {
			return os.Open(job.LocalPath)
		},
		ProgressFn: func(partNumber int) {}, // could add progress tracker
	}

	fmt.Printf("上传: %s\n", job.LocalPath)
	_, err := p.uploader.Upload(req)
	if err != nil {
		fmt.Printf("上传失败 %s: %v\n", job.LocalPath, err)
	}
}

func (p *WorkerPool) handleDownload(ctx context.Context, job TransferJob) {
	if p.update {
		if stat, err := os.Stat(job.LocalPath); err == nil {
			plainSize, _ := p.cipher.DecryptedSize(job.RemoteEntry.Size)
			if stat.Size() == plainSize {
				fmt.Printf("跳过: %s (大小一致)\n", job.RemoteName)
				return
			}
		}
	}

	req := DownloadRequest{
		Entry:     job.RemoteEntry,
		LocalPath: job.LocalPath,
		ProgressFn: func(written, total int64) {},
	}

	fmt.Printf("下载: %s -> %s\n", job.RemoteName, job.LocalPath)
	if err := p.downloader.Download(ctx, req); err != nil {
		fmt.Printf("下载失败 %s: %v\n", job.RemoteName, err)
	}
}

// Scanners

type RemoteDirCreator interface {
	Mkdir(ctx context.Context, parentID, name string) (drive.Entry, error)
}

func ScanLocalForUpload(ctx context.Context, localDir string, remoteParentFid string, drv drive.Driver, cipher *crypt.RcloneCipher, pool *WorkerPool) error {
	w, hasMkdir := drv.(drive.Writer)

	// Pre-build directory mapping: relative path -> remote parent FID
	dirFids := make(map[string]string)
	dirFids["."] = remoteParentFid

	// Cache remote entries for the current directory to support --update checks
	remoteCache := make(map[string][]drive.Entry)

	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(localDir, path)
		if relPath == "." {
			return nil
		}

		parentRel := filepath.Dir(relPath)
		parentFid := dirFids[parentRel]

		// Ensure we have listed the parent directory for updates/exists checks
		if _, ok := remoteCache[parentFid]; !ok {
			entries, _ := drv.List(ctx, parentFid)
			remoteCache[parentFid] = entries
		}

		if info.IsDir() {
			encName := cipher.EncryptSegment(info.Name())
			var currentFid string

			// Try to find if exists in cache
			found := false
			for _, e := range remoteCache[parentFid] {
				if e.IsDir && e.Name == encName {
					currentFid = e.ID
					found = true
					break
				}
			}

			if !found && hasMkdir && !pool.dryRun {
				newEntry, err := w.Mkdir(ctx, parentFid, encName)
				if err == nil {
					currentFid = newEntry.ID
				} else {
					fmt.Printf("创建远端目录失败: %v\n", err)
				}
			} else if pool.dryRun && !found {
				fmt.Printf("[Dry Run] Would create remote directory %s\n", encName)
				currentFid = "mock-" + encName
			}
			dirFids[relPath] = currentFid
			return nil
		}

		// It's a file, do the update check
		if pool.update {
			encName := cipher.EncryptSegment(info.Name())
			shouldSkip := false
			for _, e := range remoteCache[parentFid] {
				if !e.IsDir && e.Name == encName {
					plainSize, err := cipher.DecryptedSize(e.Size)
					if err == nil && plainSize == info.Size() {
						shouldSkip = true
						fmt.Printf("跳过: %s (大小一致)\n", path)
						break
					}
				}
			}
			if shouldSkip {
				return nil
			}
		}

		pool.Submit(TransferJob{
			Type:         JobTypeUpload,
			LocalPath:    path,
			RemoteName:   info.Name(),
			RemoteParent: parentFid,
			Size:         info.Size(),
		})

		return nil
	})
}

func ScanRemoteForDownload(ctx context.Context, remoteFid string, localDir string, drv drive.Driver, cipher *crypt.RcloneCipher, pool *WorkerPool) error {
	var walkRemote func(fid string, currentLocal string) error
	walkRemote = func(fid string, currentLocal string) error {
		entries, err := drv.List(ctx, fid)
		if err != nil {
			return err
		}

		for _, e := range entries {
			decName, decErr := cipher.DecryptSegment(e.Name)
			if decErr != nil {
				decName = e.Name
			}

			targetLocal := filepath.Join(currentLocal, decName)

			if e.IsDir {
				if !pool.dryRun {
					os.MkdirAll(targetLocal, 0755)
				} else {
					fmt.Printf("[Dry Run] Would create local directory %s\n", targetLocal)
				}
				if err := walkRemote(e.ID, targetLocal); err != nil {
					return err
				}
			} else {
				// File
				pool.Submit(TransferJob{
					Type:        JobTypeDownload,
					LocalPath:   targetLocal,
					RemoteName:  decName,
					RemoteEntry: e,
					Size:        e.Size,
				})
			}
		}
		return nil
	}

	return walkRemote(remoteFid, localDir)
}
