package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func writeTestConfig(t *testing.T, workDir string) string {
	t.Helper()
	cfgPath := filepath.Join(workDir, "qrypt.toml")
	content := `version = "1"

[[mounts]]
name = "test"
type = "localfs"
mount_point = "` + workDir + `"

[mounts.params]
local_root = "` + workDir + `"

[mounts.encryption]
password = "test"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func baseFlags(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("mount", "", "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("salt", "", "")
	cmd.Flags().Set("config", cfgPath)
	return cmd
}

func TestCLI_E2E_ListDir(t *testing.T) {
	workDir := t.TempDir()
	t.Setenv("QRYPT_WORK_DIR", workDir)
	os.WriteFile(filepath.Join(workDir, "hello.txt"), []byte("world"), 0644)
	cfgPath := writeTestConfig(t, workDir)

	cmd := baseFlags(cfgPath)
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
	cfgPath := writeTestConfig(t, workDir)

	mkdirCmd := baseFlags(cfgPath)
	mkdirCmd.Flags().Bool("parents", false, "")
	runMkdir(mkdirCmd, []string{"/subdir"})

	listCmd := baseFlags(cfgPath)
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
	cfgPath := writeTestConfig(t, workDir)

	cmd := baseFlags(cfgPath)
	cmd.Flags().Bool("recursive", false, "")
	cmd.Flags().Bool("recursive-upper", false, "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("interactive", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	runRm(cmd, []string{"/temp.txt"})
}


