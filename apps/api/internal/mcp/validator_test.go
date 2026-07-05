package mcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://auth.example.com"
	testAudience = "https://mcp.example.com"
	testKid      = "test-key-1"
)

// newJWKSServer starts an httptest server serving a JWKS document for the given
// RSA public key and returns the server plus its URL. It also reports how many
// times the JWKS endpoint was hit via the supplied counter.
func newJWKSServer(t *testing.T, pub *rsa.PublicKey, kid string, hits *int32) *httptest.Server {
	t.Helper()
	doc := jwks{Keys: []jwk{{
		Kid: kid,
		Kty: "RSA",
		Alg: "RS256",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// signToken signs a token with the given key/kid and claims.
func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func validClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub":   "user-123",
		"iss":   testIssuer,
		"aud":   []string{testAudience},
		"exp":   now.Add(15 * time.Minute).Unix(),
		"iat":   now.Unix(),
		"nbf":   now.Add(-time.Minute).Unix(),
		"scope": "read:accounts read:transactions",
	}
}

func TestHydraValidator_Validate(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	srv := newJWKSServer(t, &key.PublicKey, testKid, nil)
	newValidator := func() *HydraValidator {
		return NewHydraValidator(srv.URL, testIssuer, testAudience)
	}

	t.Run("valid token", func(t *testing.T) {
		v := newValidator()
		tok := signToken(t, key, testKid, validClaims())
		claims, err := v.Validate(context.Background(), tok)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if claims.Subject != "user-123" {
			t.Errorf("subject = %q, want user-123", claims.Subject)
		}
		if len(claims.Scopes) != 2 || claims.Scopes[0] != "read:accounts" {
			t.Errorf("scopes = %v, want [read:accounts read:transactions]", claims.Scopes)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		if _, err := newValidator().Validate(context.Background(), "  "); err == nil {
			t.Fatal("expected error for empty token")
		}
	})

	t.Run("wrong signing key", func(t *testing.T) {
		tok := signToken(t, otherKey, testKid, validClaims())
		if _, err := newValidator().Validate(context.Background(), tok); err == nil {
			t.Fatal("expected error for token signed by unknown key")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		c := validClaims()
		c["exp"] = time.Now().Add(-time.Hour).Unix()
		tok := signToken(t, key, testKid, c)
		if _, err := newValidator().Validate(context.Background(), tok); err == nil {
			t.Fatal("expected error for expired token")
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		c := validClaims()
		c["iss"] = "https://evil.example.com"
		tok := signToken(t, key, testKid, c)
		if _, err := newValidator().Validate(context.Background(), tok); err == nil {
			t.Fatal("expected error for wrong issuer")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		c := validClaims()
		c["aud"] = []string{"https://other.example.com"}
		tok := signToken(t, key, testKid, c)
		if _, err := newValidator().Validate(context.Background(), tok); err == nil {
			t.Fatal("expected error for wrong audience")
		}
	})

	t.Run("missing sub", func(t *testing.T) {
		c := validClaims()
		delete(c, "sub")
		tok := signToken(t, key, testKid, c)
		if _, err := newValidator().Validate(context.Background(), tok); err == nil {
			t.Fatal("expected error for missing sub")
		}
	})

	t.Run("unknown kid", func(t *testing.T) {
		tok := signToken(t, key, "nonexistent-kid", validClaims())
		if _, err := newValidator().Validate(context.Background(), tok); err == nil {
			t.Fatal("expected error for unknown kid")
		}
	})

	t.Run("none algorithm rejected", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims())
		tok.Header["kid"] = testKid
		signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("sign none token: %v", err)
		}
		if _, err := newValidator().Validate(context.Background(), signed); err == nil {
			t.Fatal("expected error for alg=none token")
		}
	})
}

func TestHydraValidator_ScopeFormats(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	srv := newJWKSServer(t, &key.PublicKey, testKid, nil)
	v := NewHydraValidator(srv.URL, testIssuer, testAudience)

	t.Run("array scope claim", func(t *testing.T) {
		c := validClaims()
		c["scope"] = []interface{}{"read:budgets", "read:portfolio"}
		tok := signToken(t, key, testKid, c)
		claims, err := v.Validate(context.Background(), tok)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(claims.Scopes) != 2 || claims.Scopes[1] != "read:portfolio" {
			t.Errorf("scopes = %v", claims.Scopes)
		}
	})

	t.Run("missing scope claim", func(t *testing.T) {
		c := validClaims()
		delete(c, "scope")
		tok := signToken(t, key, testKid, c)
		claims, err := v.Validate(context.Background(), tok)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(claims.Scopes) != 0 {
			t.Errorf("scopes = %v, want empty", claims.Scopes)
		}
	})
}

func TestHydraValidator_JWKSCaching(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var hits int32
	srv := newJWKSServer(t, &key.PublicKey, testKid, &hits)
	v := NewHydraValidator(srv.URL, testIssuer, testAudience)

	for i := 0; i < 3; i++ {
		tok := signToken(t, key, testKid, validClaims())
		if _, err := v.Validate(context.Background(), tok); err != nil {
			t.Fatalf("validate %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("JWKS fetched %d times, want 1 (cached)", got)
	}
}

func TestHydraValidator_RefreshThrottle(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var hits int32
	srv := newJWKSServer(t, &key.PublicKey, testKid, &hits)
	v := NewHydraValidator(srv.URL, testIssuer, testAudience)

	// Repeated unknown-kid tokens must not fetch JWKS more than once within the
	// throttle window.
	for i := 0; i < 5; i++ {
		tok := signToken(t, key, "unknown-kid", validClaims())
		if _, err := v.Validate(context.Background(), tok); err == nil {
			t.Fatal("expected error for unknown kid")
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("JWKS fetched %d times, want 1 (throttled)", got)
	}
}
