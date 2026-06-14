package vfs

type UploadObserver interface {
	OnUploadComplete(path string, newFid string, err error)
}
