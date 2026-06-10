package integration

import (
	"testing"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

// skipIfNotWriter skips the test if the driver doesn't implement Writer.
func skipIfNotWriter(t *testing.T, drv drivers.Driver) drivers.Writer {
	t.Helper()
	w, ok := drv.(drivers.Writer)
	if !ok {
		t.Skip("driver does not implement Writer")
	}
	return w
}

// skipIfNotUploader skips the test if the driver doesn't implement Uploader.
func skipIfNotUploader(t *testing.T, drv drivers.Driver) drivers.Uploader {
	t.Helper()
	up, ok := drv.(drivers.Uploader)
	if !ok {
		t.Skip("driver does not implement Uploader")
	}
	return up
}
