# Orgkeep examples

Everything to try Orgkeep: a demo stack to run it, and APIs of a realm owner protected by Orgkeep, what an
integrator writes.

## Demo stack

[`docker-compose`](docker-compose): Keycloak, PostgreSQL, Orgkeep (API and dashboard) and Mailpit, with demo orgs
and accounts. See the comment at the top of [`docker-compose.yaml`](docker-compose/docker-compose.yaml).

```
cd docker-compose
docker compose up -d
```

- Dashboard: http://localhost:8284 (`realm-owner` / `password`)
- Keycloak: http://localhost:8280 (admin console: `admin` / `admin`), realm `demo`
- Emails: http://localhost:8025

## Demo APIs

The same greetings API in several stacks, each a complete project of its own, accepting full and lightweight access
tokens.

| Project | Stack | Port |
|---|---|---|
| [`greetings-api-spring`](greetings-api-spring) | Spring Boot, Spring Security resource server | 8290 |
| [`greetings-api-vertx`](greetings-api-vertx) | Vert.x 5, vertx-auth-oauth2 | 8291 |

Each one builds and runs from its own folder (see its README). To build them all at once:

```
mvn package
```

They run against the demo stack above (Keycloak on 8280, realm `demo`).

## Demo API gateway

[`gateway-go`](gateway-go) (Go, port 8292): in front of the APIs, it introspects the lightweight tokens of the
clients once each, and passes full tokens to the APIs behind it (the *phantom token* pattern). For many autoscaled
API instances: Keycloak only sees one introspection per token. A Go module, not a Maven module: `go run .` and
`go test ./...` in its folder.
