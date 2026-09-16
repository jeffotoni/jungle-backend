package application

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalid           = errors.New("invalid request")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrPending           = errors.New("pending")
	ErrUnavailable       = errors.New("unavailable")
)
