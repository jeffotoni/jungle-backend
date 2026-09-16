package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	jlog "github.com/jeffotoni/log"
)

func HTTPMiddleware(logger *jlog.Logger, traceKey string) func(http.Handler) http.Handler {
	if strings.TrimSpace(traceKey) == "" {
		traceKey = jlog.DefaultTraceIDKey
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			startedAt := time.Now()
			traceID := request.Header.Get(traceKey)
			if traceID == "" {
				traceID = newTraceID()
			}

			ctx := jlog.WithCtx(request.Context()).
				TraceKey(traceKey).
				TraceID(traceID).
				Context()
			request = request.WithContext(ctx)
			writer.Header().Set(traceKey, traceID)

			response := &responseWriter{ResponseWriter: writer}
			defer func() {
				status := response.status
				if status == 0 {
					status = http.StatusOK
				}

				entry := logger.Info().Ctx(request.Context())
				if status >= http.StatusInternalServerError {
					entry = logger.Error().Ctx(request.Context())
				} else if status >= http.StatusBadRequest {
					entry = logger.Warn().Ctx(request.Context())
				}

				_ = entry.
					Str("method", request.Method).
					Str("path", request.URL.Path).
					Str("remoteIp", remoteIP(request)).
					Int("status", status).
					Int("responseBytes", response.bytes).
					Str("latency", time.Since(startedAt).String()).
					Msg("http request completed").
					Send()
			}()

			next.ServeHTTP(response, request)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	return n, err
}

func remoteIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}

func newTraceID() string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err == nil {
		return hex.EncodeToString(buffer[:])
	}
	return time.Now().UTC().Format("20060102150405.000000000")
}
