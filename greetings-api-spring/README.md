# greetings-api-spring

An API of a realm owner, protected by Orgkeep, in Spring Boot (the same greetings API may come in other stacks).

It accepts both kinds of access tokens Orgkeep can give, and chooses token by token:

| Token received | How Orgkeep marks it | Verification | Calls to Keycloak |
|---|---|---|---|
| full JWT: roles, catalogs, quotas, profile | no marker | offline, with the keys of Keycloak (JWKS) | none per request |
| lightweight JWT: nothing but the token itself | `"lightweight": true` | introspection (RFC 7662), cached until the token expires | one per token |

This is what an API needs while the realm owner switches to lightweight tokens (both kinds arrive), and afterwards:
a full token, from a client that is not in lightweight mode, still costs no call to Keycloak.

`GET /greetings` says hello and returns what the API read: how the token was verified, the size of the
`Authorization` header, the rights of the user in the catalog of the API (`resource_access.prod.roles`) and the quotas
and parameters of its products (claim `prod`).

## What a lightweight token changes

The size of the token sent on **every** request, in a header. Proxies and servers limit a header, often to 8 KB
(nginx, Tomcat, many gateways), and a full token grows with the catalogs, products and features of the user. A
lightweight token keeps the same small size; the content comes in the **body** of the introspection, server to
server, once per token thanks to the cache, where the size does not matter.

Measured on the demo, for Paul with `scope=openid prod sandbox` (two catalogs, two products each), both tokens sent to
the same running API:

| | `"verification"` | `Authorization` header | Rights and quotas read |
|---|---|---|---|
| full token | `jwt (offline)` | 2 244 bytes | yes |
| lightweight token | `introspection` | 812 bytes, whatever the number of products | yes |

Two requests with the same lightweight token: one introspection, then one from the cache (see the logs,
`Introspection call to Keycloak` / `Introspection from the cache`).

What it does **not** change: the API still sees every catalog the token carries, in the answer of the introspection.
To see less, the application asks for fewer catalogs (`scope=openid prod`).

## Run it

The demo of Orgkeep must run: `mvn quarkus:dev` in `orgkeep-dashboard` (Keycloak on 8280, dashboard on 8284), or the
demo compose in [`../docker-compose`](../docker-compose). Accounts: `realm-owner` / `password` in the dashboard, `paul` / `password` for the
tokens.

1. In the dashboard, as `realm-owner`: page of the catalog **prod**, **Generate a secret**. The API introspects with
   the credentials of its catalog (client ID `prod`): Keycloak only lets a client introspect the tokens it is the
   audience of, and the catalog is in the audience of every token carrying one of its roles.
2. Run the API with this secret:

   ```
   CATALOG_CLIENT_SECRET=<the secret> mvn spring-boot:run
   ```

   Without the secret, the API starts and accepts only full tokens (a warning says so).
3. Requests 1 and 2 of [`greetings.http`](greetings.http): a full token, `"verification": "jwt (offline)"`.
4. In the dashboard, **Login client** page, turn on **Lightweight tokens**. Requests 1 and 2 again, with a new token:
   `"verification": "introspection"`, the same rights, a smaller header.

## How it chooses

[`SecurityConfig`](src/main/java/io/orgkeep/demo/greetings/SecurityConfig.java) gives Spring Security an
`AuthenticationManagerResolver`: for each request, it reads the claim `lightweight` of the bearer token and picks the
JWT decoder or the introspection.

The marker is read **before** the token is verified. That is safe because both ways verify it: removing the marker of
a lightweight token breaks its signature (the JWT way refuses it); adding it to a forged token sends it to Keycloak,
which answers `"active": false` (checked: an unsigned token carrying the marker gets a `401`).

## Pitfalls

- **A lightweight token is still a signed JWT.** An API that only verifies JWTs offline accepts it (valid signature,
  issuer and expiry) but finds nothing in it: no user, no rights, no quotas, and an HTTP 200. Such an API must at
  least refuse the tokens marked `"lightweight": true` (the API of Orgkeep answers them a `401`), or introspect them
  as here.
- **Spring Security does not cache the introspection.** Without a cache, each request is a call to Keycloak.
  [`CachingIntrospector`](src/main/java/io/orgkeep/demo/greetings/CachingIntrospector.java) keeps each answer until
  the token expires (in memory, for the demo; Caffeine with a maximum size in a real API). A token revoked in Keycloak
  stays valid in the cache until it expires.
- **A token without the catalog of the API** (not asked for, or a user with no right in it) is answered
  `"active": false` by the introspection: the API answers `401`, not `403`.
- **A service account** should reuse its token until it expires: a new token on every call is a new introspection on
  every call, which the cache cannot absorb.

## Code

- [`SecurityConfig`](src/main/java/io/orgkeep/demo/greetings/SecurityConfig.java): the choice by the marker, the JWT
  decoder, the cached introspection.
- [`GreetingsController`](src/main/java/io/orgkeep/demo/greetings/GreetingsController.java): reads the claims the same
  way for both kinds (`AbstractOAuth2TokenAuthenticationToken.getTokenAttributes()`).
- `application.properties`: the issuer, the catalog, and its secret (`CATALOG_CLIENT_SECRET`).
