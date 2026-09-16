package env

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), v != ""
}

func GetString(name, def string) string {
	if s, ok := lookup(name); ok {
		return s
	}
	return def
}

func GetInt(name string, def int) int {
	if s, ok := lookup(name); ok {
		if n, err := strconv.ParseInt(s, 10, 0); err == nil {
			return int(n)
		}
	}
	return def
}

func GetInt64(name string, def int64) int64 {
	if s, ok := lookup(name); ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func GetDuration(name string, def time.Duration) time.Duration {
	if s, ok := lookup(name); ok {
		if strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
			if d, err := time.ParseDuration(s); err == nil {
				return d
			}
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Duration(n) * time.Millisecond
		}
	}
	return def
}

func GetBool(name string, def bool) bool {
	if s, ok := lookup(name); ok {
		if v, err := strconv.ParseBool(s); err == nil {
			return v
		}
	}
	return def
}
