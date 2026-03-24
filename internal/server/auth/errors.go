package auth

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrAlreadyExists   = errors.New("already exists")
	ErrUnauthorized    = errors.New("unauthorized")
)
