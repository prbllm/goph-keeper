package vault

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("item not found")
	ErrConflict        = errors.New("version conflict")
)

