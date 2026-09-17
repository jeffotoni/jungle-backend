package application

import "errors"

type PermanentFailure struct {
	Code  string
	Cause error
}

func (e *PermanentFailure) Error() string {
	if e.Cause == nil {
		return e.Code
	}
	return e.Code + ": " + e.Cause.Error()
}

func (e *PermanentFailure) Unwrap() error { return e.Cause }

func NewPermanentFailure(code string, cause error) error {
	if code == "" {
		code = "PERMANENT_INFRASTRUCTURE_FAILURE"
	}
	return &PermanentFailure{Code: code, Cause: cause}
}

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalid           = errors.New("invalid request")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrPending           = errors.New("pending")
	ErrUnavailable       = errors.New("unavailable")
)
