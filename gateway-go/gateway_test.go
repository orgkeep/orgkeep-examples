package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A fake Keycloak: the JWKS of a test key, and an introspection endpoint that counts its calls.
type fakeKeycloak struct {
	server         *httptest.Server
	key            *rsa.PrivateKey
	introspections atomic.Int32
	active         atomic.Bool
}

func newFakeKeycloak(t *testing.T) *fakeKeycloak {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeKeycloak{key: key}
	fake.active.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/demo/protocol/openid-connect/certs", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "test", "kty": "RSA", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/realms/demo/protocol/openid-connect/token/introspect", func(w http.ResponseWriter, r *http.Request) {
		fake.introspections.Add(1)
		// slow, so that concurrent requests really overlap
		time.Sleep(100 * time.Millisecond)
		user, password, _ := r.BasicAuth()
		if user != "prod" || password != "secret" || r.Header.Get("Accept") != "application/jwt" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"active": fake.active.Load(), "jwt": "the-full-token"})
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeKeycloak) issuer() string {
	return f.server.URL + "/realms/demo"
}

// token signs these claims with the key of the fake Keycloak (or with another key, to forge one).
func (f *fakeKeycloak) token(t *testing.T, claims map[string]any, key *rsa.PrivateKey) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test"}`))
	payload, _ := json.Marshal(claims)
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func (f *fakeKeycloak) lightweightClaims() map[string]any {
	return map[string]any{"iss": f.issuer(), "exp": time.Now().Add(5 * time.Minute).Unix(),
		"scope": "openid prod", markerClaim: true}
}

// gateway runs the handler in front of an upstream that echoes the Authorization header it received.
func gateway(t *testing.T, fake *fakeKeycloak) *httptest.Server {
	verifier := NewVerifier(fake.issuer(), http.DefaultClient)
	if err := verifier.Refresh(); err != nil {
		t.Fatal(err)
	}
	introspector := NewIntrospector(fake.issuer(), map[string]string{"prod": "secret"}, http.DefaultClient)
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("Authorization"))
	})
	server := httptest.NewServer(handler(verifier, introspector, upstream))
	t.Cleanup(server.Close)
	return server
}

func call(t *testing.T, server *httptest.Server, token string) (int, string, string) {
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/greetings", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header.Get("X-Gateway-Token"), string(body)
}

// A full token goes through as it is, without any call to Keycloak.
func TestFullTokenPassesThrough(t *testing.T) {
	fake := newFakeKeycloak(t)
	server := gateway(t, fake)
	claims := fake.lightweightClaims()
	delete(claims, markerClaim)
	token := fake.token(t, claims, fake.key)

	status, source, forwarded := call(t, server, token)

	if status != 200 || source != "passthrough" || forwarded != "Bearer "+token {
		t.Fatalf("got %d %q %q", status, source, forwarded)
	}
	if fake.introspections.Load() != 0 {
		t.Fatalf("%d introspections for a full token", fake.introspections.Load())
	}
}

// A lightweight token is swapped for the full one: one introspection, then the cache.
func TestLightweightTokenIsSwapped(t *testing.T) {
	fake := newFakeKeycloak(t)
	server := gateway(t, fake)
	token := fake.token(t, fake.lightweightClaims(), fake.key)

	status, source, forwarded := call(t, server, token)
	if status != 200 || source != "introspection" || forwarded != "Bearer the-full-token" {
		t.Fatalf("first call: got %d %q %q", status, source, forwarded)
	}
	_, source, _ = call(t, server, token)
	if source != "cache" || fake.introspections.Load() != 1 {
		t.Fatalf("second call: %q, %d introspections", source, fake.introspections.Load())
	}
}

// 50 requests at once with the same new token: one introspection, not 50.
func TestConcurrentRequestsShareOneIntrospection(t *testing.T) {
	fake := newFakeKeycloak(t)
	server := gateway(t, fake)
	token := fake.token(t, fake.lightweightClaims(), fake.key)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if status, _, _ := call(t, server, token); status != 200 {
				t.Errorf("status %d", status)
			}
		})
	}
	wg.Wait()

	if fake.introspections.Load() != 1 {
		t.Fatalf("%d introspections for 50 concurrent requests", fake.introspections.Load())
	}
}

// Forged, expired or "none" tokens marked lightweight never reach Keycloak.
func TestInvalidTokensNeverReachKeycloak(t *testing.T) {
	fake := newFakeKeycloak(t)
	server := gateway(t, fake)
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	expired := fake.lightweightClaims()
	expired["exp"] = time.Now().Add(-time.Minute).Unix()
	otherIssuer := fake.lightweightClaims()
	otherIssuer["iss"] = "http://evil/realms/demo"
	payload, _ := json.Marshal(fake.lightweightClaims())
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + "."

	for name, token := range map[string]string{
		"forged":       fake.token(t, fake.lightweightClaims(), otherKey),
		"expired":      fake.token(t, expired, fake.key),
		"other issuer": fake.token(t, otherIssuer, fake.key),
		"alg none":     unsigned,
		"garbage":      "not-a-jwt",
	} {
		if status, _, _ := call(t, server, token); status != 401 {
			t.Errorf("%s: status %d", name, status)
		}
	}
	if fake.introspections.Load() != 0 {
		t.Fatalf("%d introspections for invalid tokens", fake.introspections.Load())
	}
}

// A token Keycloak no longer accepts (logout, user disabled): 401, and nothing is cached.
func TestInactiveTokenIsRefused(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.active.Store(false)
	server := gateway(t, fake)
	token := fake.token(t, fake.lightweightClaims(), fake.key)

	if status, _, _ := call(t, server, token); status != 401 {
		t.Fatalf("status %d", status)
	}
	fake.active.Store(true)
	if _, source, _ := call(t, server, token); source != "introspection" {
		t.Fatalf("an inactive token was cached: %q", source)
	}
}
