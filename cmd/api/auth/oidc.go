package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	ErrAuthUnavailable = errors.New("authentication unavailable")
)

type Principal struct {
	ProviderID string
	Internal   bool
}

type Verifier struct {
	issuer       string
	audience     string
	internalRole string
	client       *http.Client
	mu           sync.RWMutex
	keys         map[string]*rsa.PublicKey
	jwksURL      string
}

func NewVerifier(issuer, audience, internalRole string) *Verifier {
	return &Verifier{
		issuer:       strings.TrimRight(strings.TrimSpace(issuer), "/"),
		audience:     strings.TrimSpace(audience),
		internalRole: strings.TrimSpace(internalRole),
		client:       &http.Client{Timeout: 5 * time.Second},
		keys:         make(map[string]*rsa.PublicKey),
	}
}

func (v *Verifier) Authenticate(request *http.Request) (Principal, error) {
	if v.issuer == "" || v.audience == "" {
		return Principal{}, ErrAuthUnavailable
	}
	authorization := strings.TrimSpace(request.Header.Get("Authorization"))
	if !strings.HasPrefix(authorization, "Bearer ") {
		return Principal{}, ErrUnauthorized
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, ErrUnauthorized
	}
	header, err := decodeJSON(parts[0])
	algorithm := stringClaim(header, "alg")
	if err != nil ||
		(algorithm != "RS256" && algorithm != "RS384" && algorithm != "RS512") {
		return Principal{}, ErrUnauthorized
	}
	claims, err := decodeJSON(parts[1])
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	if stringClaim(claims, "iss") != v.issuer || !audienceContains(claims["aud"], v.audience) || !validTimes(claims) {
		return Principal{}, ErrUnauthorized
	}
	key, err := v.key(request.Context(), stringClaim(header, "kid"))
	if err != nil {
		return Principal{}, ErrAuthUnavailable
	}
	hashID := signingHash(stringClaim(header, "alg"))
	digest := hashID.New()
	_, _ = digest.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := decode(parts[2])
	if err != nil || rsa.VerifyPKCS1v15(key, hashID, digest.Sum(nil), signature) != nil {
		return Principal{}, ErrUnauthorized
	}
	providerID := firstClaim(claims, "providerId", "provider_id", "azp", "client_id", "sub")
	return Principal{ProviderID: providerID, Internal: hasRole(claims, v.internalRole)}, nil
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key := v.keys[kid]
	v.mu.RUnlock()
	if key != nil {
		return key, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	key = v.keys[kid]
	if key == nil {
		return nil, errors.New("signing key not found")
	}
	return key, nil
}

func (v *Verifier) refresh(ctx context.Context) error {
	v.mu.RLock()
	jwksURL := v.jwksURL
	v.mu.RUnlock()
	if jwksURL == "" {
		body, err := v.get(ctx, v.issuer+"/.well-known/openid-configuration")
		if err != nil {
			return err
		}
		var discovery struct {
			JWKSURL string `json:"jwks_uri"`
		}
		if json.Unmarshal(body, &discovery) != nil || discovery.JWKSURL == "" {
			return errors.New("invalid OIDC discovery")
		}
		jwksURL = discovery.JWKSURL
		v.mu.Lock()
		v.jwksURL = jwksURL
		v.mu.Unlock()
	}
	body, err := v.get(ctx, jwksURL)
	if err != nil {
		return err
	}
	var document struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if json.Unmarshal(body, &document) != nil {
		return errors.New("invalid JWKS")
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, item := range document.Keys {
		if item.Kty != "RSA" || item.Kid == "" {
			continue
		}
		n, errN := decode(item.N)
		e, errE := decode(item.E)
		if errN != nil || errE != nil {
			continue
		}
		exponent := 0
		for _, b := range e {
			exponent = exponent<<8 | int(b)
		}
		if exponent == 0 {
			continue
		}
		keys[item.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
	}
	if len(keys) == 0 {
		return errors.New("no RSA keys")
	}
	v.mu.Lock()
	for kid, key := range keys {
		v.keys[kid] = key
	}
	v.mu.Unlock()
	return nil
}

func (v *Verifier) get(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := v.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC endpoint returned %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 2<<20))
}

func decodeJSON(value string) (map[string]json.RawMessage, error) {
	decoded, err := decode(value)
	if err != nil {
		return nil, err
	}
	var result map[string]json.RawMessage
	err = json.Unmarshal(decoded, &result)
	return result, err
}

func decode(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}

func stringClaim(claims map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(claims[key], &value)
	return value
}

func firstClaim(claims map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringClaim(claims, key)); value != "" {
			return value
		}
	}
	return ""
}

func audienceContains(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var many []string
	if json.Unmarshal(raw, &many) != nil {
		return false
	}
	for _, value := range many {
		if value == expected {
			return true
		}
	}
	return false
}

func validTimes(claims map[string]json.RawMessage) bool {
	now := time.Now().Unix()
	var exp, nbf int64
	if json.Unmarshal(claims["exp"], &exp) != nil || exp <= now {
		return false
	}
	if raw, ok := claims["nbf"]; ok && json.Unmarshal(raw, &nbf) == nil && nbf > now {
		return false
	}
	return true
}

func signingHash(algorithm string) crypto.Hash {
	switch algorithm {
	case "RS384":
		return crypto.SHA384
	case "RS512":
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

func hasRole(claims map[string]json.RawMessage, role string) bool {
	if role == "" {
		return false
	}
	var scope string
	if json.Unmarshal(claims["scope"], &scope) == nil && containsWord(scope, role) {
		return true
	}
	var roles []string
	if json.Unmarshal(claims["roles"], &roles) == nil {
		for _, value := range roles {
			if value == role {
				return true
			}
		}
	}
	var realm struct {
		Roles []string `json:"roles"`
	}
	if json.Unmarshal(claims["realm_access"], &realm) == nil {
		for _, value := range realm.Roles {
			if value == role {
				return true
			}
		}
	}
	return false
}

func containsWord(value, expected string) bool {
	for _, item := range strings.Fields(value) {
		if item == expected {
			return true
		}
	}
	return false
}
