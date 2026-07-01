package mcp

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessClaims holds the fields the Resource Server extracts from a validated
// Hydra-issued access token.
type AccessClaims struct {
	// Subject is the Kuberan user ID (Hydra `sub`, set at AcceptLogin time).
	Subject string
	// Scopes is the granted scope set, parsed from the space-delimited `scope`.
	Scopes []string
}

// TokenValidator validates a raw bearer access token and returns its claims.
// It is an interface so the MCP server can be tested with a fake validator.
type TokenValidator interface {
	Validate(ctx context.Context, raw string) (*AccessClaims, error)
}

// jwk is a single RSA key from a JWKS document.
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

// HydraValidator validates JWT access tokens issued by Ory Hydra against the
// authorization server's published JWKS. Keys are fetched on demand and cached;
// an unknown key ID triggers a refresh to handle key rotation.
type HydraValidator struct {
	jwksURL  string
	issuer   string
	audience string
	client   *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	// minRefreshInterval bounds how often an unknown-kid miss will hit the
	// network, so a stream of bogus kids cannot be used to hammer Hydra.
	minRefreshInterval time.Duration
}

// NewHydraValidator constructs a validator. jwksURL is the authorization
// server's JWKS endpoint (e.g. issuer + "/.well-known/jwks.json"), issuer is the
// expected `iss` claim, and audience is the expected entry in the `aud` claim
// (the MCP resource URL).
func NewHydraValidator(jwksURL, issuer, audience string) *HydraValidator {
	return &HydraValidator{
		jwksURL:            jwksURL,
		issuer:             issuer,
		audience:           audience,
		client:             &http.Client{Timeout: 10 * time.Second},
		keys:               make(map[string]*rsa.PublicKey),
		minRefreshInterval: 30 * time.Second,
	}
}

// Validate verifies the token's RS256 signature against the cached JWKS and
// checks the standard claims (exp/nbf, issuer, audience). On success it returns
// the subject and granted scopes.
func (v *HydraValidator) Validate(ctx context.Context, raw string) (*AccessClaims, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("empty access token")
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
	)

	claims := jwt.MapClaims{}
	_, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("access token missing kid header")
		}
		return v.keyForKid(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("access token validation failed: %w", err)
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, errors.New("access token missing sub claim")
	}

	return &AccessClaims{
		Subject: sub,
		Scopes:  parseScopes(claims["scope"]),
	}, nil
}

// keyForKid returns the RSA public key for the given key ID, refreshing the
// JWKS cache once if the key is not already known (handles rotation).
func (v *HydraValidator) keyForKid(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	v.mu.RUnlock()
	if ok {
		return key, nil
	}

	if err := v.refresh(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no JWKS key for kid %q", kid)
	}
	return key, nil
}

// refresh fetches and caches the current JWKS, throttled by minRefreshInterval.
func (v *HydraValidator) refresh(ctx context.Context) error {
	v.mu.Lock()
	if !v.fetchedAt.IsZero() && time.Since(v.fetchedAt) < v.minRefreshInterval {
		v.mu.Unlock()
		return nil
	}
	v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("build JWKS request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: unexpected status %d", resp.StatusCode)
	}

	var doc jwks
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := k.rsaPublicKey()
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

// rsaPublicKey reconstructs an *rsa.PublicKey from the JWK's base64url modulus
// and exponent.
func (k jwk) rsaPublicKey() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, errors.New("empty modulus or exponent")
	}

	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() > int64(^uint32(0)) {
		return nil, errors.New("exponent out of range")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(e.Int64()),
	}, nil
}

// parseScopes normalizes a `scope` claim (space-delimited string per RFC 8693,
// or an array as some servers emit) into a slice.
func parseScopes(raw interface{}) []string {
	switch v := raw.(type) {
	case string:
		return strings.Fields(v)
	case []interface{}:
		scopes := make([]string, 0, len(v))
		for _, s := range v {
			if str, ok := s.(string); ok && str != "" {
				scopes = append(scopes, str)
			}
		}
		return scopes
	default:
		return nil
	}
}
