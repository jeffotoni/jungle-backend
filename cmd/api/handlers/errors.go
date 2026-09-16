package handlers

import (
	"errors"
	"net/http"

	"github.com/jeffotoni/quick"

	"github.com/jeffotoni/jungle-backend-challenge/cmd/api/models"
	"github.com/jeffotoni/jungle-backend-challenge/internal/application"
	apirepository "github.com/jeffotoni/jungle-backend-challenge/internal/repository/postgres"
)

type httpError struct {
	status        int
	code, message string
}

func (e *httpError) Error() string { return e.message }

func errInvalid() error {
	return &httpError{status: http.StatusBadRequest, code: "INVALID_REQUEST", message: "invalid request"}
}

func writeError(c *quick.Ctx, err error) error {
	if value, ok := err.(*httpError); ok {
		return c.Status(value.status).JSON(models.ErrorResponse{Code: value.code, Message: value.message})
	}
	switch {
	case errors.Is(err, apirepository.ErrInvalidCursor):
		return c.Status(http.StatusBadRequest).JSON(models.ErrorResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid request",
		})
	case errors.Is(err, application.ErrInvalid):
		return c.Status(http.StatusBadRequest).JSON(models.ErrorResponse{Code: "INVALID_REQUEST", Message: "invalid request"})
	case errors.Is(err, application.ErrNotFound):
		return c.Status(http.StatusNotFound).JSON(models.ErrorResponse{Code: "NOT_FOUND", Message: "not found"})
	case errors.Is(err, application.ErrConflict):
		return c.Status(http.StatusConflict).JSON(models.ErrorResponse{Code: "CONFLICT", Message: "conflict"})
	case errors.Is(err, application.ErrUnavailable):
		return c.Status(http.StatusServiceUnavailable).JSON(models.ErrorResponse{
			Code:    "UNAVAILABLE",
			Message: "service unavailable",
		})
	default:
		return c.Status(http.StatusInternalServerError).JSON(models.ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "internal server error",
		})
	}
}
