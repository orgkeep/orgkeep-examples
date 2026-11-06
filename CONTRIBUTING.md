# Contributing to Orgkeep

Thank you for your interest in Orgkeep. It is a personal project, maintained in spare time: a review can take a few
days.

## Before you start

- **Small changes** (a typo, the docs, an obvious bug): open a pull request directly.
- **Larger changes** (a new example API, a new service in the demo stack): open an issue first, to agree on the
  approach before you spend time on it.
- **Security vulnerabilities**: never in a public issue. Use *Report a vulnerability* in the *Security* tab of the
  repository, which keeps the report private.

## Sign off your commits (DCO)

Orgkeep uses the [Developer Certificate of Origin](https://developercertificate.org) (DCO) instead of a contributor
license agreement. By signing off a commit, you certify that you wrote it, or that you have the right to submit it
under the license of the project.

Sign off each commit with `-s`:

```
git commit -s -m "Read the team pool in the Vert.x example"
```

Git adds a line with the name and email of your Git configuration, which must match the author of the commit:

```
Signed-off-by: Jane Doe <jane@example.com>
```

A check fails on the pull request if a commit is not signed off. To fix the last commit, `git commit --amend -s`; for
all the commits of your branch, `git rebase --signoff main`, then push your branch again with `--force-with-lease`.

Contributions are licensed under the [Apache License 2.0](LICENSE), like the rest of the project (section 5 of the
license).

## Pull requests

- One change per pull request, on a branch created from `main`. Pull requests are squash merged.
- The example builds and its tests pass (`mvn package` in its folder, `go test ./...` for the gateway), and the demo
  stack still starts (`docker compose up -d --build` in `docker-compose`).
- Follow the style of the surrounding code. Comments are in English.
- A commit message starts with a short summary line, followed by why the change is needed when it is not obvious.
- No IDE files or build output in the commits.

## Where things go

- `docker-compose`: the demo stack (Keycloak, PostgreSQL, Orgkeep, Mailpit and the demo APIs).
- One folder per example (`greetings-api-spring`, `greetings-api-vertx`, `gateway-go`), each a complete project that
  builds on its own, with its own README.

The examples are documentation for integrators: each one shows the shortest code that does the job in its framework,
and its README explains the choices. A new example implements the same small greetings API, so that they can be
compared.
