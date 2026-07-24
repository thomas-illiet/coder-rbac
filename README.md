# coder-policy

`coder-policy` is a transparent, policy-enforcing reverse proxy for Coder 2.35.x.
It preserves Coder authentication and streaming behavior while admitting only
configured users and restricting workspace lifecycle and configuration changes
for admitted non-administrators.

The proxy must be the only route to `coderd` for browsers, the Coder CLI,
workspace agents, and workspace containers. It supports HTTP streaming and
upgraded connections used by WebSockets, DERP, terminals, and workspace
applications.

## Policy at a glance

`CODER_POLICY_ADMIN_USERNAMES` lists admitted administrators who bypass workspace
restrictions. `CODER_POLICY_ALLOWED_USERNAMES` lists admitted standard users, for
whom the proxy blocks password and profile updates plus workspace creation,
deletion, renaming, lifecycle settings, dormancy, provisioner state overrides,
and parameter changes. Any other authenticated username receives
`403 user_access_forbidden`. Browser navigations receive a themed access-denied
page consistent with the Code Studio gateway, while API and CLI requests retain
the stable JSON response. Its **Change account** action revokes the native Coder
session and application tokens before redirecting to the login page.

The complete and normative behavior, including response codes and parameter
rules, is documented in [Authorization policy](docs/policy.md).

> [!WARNING]
> `POST /api/v2/organizations/{organization}/members/{user}/workspaces` is
> intentionally forwarded. Coder 2.35.x marks this route as deprecated, but it
> can still create a workspace and bypass the primary creation restriction.
> This accepted risk is covered by a regression test.

Application wildcard hosts hide `/api/v2`, `/cli-auth`, and `/setup` with a
`404` response. Both `*.apps.example.com` and `*-apps.example.com` wildcard
forms are supported. Undeclared hosts receive `421`.

## Quick start

```shell
go run ./cmd/coder-policy serve \
  --upstream-url=http://127.0.0.1:3000 \
  --public-url=https://coder.example.com \
  --wildcard-access-host='*.apps.example.com' \
  --admin-usernames=platform-admin \
  --allowed-usernames=developer \
  --expected-version-prefix=v2.35.
```

The equivalent settings are available through `CODER_POLICY_*` environment
variables. See [Configuration](docs/configuration.md) for every flag, variable,
default, validation rule, and Helm mapping.

The process exposes three independent listeners by default:

| Listener | Address | Endpoints |
| --- | --- | --- |
| Proxy | `:8080` | Coder UI, API, CLI, agents, and applications |
| Health | `:8081` | `GET /healthz` and `GET /readyz` |
| Metrics | `:9090` | `GET /metrics` |

## Deployment

- [Deployment overview](docs/deployment.md)
- [Container deployment](docs/container-deployment.md)
- [Kubernetes deployment](docs/kubernetes-deployment.md)
- [Security model and hardening](docs/security.md)

Every workspace must use `CODER_POLICY_PUBLIC_URL` for `CODER_URL`, and
platform-level network controls must prevent direct access to `coderd`. An
alternate Ingress, Service, LoadBalancer, or port-forward can bypass the policy.

## Local validation

The repository includes a Coder 2.35.2 Docker Compose environment with
PostgreSQL, an automatically provisioned template, and a real workspace:

```shell
docker compose up --build -d
docker compose logs -f bootstrap
./dev/local/validate.sh
```

See [Local validation](docs/local-validation.md) for prerequisites, test
credentials, expected checks, troubleshooting, and reset instructions.

## Development

```shell
make check
```

The aggregate check covers formatting, vet, race tests, static analysis,
vulnerability scanning, ShellCheck, Markdown, Helm, and Compose validation.
See [Development](docs/development.md) for contribution rules and focused
commands.

## Documentation

The [documentation index](docs/README.md) links architecture, policy,
configuration, deployment, security, observability, operations, limitations,
local validation, and development guidance.
