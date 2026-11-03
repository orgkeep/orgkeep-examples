package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Claims are the decoded payload of a JWT.
type Claims map[string]any

// Verifier checks the signature (RS256), the issuer and the expiry of a JWT with the public keys of the realm
// (JWKS), offline: no call to Keycloak per token. Every token is checked this way before anything else, the
// lightweight ones included: only a real token of Keycloak, still valid, may cost an introspection.
type Verifier struct {
	issuer  string
	jwksURL string
	client  *http.Client

	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey // by kid
	lastRefresh time.Time
}

// NewVerifier takes the issuer of the tokens (Keycloak as the browsers see it) and the realm as the gateway reaches it,
// for the keys: the same URL, except in Docker, where localhost is the container itself.
func NewVerifier(issuer, realmURL string, client *http.Client) *Verifier {
	return &Verifier{
		issuer:  issuer,
		jwksURL: realmURL + "/protocol/openid-connect/certs",
		client:  client,
		keys:    map[string]*rsa.PublicKey{},
	}
}

// Verify returns the claims of a valid token, or an error.
func (v *Verifier) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("not a JWT")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodePart(parts[0], &header); err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	// only RS256, the default of Keycloak: never "none", never an algorithm chosen by the token itself
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("algorithm %q not accepted", header.Alg)
	}
	key, err := v.key(header.Kid)
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return nil, errors.New("invalid signature")
	}

	var claims Claims
	if err := decodePart(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("payload: %w", err)
	}
	if claims["iss"] != v.issuer {
		return nil, fmt.Errorf("issuer %v not accepted", claims["iss"])
	}
	exp, ok := claims["exp"].(float64)
	if !ok || time.Now().Unix() >= int64(exp) {
		return nil, errors.New("expired")
	}
	return claims, nil
}

// key returns the public key of this kid. An unknown kid reloads the JWKS (Keycloak rotated its keys), at most once
// every 10 seconds: a token with a made up kid must not make the gateway call Keycloak on every request.
func (v *Verifier) key(kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, found := v.keys[kid]
	recent := time.Since(v.lastRefresh) < 10*time.Second
	v.mu.RUnlock()
	if found {
		return key, nil
	}
	if recent {
		return nil, fmt.Errorf("unknown key %q", kid)
	}
	if err := v.Refresh(); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if key, found = v.keys[kid]; !found {
		return nil, fmt.Errorf("unknown key %q", kid)
	}
	return key, nil
}

// Refresh loads the public keys of the realm (JWKS).
func (v *Verifier) Refresh() error {
	response, err := v.client.Get(v.jwksURL)
	if err != nil {
		return fmt.Errorf("JWKS: %w", err)
	}
	defer response.Body.Close()
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(response.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("JWKS: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, jwk := range jwks.Keys {
		// the realm also publishes its encryption keys ("enc"): only the signing keys
		if jwk.Kty != "RSA" || jwk.Use == "enc" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(jwk.N)
		e, errE := base64.RawURLEncoding.DecodeString(jwk.E)
		if errN != nil || errE != nil {
			continue
		}
		keys[jwk.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	v.mu.Lock()
	v.keys = keys
	v.lastRefresh = time.Now()
	v.mu.Unlock()
	return nil
}

// decodePart decodes a base64url part of a JWT (header or payload) into target.
func decodePart(part string, target any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
