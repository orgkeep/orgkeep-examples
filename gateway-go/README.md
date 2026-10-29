# gateway-go

A demo API gateway in Go, in front of the APIs of a realm owner protected by Orgkeep (the *phantom token* pattern):

```
(client) --lightweight token--> (gateway) --full token--> (APIs, autoscaled)
```

- The clients send **lightweight tokens**: about 800 bytes whatever their rights, nothing of their content on the
  public network.
- The gateway **introspects each new token once**, gets the full token, signed by Keycloak, keeps it until it expires,
  and passes it to the APIs as the bearer.
- The APIs receive **full tokens** and verify them offline, as usual. They need no introspection credentials, and
  however many instances autoscale behind the gateway, Keycloak only sees one introspection per token.

The load on Keycloak depends on the number of active tokens, not of requests: 1 000 clients with tokens of 5 minutes
make about 3 introspections a second, whether they send a million requests or a billion (times the number of gateway
replicas, each with its own cache).

## How

For each request ([`main.go`](main.go)):

1. **Verify the token offline first** ([`jwt.go`](jwt.go)): RS256 signature with the keys of the realm (JWKS),
   issuer, expiry. A lightweight token is a signed JWT too. Without this step, anyone could send made-up tokens
   marked lightweight, each one an introspection the cache knows nothing about: an amplification attack on Keycloak.
   Here only real, unexpired tokens of Keycloak can cost an introspection.
2. **A full token** (no marker) goes through as it is.
3. **A lightweight token** (`"lightweight": true`, a convention of Orgkeep) is swapped for the full one
   ([`introspection.go`](introspection.go)):
   - from the cache, keyed by a hash of the token, until its expiry;
   - otherwise by introspection, with **`Accept: application/jwt`**: Keycloak then adds the full access token, signed,
     as a `jwt` field of the answer (Orgkeep turns "Support JWT claim in Introspection Response" on for every catalog
     client). It is the full version of the same token: same `jti`, same `exp`;
   - the requests arriving together with the same new token share **one** introspection
     (`golang.org/x/sync/singleflight`).
4. The request goes on to the API with the full token. The response header `X-Gateway-Token` says what happened,
   for the demo: `passthrough`, `introspection` or `cache`.

**Which credentials.** Keycloak only lets a client introspect the tokens it is the audience of, and Orgkeep makes the
catalog client the introspecting one: the gateway needs the secret of each catalog it serves, and picks it by the
`scope` claim of the token (the lightweight token has no `aud`).

**Not in this demo:** a rate limit per client on new tokens (a client asking for a new token on every call makes
every request an introspection), metrics (the ratio of introspections to requests is the one to watch), a shared
cache between replicas.

## Run it

The demo of Orgkeep must run (see the [README](../README.md) of the examples: Keycloak on 8280, realm `demo`), and an
API behind the gateway, by default [`greetings-api-spring`](../greetings-api-spring) on 8290. The API needs no
secret: behind the gateway, it only receives full tokens.

1. In the dashboard, as `realm-owner`: **Login client** page, turn on **Lightweight tokens**; page of the catalog
   **prod**, **Generate a secret**.
2. Run the gateway (Go 1.25 or later):

   ```
   CATALOG_SECRETS=prod=<the secret> go run .
   ```

   Other environment variables, with the demo as defaults: `KEYCLOAK_ISSUER` (`http://localhost:8280/realms/demo`),
   `UPSTREAM` (`http://localhost:8290`), `PORT` (`8292`). Several catalogs: `CATALOG_SECRETS=prod=...,sandbox=...`.
3. The requests of [`gateway.http`](gateway.http).

Measured on the demo, for Paul with `scope=openid prod sandbox`:

| | Header sent by the client | Header received by the API | `X-Gateway-Token` |
|---|---|---|---|
| first request | 805 bytes (lightweight) | 2 221 bytes (full, `"verification": "jwt (offline)"`) | `introspection` |
| next requests | 805 bytes | 2 221 bytes | `cache` |

50 concurrent requests with a new token: one introspection. The same lightweight token sent directly to the API,
which has no secret: `401`.

## Tests

```
go test -race ./...
```

Against a fake Keycloak ([`gateway_test.go`](gateway_test.go), `httptest`): a full token passes through without any
call to Keycloak; a lightweight token is swapped, then served from the cache; 50 concurrent requests share one
introspection; forged, expired, unsigned (`alg: none`) or foreign tokens never reach Keycloak; an inactive token is
refused and not cached.
