package handlers

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apiauth "github.com/jeffotoni/jungle-backend/cmd/api/auth"
)

func TestLiveEndpoint(t *testing.T) {
	verifier := apiauth.NewVerifier("issuer", "audience", "wallet-internal")
	server := NewRouter(nil, nil, verifier, nil, nil, "", time.Second)
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusOK)
	}
}

func TestWagerEndpointRequiresAuthentication(t *testing.T) {
	verifier := apiauth.NewVerifier("issuer", "audience", "wallet-internal")
	server := NewRouter(nil, nil, verifier, nil, nil, "", time.Second)
	body := strings.NewReader(`{
		"providerId":"provider-a",
		"externalTransactionId":"external-1",
		"walletId":"516be6a5-8338-4560-a723-0fc1e6e6e801",
		"playerId":"player-001",
		"roundId":"round-001",
		"gameId":"game-001",
		"kind":"BET",
		"money":{"amount":"10.00","currency":"BRL"}
	}`)
	request := httptest.NewRequest(http.MethodPost, "/wagering/transactions", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestProviderEndpointRejectsAnotherProvider(t *testing.T) {
	verifier, token := newOIDCTestVerifier(t, "provider-a", nil)
	server := NewRouter(nil, nil, verifier, nil, nil, "", time.Second)
	request := httptest.NewRequest(
		http.MethodGet,
		"/providers/provider-b/wagering/transactions/external-1",
		nil,
	)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusForbidden)
	}
}

func TestLedgerEndpointRejectsInvalidCursor(t *testing.T) {
	verifier, token := newOIDCTestVerifier(t, "wallet-internal", []string{"wallet-internal"})
	server := NewRouter(nil, nil, verifier, nil, nil, "", time.Second)
	request := httptest.NewRequest(
		http.MethodGet,
		"/wallets/516be6a5-8338-4560-a723-0fc1e6e6e801/ledger?cursor=invalid",
		nil,
	)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusBadRequest)
	}
}

func newOIDCTestVerifier(t *testing.T, providerID string, roles []string) (*apiauth.Verifier, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"jwks_uri": server.URL + "/jwks",
			})
		case "/jwks":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"keys": []map[string]string{{
					"kid": "test-key",
					"kty": "RSA",
					"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(bigExponent(key.E)),
				}},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	claims := map[string]any{
		"iss": server.URL,
		"aud": []string{"jungle-api"},
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"azp": providerID,
	}
	if roles != nil {
		claims["realm_access"] = map[string]any{"roles": roles}
	}
	return apiauth.NewVerifier(server.URL, "jungle-api", "wallet-internal"), signTestToken(t, key, claims)
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	unsigned := encodedHeader + "." + encodedPayload
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func bigExponent(value int) []byte {
	if value == 0 {
		return nil
	}
	result := make([]byte, 0, 4)
	for value > 0 {
		result = append([]byte{byte(value)}, result...)
		value >>= 8
	}
	return result
}
