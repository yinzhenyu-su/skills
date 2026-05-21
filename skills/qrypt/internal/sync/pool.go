package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

// ScanRemoteForDownload walks the remote directory tree at remoteFid and
// calls submit for each file found. Directories are created locally inline.
func ScanRemoteForDownload(
	ctx context.Context, remoteFid string, localDir string,
	drv drive.Driver, cipher *crypt.RcloneCipher,
	dryRun bool,
	submit func(entry drive.Entry, localPath string),
) error {
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
				if !dryRun {
					os.MkdirAll(targetLocal, 0755)
				} else {
					fmt.Printf("[Dry Run] Would create local directory %s\n", targetLocal)
				}
				if err := walkRemote(e.ID, targetLocal); err != nil {
					return err
				}
			} else {
				submit(e, targetLocal)
			}
		}
		return nil
	}

	return walkRemote(remoteFid, localDir)
}
