# greetings-api-vertx

The greetings API of a realm owner, protected by Orgkeep, in Vert.x 5 with `vertx-auth-oauth2`. The same API as
[`greetings-api-spring`](../greetings-api-spring), and the same behaviour: it accepts both kinds of access tokens
Orgkeep can give, and chooses token by token.

| Token received | How Orgkeep marks it | Verification | Calls to Keycloak |
|---|---|---|---|
| full JWT: roles, catalogs, quotas, profile | no marker | offline, with the keys of Keycloak (JWKS) | none per request |
| lightweight JWT: nothing but the token itself | `"lightweight": true` | introspection (RFC 7662), cached until the token expires | one per token |

`GET /greetings` says hello and returns what the API read: how the token was verified, the size of the
`Authorization` header, the rights of the user in the catalog of the API (`resource_access.prod.roles`) and the quotas
and parameters of its products (claim `prod`).

Measured on the demo, for Paul with `scope=openid prod sandbox`, both tokens sent to the same running API:

| | `"verification"` | `Authorization` header | Rights and quotas read |
|---|---|---|---|
| full token | `jwt (offline)` | 2 244 bytes | yes |
| lightweight token | `introspection` | 812 bytes, whatever the number of products | yes |

## The pitfalls of vertx-auth-oauth2

Vert.x needs more care than Spring here, which is the point of this example.

- **With the keys of Keycloak loaded, a provider never introspects a JWT.** `OAuth2Auth.authenticate` validates a JWT
  offline as soon as it has keys, and only introspects what it cannot decode. A lightweight token is a signed JWT:
  it passes, with nothing in it (no user, no rights, an HTTP 200). Hence **two providers**
  ([`TokenVerifier`](src/main/java/io/orgkeep/demo/greetings/TokenVerifier.java)): one with the keys for the full
  tokens, one without any key for the lightweight ones, which takes every token as opaque and introspects it.
- **The OpenID discovery sets the introspection endpoint too.** A provider created by `OpenIDConnectAuth.discover`
  introspects a JWT it refused offline (a token without the catalog in its audience): a useless call to Keycloak,
  without credentials (`invalid_client`). The offline provider is therefore created without discovery: the keys
  (`/protocol/openid-connect/certs`) and the issuer, nothing else.
- **The audience is checked against the client ID.** Offline, the `aud` of the token must contain the `clientId` of
  the provider: here `prod`, the catalog, which is in the audience of every token carrying one of its roles. A token
  without the catalog is refused, offline as by introspection.
- **No cache for the introspection.** Without one, every request is a call to Keycloak. `TokenVerifier` keeps each
  answer until the token expires (in memory, for the demo; with a maximum size in a real API). A token revoked in
  Keycloak stays valid in the cache until it expires.
- `Introspected client_id doesn't match configured client_id` in the logs is an info of Vert.x, not an error: the
  token was issued to the login client (`demo`), and the API introspects as the catalog (`prod`).

The marker is read before the token is verified, which is safe because both ways verify it: removing the marker of a
lightweight token breaks its signature (the offline way refuses it); adding it to a forged token sends it to
Keycloak, which answers `"active": false` (checked: an unsigned token carrying the marker gets a `401`).

## Run it

The demo of Orgkeep must run: `mvn quarkus:dev` in `orgkeep-dashboard` (Keycloak on 8280, dashboard on 8284), or the
demo compose in [`../docker-compose`](../docker-compose). Accounts: `realm-owner` / `password` in the dashboard, `paul` / `password` for the
tokens.

1. In the dashboard, as `realm-owner`: page of the catalog **prod**, **Generate a secret**: the API introspects with
   the credentials of its catalog (client ID `prod`).
2. Run the API with this secret:

   ```
   CATALOG_CLIENT_SECRET=<the secret> mvn compile exec:java
   ```

   Without the secret, the API starts and accepts only full tokens (a warning says so). The other environment
   variables have the demo as defaults: `KEYCLOAK_ISSUER` (`http://localhost:8280/realms/demo`), `CATALOG` (`prod`),
   `PORT` (`8291`).
3. Requests 1 and 2 of [`greetings.http`](greetings.http): a full token, `"verification": "jwt (offline)"`.
4. In the dashboard, **Login client** page, turn on **Lightweight tokens**. Requests 1 and 2 again, with a new token:
   `"verification": "introspection"`, the same rights, a smaller header.

## Code

- [`TokenVerifier`](src/main/java/io/orgkeep/demo/greetings/TokenVerifier.java): the choice by the marker, the two
  providers, the cache.
- [`GreetingsApi`](src/main/java/io/orgkeep/demo/greetings/GreetingsApi.java): the main class, the router and
  `GET /greetings`. The claims come from the attributes of the user (`accessToken`) offline, from its principal (the
  answer of the introspection) otherwise.
