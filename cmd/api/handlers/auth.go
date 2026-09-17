package handlers

import (
	"errors"
	"net/http"

	"github.com/jeffotoni/quick"

	apiauth "github.com/jeffotoni/jungle-backend/cmd/api/auth"
)

func (r *Routes) requireInternal(c *quick.Ctx) error {
	principal, err := r.auth.Authenticate(c.Request)
	if err != nil {
		return authError(err)
	}
	if !principal.Internal {
		return authError(apiauth.ErrForbidden)
	}
	return nil
}

func (r *Routes) requireProvider(c *quick.Ctx, providerID string) error {
	principal, err := r.auth.Authenticate(c.Request)
	if err != nil {
		return authError(err)
	}
	if principal.ProviderID == "" || principal.ProviderID != providerID {
		return authError(apiauth.ErrForbidden)
	}
	return nil
}

func authError(err error) error {
	switch {
	case errors.Is(err, apiauth.ErrAuthUnavailable):
		return &httpError{
			status:  http.StatusServiceUnavailable,
			code:    "AUTH_UNAVAILABLE",
			message: "authentication unavailable",
		}
	case errors.Is(err, apiauth.ErrForbidden):
		return &httpError{status: http.StatusForbidden, code: "FORBIDDEN", message: "forbidden"}
	default:
		return &httpError{status: http.StatusUnauthorized, code: "UNAUTHORIZED", message: "unauthorized"}
	}
}
