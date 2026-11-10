# Orgkeep demo stack

Everything needed to try Orgkeep on one machine, with demo organisations, accounts and catalogs:

```
docker compose up -d --build
```

The first start takes a minute or two (Keycloak creates its database, Orgkeep creates the realm and the demo data,
the demo APIs are built from their sources). `docker compose ps` shows every service `healthy` or `running` once it
is ready.

> A demo, not a production setup: fixed passwords and secrets, plain HTTP, Keycloak exposed directly. See
> [Going further](#going-further).

## What runs

| Service | URL | What it is |
|---|---|---|
| `orgkeep` | http://localhost:8284 | Orgkeep: the dashboard, the REST API under `/api`, Swagger UI under `/q/swagger-ui` |
| `keycloak` | http://localhost:8280 | Keycloak 26 in production mode (`start`), with the `orgkeep` theme. Admin console: `admin` / `admin` |
| `postgres` | (internal) | The database of Keycloak. Orgkeep itself has no database: everything is stored in Keycloak |
| `mailpit` | http://localhost:8025 | Catches every email of the realm (invitations, password resets) |
| `greetings-api-spring` | http://localhost:8290/greetings | A demo API of a realm owner, Spring Boot |
| `greetings-api-vertx` | http://localhost:8291/greetings | The same API in Vert.x |
| `gateway-go` | http://localhost:8292/greetings | A demo API gateway in Go, in front of `greetings-api-spring` (phantom token) |

Images:
- `ghcr.io/orgkeep/orgkeep` (core + dashboard) and `ghcr.io/orgkeep/orgkeep-keycloak` (Keycloak + theme) are
  pulled from the GitHub Container Registry;
- the three demo APIs are built from the folders next to this one (`build:` in the compose file).

The Orgkeep version is `ORGKEEP_VERSION`, by default the latest release (`0.1.0`):

```
ORGKEEP_VERSION=0.1.0 docker compose up -d --build
```

## Accounts

Realm `demo`, every password is `password`:

| Account | Who | What to look at |
|---|---|---|
| `realm-owner` | Alice Martin, the realm owner | The dashboard: organisations, catalogs, products, login client |
| `org-owner` | Bob Durand, owner of the organisation `acme` | The dashboard: members and teams of `acme` |
| `org-user` | Claire Petit, a plain member of `acme` | Gets the product `billing` of the catalog `products` |
| `paul` | Paul Lefebvre, team `sales` of `acme` | Tokens for the demo APIs: catalogs `prod` and `sandbox` |
| `emma` | Emma Roux, team `accounting` of `acme` | Every feature of `billing` |

Keycloak admin console: `admin` / `admin` (realm `master`). The realm owner can also open the console of the realm
`demo` only, from the **Login client** page of the dashboard.

## Try the demo APIs

1. Get a token for Paul, through the login client of the realm (clientId `demo`; the password grant is enabled for
   the demo only):

   ```
   TOKEN=$(curl -s http://localhost:8280/realms/demo/protocol/openid-connect/token \
     -d grant_type=password -d client_id=demo -d username=paul -d password=password \
     -d "scope=openid prod" | jq -r .access_token)
   ```

2. Call the APIs:

   ```
   curl -s http://localhost:8290/greetings -H "Authorization: Bearer $TOKEN" | jq
   ```

   The answer says how the token was verified (`"verification": "jwt (offline)"`), its size, Paul's rights in the
   catalog `prod` (`billing:search`, `shop:write`...) and his quotas (`prod.billing.search`...). Same on 8291
   (Vert.x) and 8292 (through the gateway, header `X-Gateway-Token: passthrough`).

3. Switch to lightweight tokens: in the dashboard, as `realm-owner`, **Login client** page, turn on
   **Lightweight tokens**. Ask for a new token (step 1) and call again:
   - 8290 and 8291 answer `"verification": "introspection"`: a token of about 800 bytes instead of about 1 900,
     the same rights, read by introspection;
   - 8292 answers `X-Gateway-Token: introspection`, then `cache`: the gateway swapped the lightweight token for the
     full one, and the API behind it verified the full one offline.

The APIs introspect with the credentials of the catalog `prod`, whose secret is set by the demo
(`prod-demo-secret`, `sandbox-demo-secret` for `sandbox`). Generating a new secret in the dashboard (page of the
catalog) breaks them until their `CATALOG_CLIENT_SECRET` / `CATALOG_SECRETS` is changed in `docker-compose.yaml`.

More about each one in its README: [`greetings-api-spring`](../greetings-api-spring),
[`greetings-api-vertx`](../greetings-api-vertx), [`gateway-go`](../gateway-go).

## The API of Orgkeep

- Swagger UI: http://localhost:8284/q/swagger-ui, **Authorize** with a username and a password (`realm-owner` /
  `password`).
