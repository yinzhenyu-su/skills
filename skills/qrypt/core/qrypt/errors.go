package qrypt

import "fmt"

type ErrorKind int

const (
	ErrNotFound      ErrorKind = iota
	ErrAlreadyExists
	ErrNotEmpty
	ErrPermission
	ErrNetwork
	ErrInternal
)

type Error struct {
	Kind    ErrorKind
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Kind == t.Kind
}

func NewError(kind ErrorKind, msg string) *Error {
	return &Error{Kind: kind, Message: msg}
}

func NewErrorf(kind ErrorKind, format string, args ...interface{}) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

func WrapError(kind ErrorKind, msg string, cause error) *Error {
	return &Error{Kind: kind, Message: msg, Cause: cause}
}
