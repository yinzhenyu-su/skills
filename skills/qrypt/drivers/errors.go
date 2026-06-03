package backend

import "errors"

var (
	ErrNotImplemented  = errors.New("operation not implemented by this driver")
	ErrNotFound        = errors.New("entry not found")
	ErrNotDir          = errors.New("not a directory")
	ErrAlreadyExists   = errors.New("entry already exists")
	ErrDirAlreadyExists = errors.New("directory with this name already exists")
)