- OpenAPI contract: http://localhost:8284/q/openapi
- The requests of `orgkeep-core/src/test/http/demo.http` target the core alone in dev (port 8282): replace the port
  with 8284.

## How it is wired

- **Two URLs for Keycloak.** The browser and the tokens use `http://localhost:8280` (`KC_HOSTNAME`): that is the
  issuer of every token. The containers reach Keycloak as `http://keycloak:8080`, since `localhost` is the
  container itself. `KC_HOSTNAME_BACKCHANNEL_DYNAMIC` makes Keycloak give them internal URLs for the keys and the
  introspection, while keeping the public issuer. Each container is therefore given both: `KEYCLOAK_URL` (where to
  connect) and the public URL (`ORGKEEP_KEYCLOAK_PUBLIC_URL` for Orgkeep, `KEYCLOAK_ISSUER` for the demo APIs).
- **Startup order.** `postgres` → `keycloak` (healthy on its `/health/ready`) → `orgkeep` (healthy once it has
  created the realm `demo`) → the demo APIs, which read the keys of the realm at startup.
- **Demo data** comes from `ORGKEEP_DEMO_ENABLED=true`: Orgkeep creates the organisation `acme`, its members, the
  catalogs `prod` and `sandbox` with their products and teams, and the `orgkeep-dashboard` client. Only when
  missing: a restart keeps everything, including what was changed in the dashboard.
- **Emails** go from Keycloak to Mailpit (`ORGKEEP_SMTP_*`): invite a member in the dashboard, then open
  http://localhost:8025.

Every variable of the `orgkeep` service is described in the README of
[orgkeep-core](https://github.com/orgkeep/orgkeep-core) (configuration of the API) and
[orgkeep-dashboard](https://github.com/orgkeep/orgkeep-dashboard) (the dashboard client).

## Start over

```
docker compose down -v
```

`-v` removes the volume of PostgreSQL: the next start creates a new realm with the demo data. Without it, the data
is kept between restarts.

## Build the images yourself

To run unreleased changes, build the Orgkeep images from the other repositories, cloned next to this one. They get the
same names as the published ones, tagged with the version of their `pom.xml`:

```
cd orgkeep-core && mvn install            # the dashboard depends on the core
cd ../orgkeep-dashboard && mvn package -Pdocker
cd ../orgkeep-theme && docker build -t ghcr.io/orgkeep/orgkeep-keycloak:$(mvn -q help:evaluate -Dexpression=project.version -DforceStdout) .
```

Then start the compose with that version, for example:

```
ORGKEEP_VERSION=0.2.0-SNAPSHOT docker compose up -d --build
```

(`mvn package -Pdocker` in `orgkeep-core` builds the image of the API alone, `ghcr.io/orgkeep/orgkeep-core`.)

## Going further

What this demo leaves out, and a real deployment needs:
- **TLS** in front of Keycloak and Orgkeep (a reverse proxy), and `KC_HOSTNAME` / `ORGKEEP_KEYCLOAK_PUBLIC_URL` set to
  the public HTTPS URL;
- **secrets** out of the compose file: Keycloak admin, database, dashboard client secret;
- **no demo data** (`ORGKEEP_DEMO_ENABLED` unset): create the `orgkeep-dashboard` client in Keycloak yourself (see
  the README of orgkeep-dashboard), and set `ORGKEEP_OWNER_TEMPORARY_PASSWORD` to `true` (the default) so that the
  realm owner chooses a password at the first login;
- a real SMTP server (`ORGKEEP_SMTP_*`).
