package handlers

import (
	"net/http"

	"github.com/jeffotoni/quick"
)

func (r *Routes) ready(c *quick.Ctx) error {
	if err := r.pool.Ping(c.Ctx()); err != nil {
		return c.Status(http.StatusServiceUnavailable).JSON(map[string]string{"status": "not_ready"})
	}
	return c.Status(http.StatusOK).JSON(map[string]string{"status": "ready"})
}
