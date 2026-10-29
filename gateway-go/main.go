// A demo API gateway in front of the APIs of a realm owner protected by Orgkeep (phantom token pattern):
//
//	(client) --lightweight token--> (gateway) --full token--> (APIs)
//
// The clients send lightweight tokens, small whatever their rights. The gateway introspects each new one once, keeps
// the full token until it expires, and passes it to the APIs, which verify it offline as usual. However many API
// instances autoscale behind it, Keycloak only sees one introspection per token.
//
// Configured by environment variables, with the demo of Orgkeep as defaults:
// KEYCLOAK_ISSUER (http://localhost:8280/realms/demo), UPSTREAM (http://localhost:8290, greetings-api-spring),
// CATALOG_SECRETS (catalog=secret,... the secrets generated in the dashboard, page of each catalog), PORT (8292).
package main

import (
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

const markerClaim = "lightweight"

func main() {
	issuer := env("KEYCLOAK_ISSUER", "http://localhost:8280/realms/demo")
	upstream, err := url.Parse(env("UPSTREAM", "http://localhost:8290"))
	if err != nil {
		log.Fatalf("UPSTREAM: %v", err)
	}
	secrets := parseSecrets(os.Getenv("CATALOG_SECRETS"))
	if len(secrets) == 0 {
		log.Printf("no CATALOG_SECRETS: the lightweight tokens will be refused")
	}
	port := env("PORT", "8292")

	// a timeout on every call to Keycloak: a slow Keycloak must not pile up the requests of the gateway
	keycloak := &http.Client{Timeout: 5 * time.Second}
	verifier := NewVerifier(issuer, keycloak)
	if err := verifier.Refresh(); err != nil {
		log.Fatalf("keys of the realm: %v", err)
	}
	introspector := NewIntrospector(issuer, secrets, keycloak)
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	http.Handle("/", handler(verifier, introspector, proxy))
	log.Printf("gateway on port %s, in front of %s, catalogs %v", port, upstream, keys(secrets))
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// handler verifies every token, swaps a lightweight token for the full one, and passes the request on.
// The response header X-Gateway-Token says what happened, for the demo: passthrough, introspection or cache.
func handler(verifier *Verifier, introspector *Introspector, proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || token == "" {
			unauthorized(w, "no bearer token")
			return
		}
		// first, offline: a forged or expired token never reaches Keycloak. Without this, anyone could send made up
		// tokens marked lightweight, each one an introspection the cache knows nothing about
		claims, err := verifier.Verify(token)
		if err != nil {
			unauthorized(w, err.Error())
			return
		}

		source := "passthrough"
		if lightweight, _ := claims[markerClaim].(bool); lightweight {
			fullToken, from, err := introspector.FullToken(token, claims)
			switch {
			case errors.Is(err, ErrInactive):
				unauthorized(w, err.Error())
				return
			case err != nil:
				log.Printf("introspection failed: %v", err)
				http.Error(w, "introspection failed", http.StatusBadGateway)
				return
			}
			r.Header.Set("Authorization", "Bearer "+fullToken)
			source = from
		}
		w.Header().Set("X-Gateway-Token", source)
		proxy.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter, reason string) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	http.Error(w, reason, http.StatusUnauthorized)
}

// parseSecrets reads "prod=secret1,sandbox=secret2".
func parseSecrets(value string) map[string]string {
	secrets := map[string]string{}
	for _, pair := range strings.Split(value, ",") {
		if catalog, secret, ok := strings.Cut(strings.TrimSpace(pair), "="); ok && catalog != "" && secret != "" {
			secrets[catalog] = secret
		}
	}
	return secrets
}

// keys lists the catalogs, never their secrets, for the startup log.
func keys(secrets map[string]string) []string {
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	return names
}

func env(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}
