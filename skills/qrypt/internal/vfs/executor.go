package vfs

import (
	"context"
	"io"

	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/pkg/cipherdriver"
)

type OpExecutor struct {
	cipherDrv *cipherdriver.CipherDriver
}

// NewOpExecutor creates an OpExecutor backed by the given CipherDriver.
func NewOpExecutor(cipherDrv *cipherdriver.CipherDriver) *OpExecutor {
	return &OpExecutor{cipherDrv: cipherDrv}
}

func (ex *OpExecutor) Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	return ex.cipherDrv.Put(ctx, parentID, name, size, body)
}

func (ex *OpExecutor) Remove(ctx context.Context, entry drivers.Entry) error {
	return ex.cipherDrv.Remove(ctx, entry)
}

func (ex *OpExecutor) BatchRemove(ctx context.Context, entries []drivers.Entry) error {
	return ex.cipherDrv.BatchRemove(ctx, entries)
}

func (ex *OpExecutor) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	return ex.cipherDrv.Rename(ctx, entry, newName)
}

func (ex *OpExecutor) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	return ex.cipherDrv.Mkdir(ctx, parentID, name)
}
