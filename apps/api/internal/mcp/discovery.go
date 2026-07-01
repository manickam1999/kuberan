package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ProtectedResourceMetadataPath is the well-known path (RFC 9728) at which the
// MCP Resource Server publishes its OAuth 2.0 Protected Resource Metadata.
const ProtectedResourceMetadataPath = "/.well-known/oauth-protected-resource"

// OAuthConfig carries the OAuth Resource Server settings the MCP server needs
// to advertise discovery metadata (and, in later phases, to validate tokens).
type OAuthConfig struct {
	// Issuer is Hydra's public issuer URL (the authorization server).
	Issuer string
	// ResourceURL is this Resource Server's identifier (MCP_RESOURCE_URL), used
	// as the `resource` value and as the base for the resource_metadata URL.
	ResourceURL string
	// Scopes is the space-delimited set of scopes the RS supports.
	Scopes string
}

// ProtectedResourceMetadata is the RFC 9728 OAuth 2.0 Protected Resource
// Metadata document. An MCP client fetches it to learn which authorization
// server issues tokens for this resource and which scopes it accepts.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// newProtectedResourceMetadata builds the metadata document, advertising the
// Hydra issuer as the sole authorization server and header-borne bearer tokens.
func newProtectedResourceMetadata(cfg OAuthConfig) ProtectedResourceMetadata {
	return ProtectedResourceMetadata{
		Resource:               cfg.ResourceURL,
		AuthorizationServers:   []string{cfg.Issuer},
		ScopesSupported:        strings.Fields(cfg.Scopes),
		BearerMethodsSupported: []string{"header"},
	}
}

// discoveryHandler serves the RFC 9728 Protected Resource Metadata document.
func discoveryHandler(cfg OAuthConfig) http.HandlerFunc {
	body, _ := json.Marshal(newProtectedResourceMetadata(cfg))
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

// resourceMetadataURL joins the resource base URL with the well-known metadata
// path, tolerating a trailing slash on the configured resource URL.
func resourceMetadataURL(resourceURL string) string {
	return strings.TrimRight(resourceURL, "/") + ProtectedResourceMetadataPath
}

// challengeWriter wraps an http.ResponseWriter to inject a WWW-Authenticate
// challenge on 401 responses, pointing clients at the resource metadata
// document (RFC 9728 section 5.1) so they can begin the OAuth flow.
type challengeWriter struct {
	http.ResponseWriter
	challenge string
}

// WriteHeader adds the WWW-Authenticate header before a 401 status is written,
// unless the wrapped handler already set one.
func (w *challengeWriter) WriteHeader(status int) {
	if status == http.StatusUnauthorized && w.Header().Get("WWW-Authenticate") == "" {
		w.Header().Set("WWW-Authenticate", w.challenge)
	}
	w.ResponseWriter.WriteHeader(status)
}

// withWWWAuthenticate wraps next so that any 401 it emits carries a
// `WWW-Authenticate: Bearer resource_metadata="…"` challenge advertising the
// RFC 9728 metadata document for this resource.
func withWWWAuthenticate(next http.Handler, cfg OAuthConfig) http.Handler {
	challenge := fmt.Sprintf("Bearer resource_metadata=%q", resourceMetadataURL(cfg.ResourceURL))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&challengeWriter{ResponseWriter: w, challenge: challenge}, r)
	})
}
