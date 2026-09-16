package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	jlog "github.com/jeffotoni/log"
)

const maxBodyLogBytes = 8 * 1024

func HTTPMiddleware(logger *jlog.Logger, traceKey string, captureDetails bool) func(http.Handler) http.Handler {
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

			var requestBody *requestBodyCapture
			if captureDetails && request.Body != nil {
				requestBody = &requestBodyCapture{ReadCloser: request.Body}
				request.Body = requestBody
			}

			response := &responseWriter{
				ResponseWriter: writer,
				captureBody:    captureDetails,
			}
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
					Str("request.url", safeURL(request.URL)).
					Str("request.host", request.Host).
					Str("request.user_agent", request.UserAgent()).
					Str("remoteIp", remoteIP(request)).
					Int("status", status).
					Str("response.status_text", http.StatusText(status)).
					Int("responseBytes", response.bytes).
					Str("latency", time.Since(startedAt).String()).
					Msg("http request completed")

				if captureDetails {
					entry.Any("request.headers", sanitizeHeaders(request.Header))
					if body := sanitizeBody(requestBodyValue(requestBody)); body != nil {
						entry.Any("request.body", body)
					}
					entry.Any("response.headers", sanitizeHeaders(response.Header()))
					if body := sanitizeBody(responseBodyValue(response)); body != nil {
						entry.Any("response.body", body)
					}
				}
				_ = entry.Send()
			}()

			next.ServeHTTP(response, request)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status           int
	bytes            int
	captureBody      bool
	body             []byte
	bodyWasTruncated bool
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
	if w.captureBody {
		w.capture(body[:n])
	}
	return n, err
}

func (w *responseWriter) capture(body []byte) {
	remaining := maxBodyLogBytes - len(w.body)
	if remaining <= 0 {
		if len(body) > 0 {
			w.bodyWasTruncated = true
		}
		return
	}
	if len(body) > remaining {
		w.body = append(w.body, body[:remaining]...)
		w.bodyWasTruncated = true
		return
	}
	w.body = append(w.body, body...)
}

type requestBodyCapture struct {
	io.ReadCloser
	body             []byte
	total            int
	bodyWasTruncated bool
}

func (r *requestBodyCapture) Read(body []byte) (int, error) {
	n, err := r.ReadCloser.Read(body)
	r.total += n
	remaining := maxBodyLogBytes - len(r.body)
	if remaining <= 0 {
		if n > 0 {
			r.bodyWasTruncated = true
		}
		return n, err
	}
	if n > remaining {
		r.body = append(r.body, body[:remaining]...)
		r.bodyWasTruncated = true
		return n, err
	}
	r.body = append(r.body, body[:n]...)
	return n, err
}

func requestBodyValue(body *requestBodyCapture) ([]byte, bool) {
	if body == nil {
		return nil, false
	}
	return body.body, body.bodyWasTruncated
}

func responseBodyValue(body *responseWriter) ([]byte, bool) {
	return body.body, body.bodyWasTruncated
}

func sanitizeHeaders(headers http.Header) map[string][]string {
	result := make(map[string][]string, len(headers))
	for key, values := range headers {
		if sensitiveField(key) {
			result[key] = []string{"[REDACTED]"}
			continue
		}
		result[key] = make([]string, len(values))
		for index, value := range values {
			result[key][index] = truncateText(value)
		}
	}
	return result
}

func sanitizeBody(body []byte, truncated bool) any {
	if len(body) == 0 {
		return nil
	}
	if truncated {
		return "[TRUNCATED]"
	}

	var value any
	if json.Unmarshal(body, &value) != nil {
		if !utf8.Valid(body) {
			return "[BINARY_BODY]"
		}
		return "[NON_JSON_BODY]"
	}
	if _, ok := value.(map[string]any); !ok {
		if _, ok := value.([]any); !ok {
			return "[NON_OBJECT_BODY]"
		}
	}
	sanitizeJSON(value)
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > maxBodyLogBytes {
		return "[TRUNCATED]"
	}
	return json.RawMessage(encoded)
}

func sanitizeJSON(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if sensitiveField(key) {
				value[key] = "[REDACTED]"
				continue
			}
			sanitizeJSON(item)
		}
	case []any:
		for _, item := range value {
			sanitizeJSON(item)
		}
	}
}

func sensitiveField(value string) bool {
	value = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(value, "-", "_"), " ", "_"))
	compact := strings.ReplaceAll(value, "_", "")
	switch compact {
	case "authorization", "cookie", "setcookie", "password", "secret",
		"clientsecret", "accesstoken", "refreshtoken", "idtoken", "token",
		"apikey":
		return true
	}
	return strings.HasSuffix(compact, "token") || strings.HasSuffix(compact, "secret")
}

func safeURL(value *url.URL) string {
	if value == nil {
		return ""
	}
	copyValue := *value
	query := copyValue.Query()
	for key := range query {
		if sensitiveField(key) {
			query[key] = []string{"[REDACTED]"}
		}
	}
	copyValue.RawQuery = query.Encode()
	return copyValue.String()
}

func truncateText(value string) string {
	if len(value) <= maxBodyLogBytes {
		return value
	}
	return value[:maxBodyLogBytes] + "[TRUNCATED]"
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
