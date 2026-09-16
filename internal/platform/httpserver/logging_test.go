package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jlog "github.com/jeffotoni/log"
)

func TestHTTPMiddlewareCapturesSanitizedDetails(t *testing.T) {
	var output bytes.Buffer
	logger := jlog.New(jlog.Config{
		Format:      jlog.FormatJSON,
		Writer:      &output,
		Level:       jlog.DEBUG,
		ServiceName: "api",
	})
	handler := HTTPMiddleware(logger, "traceId", true)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.ReadAll(request.Body)
		_, _ = writer.Write([]byte(`{"result":"ok"}`))
	}))
	request := httptest.NewRequest(
		http.MethodPost,
		"/wallets?access_token=secret&limit=50",
		strings.NewReader(`{"name":"player-001","password":"secret","nested":{"clientSecret":"secret"}}`),
	)
	request.Host = "api.local:8080"
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("User-Agent", "test-client/1.0")
	request.Header.Set("traceId", "trace-123")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["traceId"] != "trace-123" ||
		entry["request.url"] != "/wallets?access_token=%5BREDACTED%5D&limit=50" ||
		entry["request.host"] != "api.local:8080" ||
		entry["request.user_agent"] != "test-client/1.0" ||
		entry["response.status_text"] != "OK" {
		t.Fatalf("unexpected request fields: %+v", entry)
	}
	if !strings.Contains(output.String(), `"request.headers"`) ||
		!strings.Contains(output.String(), `"request.body"`) ||
		!strings.Contains(output.String(), `"response.headers"`) ||
		!strings.Contains(output.String(), `"response.body"`) {
		t.Fatalf("detailed fields missing: %s", output.String())
	}
	if strings.Contains(output.String(), "Bearer secret") ||
		strings.Contains(output.String(), `"password":"secret"`) ||
		strings.Contains(output.String(), `"clientSecret":"secret"`) {
		t.Fatalf("sensitive data leaked: %s", output.String())
	}
}

func TestHTTPMiddlewareOmitsDetailsWhenDisabled(t *testing.T) {
	var output bytes.Buffer
	logger := jlog.New(jlog.Config{
		Format:      jlog.FormatJSON,
		Writer:      &output,
		Level:       jlog.INFO,
		ServiceName: "api",
	})
	handler := HTTPMiddleware(logger, "traceId", false)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"result":"ok"}`))
	}))
	request := httptest.NewRequest(
		http.MethodPost,
		"/wallets",
		strings.NewReader(`{"password":"secret"}`),
	)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if strings.Contains(output.String(), `"request.headers"`) ||
		strings.Contains(output.String(), `"request.body"`) ||
		strings.Contains(output.String(), `"response.headers"`) ||
		strings.Contains(output.String(), `"response.body"`) {
		t.Fatalf("unexpected detailed fields: %s", output.String())
	}
}
