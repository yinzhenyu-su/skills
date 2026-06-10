//go:build integration

package integration

import (
	"testing"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

// runSuite runs all integration test suites against a driver.
// Each suite is a logical group of related test scenarios.
func runSuite(t *testing.T, drv drivers.Driver) {
	t.Helper()

	// File operation scenarios
	t.Run("FileCreateWriteRead", func(t *testing.T) {
		testFileCreateWriteRead(t, drv)
	})
	t.Run("MkdirAndList", func(t *testing.T) {
		testMkdirAndList(t, drv)
	})
	t.Run("RenameFile", func(t *testing.T) {
		testRenameFile(t, drv)
	})
	t.Run("RenameCrossDir", func(t *testing.T) {
		testRenameCrossDir(t, drv)
	})
	t.Run("DeleteFile", func(t *testing.T) {
		testDeleteFile(t, drv)
	})
	t.Run("DeleteDir", func(t *testing.T) {
		testDeleteDir(t, drv)
	})
	t.Run("OverwriteFile", func(t *testing.T) {
		testOverwriteFile(t, drv)
	})
}


