package handlers

import (
	"net/http"

	"github.com/jeffotoni/quick"
)

func (r *Routes) live(c *quick.Ctx) error {
	return c.Status(http.StatusOK).JSON(map[string]string{"status": "ok"})
}
