package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Introspector exchanges a lightweight token for the full one, by introspection (RFC 7662) with
// "Accept: application/jwt": Keycloak then adds the full access token, signed, as a "jwt" field of the answer.
// Orgkeep turns this on for every catalog client ("Support JWT claim in Introspection Response").
//
// The cost depends on the number of tokens, not of requests: each full token is kept until its expiry, and the
// concurrent requests carrying the same new token share one introspection (singleflight).
type Introspector struct {
	url     string
	secrets map[string]string // catalog -> secret of its client
	client  *http.Client

	group singleflight.Group
	mu    sync.RWMutex
	cache map[string]cached // by hash of the lightweight token
}

type cached struct {
	fullToken string
	expiresAt time.Time
}

// ErrInactive is a token Keycloak no longer accepts (logout, user disabled, not of a catalog of the gateway).
var ErrInactive = errors.New("inactive token")

// NewIntrospector takes the realm as the gateway reaches it (see NewVerifier).
func NewIntrospector(realmURL string, secrets map[string]string, client *http.Client) *Introspector {
	return &Introspector{
		url:     realmURL + "/protocol/openid-connect/token/introspect",
		secrets: secrets,
		client:  client,
		cache:   map[string]cached{},
	}
}

// FullToken returns the full token of a lightweight one, from the cache or by introspection, and where it came from.
// The lightweight token must have been verified first (signature, issuer, expiry).
func (in *Introspector) FullToken(token string, claims Claims) (string, string, error) {
	key := hash(token)
	in.mu.RLock()
	entry, found := in.cache[key]
	in.mu.RUnlock()
	if found && time.Now().Before(entry.expiresAt) {
		return entry.fullToken, "cache", nil
	}

	// the requests arriving together with this new token wait for the same introspection
	full, err, _ := in.group.Do(key, func() (any, error) {
		fullToken, err := in.introspect(token, claims)
		if err != nil {
			return nil, err
		}
		in.store(key, fullToken, claims)
		return fullToken, nil
	})
	if err != nil {
		return "", "", err
	}
	return full.(string), "introspection", nil
}

// introspect asks Keycloak for the full token, with the credentials of a catalog of the token.
func (in *Introspector) introspect(token string, claims Claims) (string, error) {
	catalog, secret, err := in.credentials(claims)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodPost, in.url, strings.NewReader(url.Values{"token": {token}}.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// without this header, Keycloak answers the claims as JSON, which is no bearer token
	request.Header.Set("Accept", "application/jwt")
	request.SetBasicAuth(catalog, secret)

	response, err := in.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("introspection: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// 401: wrong secret of the catalog
		return "", fmt.Errorf("introspection by %s: HTTP %d", catalog, response.StatusCode)
	}
	var answer struct {
		Active bool   `json:"active"`
		JWT    string `json:"jwt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		return "", fmt.Errorf("introspection: %w", err)
	}
	if !answer.Active {
		return "", ErrInactive
	}
	if answer.JWT == "" {
		return "", fmt.Errorf("no jwt in the introspection by %s: is \"Support JWT claim in Introspection Response\" on?", catalog)
	}
	log.Printf("introspection by %s: full token of %d bytes (lightweight %d)", catalog, len(answer.JWT), len(token))
	return answer.JWT, nil
}

// credentials picks the catalog to introspect with: the first catalog of the token (its "scope" claim, the
// lightweight token has no "aud") the gateway has a secret for. Keycloak only lets a catalog client introspect the
// tokens of its catalog.
func (in *Introspector) credentials(claims Claims) (string, string, error) {
	scope, _ := claims["scope"].(string)
	for _, name := range strings.Fields(scope) {
		if secret, found := in.secrets[name]; found {
			return name, secret, nil
		}
	}
	return "", "", fmt.Errorf("%w: no catalog of the gateway in the scope %q", ErrInactive, scope)
}

// store keeps the full token until the expiry of the lightweight one (the same "exp": the same token, in full),
// and drops the expired entries as it goes.
func (in *Introspector) store(key, fullToken string, claims Claims) {
	exp, _ := claims["exp"].(float64)
	now := time.Now()
	in.mu.Lock()
	defer in.mu.Unlock()
	for k, entry := range in.cache {
		if !now.Before(entry.expiresAt) {
			delete(in.cache, k)
		}
	}
	in.cache[key] = cached{fullToken: fullToken, expiresAt: time.Unix(int64(exp), 0)}
}

// hash is the key of a token in the cache: the token itself is a credential, it is not kept as a key.
func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
