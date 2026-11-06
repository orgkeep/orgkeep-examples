# Orgkeep examples

Everything to try Orgkeep: a demo stack to run it, and APIs of a realm owner protected by Orgkeep, what an
integrator writes.

## Demo stack

[`docker-compose`](docker-compose): Keycloak, PostgreSQL, Orgkeep (API and dashboard), Mailpit, and the demo APIs
below, with demo organisations, accounts and catalogs. Requires Docker with Compose v2 only.

```
cd docker-compose
docker compose up -d --build
```

- Dashboard: http://localhost:8284 (`realm-owner` / `password`)
- Keycloak: http://localhost:8280 (admin console: `admin` / `admin`), realm `demo`
- Emails: http://localhost:8025
- Demo APIs: http://localhost:8290, 8291 and 8292 (`/greetings`)

Accounts, a walkthrough of the demo APIs and how the stack is wired: [`docker-compose/README.md`](docker-compose/README.md).

## Demo APIs

The same greetings API in several stacks, each a complete project of its own, accepting full and lightweight access
tokens.

| Project | Stack | Port |
|---|---|---|
| [`greetings-api-spring`](greetings-api-spring) | Spring Boot, Spring Security resource server | 8290 |
| [`greetings-api-vertx`](greetings-api-vertx) | Vert.x 5, vertx-auth-oauth2 | 8291 |

Each one builds and runs from its own folder (see its README), or in the demo stack. To build them all at once
(Java 25, Maven):

```
mvn package
```

Outside the demo stack, they run against a Keycloak on 8280 with the realm `demo`: the demo stack, or
`mvn quarkus:dev` in `orgkeep-dashboard`.

## Demo API gateway

[`gateway-go`](gateway-go) (Go, port 8292): in front of the APIs, it introspects the lightweight tokens of the
clients once each, and passes full tokens to the APIs behind it (the *phantom token* pattern). For many autoscaled
API instances: Keycloak only sees one introspection per token. A Go module, not a Maven module: `go run .` and
`go test ./...` in its folder.

## Configuration of the demo APIs

All three are configured by environment variables, with the demo as defaults:

| Variable | Default | Used by | What |
|---|---|---|---|
| `KEYCLOAK_ISSUER` | `http://localhost:8280/realms/demo` | all | The issuer of the tokens: the realm as the browsers see Keycloak |
| `KEYCLOAK_URL` | `KEYCLOAK_ISSUER` | all | The same realm as the API reaches it, for its keys and the introspection. Differs in Docker: `http://keycloak:8080/realms/demo` in the demo stack |
| `CATALOG` | `prod` | spring, vertx | The catalog of the API: its rights in `resource_access.<catalog>`, its quotas in the claims `<catalog>.*` |
| `CATALOG_CLIENT_SECRET` | none | spring, vertx | The secret of the catalog client, to introspect lightweight tokens. Without it, only full tokens are accepted |
| `CATALOG_SECRETS` | none | gateway | `prod=<secret>,sandbox=<secret>`: a secret per catalog served |
| `UPSTREAM` | `http://localhost:8290` | gateway | The API behind the gateway |
| `PORT` / `SERVER_PORT` | 8290, 8291, 8292 | vertx and gateway / spring | The listening port |

## Docker images

Each project has a multi-stage `Dockerfile` that builds it from its sources (no JDK, Maven or Go needed):

```
docker build -t ghcr.io/orgkeep/greetings-api-spring greetings-api-spring
```

The demo stack builds them itself (`docker compose up --build`).

## Contributing

Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md). Commits are signed off (DCO), no contributor
license agreement.

## License

[Apache License 2.0](LICENSE).
