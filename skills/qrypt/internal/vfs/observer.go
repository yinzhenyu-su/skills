package vfs

// UploadObserver receives notifications about upload lifecycle events.
type UploadObserver interface {
	// OnUploadComplete is called when an upload finishes, successfully or not.
	// path is the plaintext path that was uploaded.
	// err is nil on success, non-nil on failure.
	OnUploadComplete(path string, err error)
}
