package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
)

func startTestDaemon(t *testing.T, workDir string) {
	t.Helper()

	cfg := &config.Config{
		Mounts: []config.MountInstance{{
			Name:   "test",
			Type:   "localfs",
			Params: config.MountParams{LocalRoot: workDir},
			Encryption: &config.EncryptionConfig{
				Password: "test",
			},
		}},
	}

	d := daemon.NewDaemon(cfg, "test")
	srv := daemon.NewWSServer(d, daemon.FindSocketPath())
	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() { srv.Stop(); os.Remove(daemon.FindSocketPath()) })
}

func baseFlags() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("mount", "", "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("salt", "", "")
	return cmd
}

func TestCLI_E2E_ListDir(t *testing.T) {
	workDir := t.TempDir()
	t.Setenv("QRYPT_WORK_DIR", workDir)
	os.WriteFile(filepath.Join(workDir, "hello.txt"), []byte("world"), 0644)
	startTestDaemon(t, workDir)

	cmd := baseFlags()
	cmd.Flags().Bool("long", false, "")
	cmd.Flags().Bool("encrypted", false, "")
	cmd.Flags().Bool("human-readable", false, "")
	cmd.Flags().Bool("sort-time", false, "")
	cmd.Flags().Bool("sort-size", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("recursive", false, "")
	cmd.Flags().Bool("help", false, "")

	runList(cmd, []string{"/"})
}

func TestCLI_E2E_MkdirAndList(t *testing.T) {
	workDir := t.TempDir()
	t.Setenv("QRYPT_WORK_DIR", workDir)
	startTestDaemon(t, workDir)

	mkdirCmd := baseFlags()
	mkdirCmd.Flags().Bool("parents", false, "")
	runMkdir(mkdirCmd, []string{"/subdir"})

	listCmd := baseFlags()
	listCmd.Flags().Bool("json", false, "")
	listCmd.Flags().Bool("long", false, "")
	listCmd.Flags().Bool("encrypted", false, "")
	listCmd.Flags().Bool("human-readable", false, "")
	listCmd.Flags().Bool("sort-time", false, "")
	listCmd.Flags().Bool("sort-size", false, "")
	listCmd.Flags().Bool("recursive", false, "")
	listCmd.Flags().Bool("help", false, "")
	runList(listCmd, []string{"/"})
}

func TestCLI_E2E_Remove(t *testing.T) {
	workDir := t.TempDir()
	t.Setenv("QRYPT_WORK_DIR", workDir)
	os.WriteFile(filepath.Join(workDir, "temp.txt"), []byte("data"), 0644)
	startTestDaemon(t, workDir)

	cmd := baseFlags()
	cmd.Flags().Bool("recursive", false, "")
	cmd.Flags().Bool("recursive-upper", false, "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("interactive", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	runRm(cmd, []string{"/temp.txt"})
}


