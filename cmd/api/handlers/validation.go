package handlers

import (
	"strconv"
	"strings"
)

func parseLimit(value string) int {
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func validID(value string) bool { return strings.TrimSpace(value) != "" }
