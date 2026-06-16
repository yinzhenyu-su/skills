package vfs

type UploadObserver interface {
	OnUploadComplete(path string, newFid string, encSize int64, err error, attempt *uploadAttempt)
}
