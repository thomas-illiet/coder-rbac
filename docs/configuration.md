# Configuration

## Sources and precedence

The `serve` command uses Cobra and Viper. Precedence is:

```text
CLI flags > environment variables > defaults
```

Configuration is loaded and validated once at startup. There is no dynamic
reload from a file, ConfigMap, or Secret. A change requires a restart or
redeployment.

Environment variables use the `CODER_RBAC_` prefix. Flag names use hyphens;
their environment equivalents use underscores.

## Complete reference

| Environment variable | Flag | Type | Default | Required | Validation |
| --- | --- | --- | --- | --- | --- |
| `CODER_RBAC_UPSTREAM_URL` | `--upstream-url` | URL | — | Yes | Absolute `http` or `https` URL without credentials, query, fragment, or path |
| `CODER_RBAC_PUBLIC_URL` | `--public-url` | URL | — | Yes | Same validation; must be the public URL served by the proxy |
| `CODER_RBAC_WILDCARD_ACCESS_HOST` | `--wildcard-access-host` | host | — | Yes | `*.example.com` form without a scheme or port |
| `CODER_RBAC_INTERNAL_HOSTS` | `--internal-hosts` | CSV | — | Yes | At least one host; `/`, `?`, `*`, and `#` are forbidden |
| `CODER_RBAC_ADMIN_USERNAMES` | `--admin-usernames` | username CSV | empty | No | Admitted administrators; exact, case-sensitive canonical usernames |
| `CODER_RBAC_ALLOWED_USERNAMES` | `--allowed-usernames` | username CSV | empty | No | Admitted standard users; exact, case-sensitive canonical usernames |
| `CODER_RBAC_EXPECTED_VERSION_PREFIX` | `--expected-version-prefix` | string | — | Yes | Non-empty and starts with `v` |
| `CODER_RBAC_LISTEN_ADDRESS` | `--listen-address` | host:port | `:8080` | No | Valid address with a port distinct from the other listeners |
| `CODER_RBAC_HEALTH_LISTEN_ADDRESS` | `--health-listen-address` | host:port | `:8081` | No | Valid address with a distinct port |
| `CODER_RBAC_METRICS_LISTEN_ADDRESS` | `--metrics-listen-address` | host:port | `:9090` | No | Valid address with a distinct port |
| `CODER_RBAC_VERSION_CHECK_INTERVAL` | `--version-check-interval` | Go duration | `1m` | No | Greater than zero |
| `CODER_RBAC_UPSTREAM_TIMEOUT` | `--upstream-timeout` | Go duration | `15s` | No | Greater than zero |
| `CODER_RBAC_SHUTDOWN_TIMEOUT` | `--shutdown-timeout` | Go duration | `15s` | No | Greater than zero |
| `CODER_RBAC_MAX_INSPECTION_BODY_BYTES` | `--max-inspection-body-bytes` | integer | `1048576` | No | Greater than zero |

Durations use Go syntax, for example `500ms`, `15s`, `1m`, or `2h`.

## Environment example

```shell
export CODER_RBAC_UPSTREAM_URL=http://coder.coder.svc.cluster.local:80
export CODER_RBAC_PUBLIC_URL=https://coder.example.com
export CODER_RBAC_WILDCARD_ACCESS_HOST='*.apps.example.com'
export CODER_RBAC_INTERNAL_HOSTS='coder-rbac,coder-rbac.coder.svc.cluster.local'
export CODER_RBAC_ADMIN_USERNAMES='platform-admin,automation'
export CODER_RBAC_ALLOWED_USERNAMES='developer,contractor'
export CODER_RBAC_EXPECTED_VERSION_PREFIX='v2.35.'
export CODER_RBAC_LISTEN_ADDRESS=':8080'
export CODER_RBAC_HEALTH_LISTEN_ADDRESS=':8081'
export CODER_RBAC_METRICS_LISTEN_ADDRESS=':9090'

coder-rbac serve
```

## Flag example

```shell
coder-rbac serve \
  --upstream-url=http://coder.coder.svc.cluster.local:80 \
  --public-url=https://coder.example.com \
  --wildcard-access-host='*.apps.example.com' \
  --internal-hosts='coder-rbac,coder-rbac.coder.svc.cluster.local' \
  --admin-usernames='platform-admin,automation' \
  --allowed-usernames='developer,contractor' \
  --expected-version-prefix='v2.35.' \
  --listen-address=:8080 \
  --health-listen-address=:8081 \
  --metrics-listen-address=:9090
```

## Related Coder URLs

Coder and proxy settings must remain consistent:

| Location | Expected value |
| --- | --- |
| Coder `CODER_ACCESS_URL` | Same URL as `CODER_RBAC_PUBLIC_URL` |
| Coder `CODER_WILDCARD_ACCESS_URL` | Same host as `CODER_RBAC_WILDCARD_ACCESS_HOST` |
| Workspace `CODER_URL` | Internal `coder-rbac` Service URL |
| Proxy `CODER_RBAC_UPSTREAM_URL` | Direct internal `coderd` URL, unreachable from workspaces |

For example:

```text
CODER_ACCESS_URL=https://coder.example.com
CODER_WILDCARD_ACCESS_URL=*.apps.example.com
CODER_URL=http://coder-rbac.coder.svc.cluster.local:8080
CODER_RBAC_UPSTREAM_URL=http://coder.coder.svc.cluster.local:80
```

## Administrator and allowed usernames

Both lists accept canonical Coder usernames, not UUIDs or email addresses:

```text
CODER_RBAC_ADMIN_USERNAMES=platform-admin,automation
CODER_RBAC_ALLOWED_USERNAMES=developer,contractor
```

Entries are trimmed and deduplicated, then compared exactly and
case-sensitively with the canonical `username` returned by
`/api/v2/users/me`. Administrators are implicitly admitted and bypass all
workspace restrictions. Allowed users may access Coder but remain subject to
the workspace policy. An authenticated username in neither list receives
`403 user_access_forbidden`. If the allowed list is empty, only administrators
may access Coder. The Helm chart stores both lists in a Kubernetes `Secret`.

To inspect the canonical username of the authenticated user:

```shell
coder users show me
```

CLI output varies by version. Copy the username exactly as Coder reports it.
The same list can be reused across Coder instances only when each username maps
to the intended account on every instance.

Usernames are mutable identifiers. Renaming a configured user removes its
access or administrator exemption, while reusing that username grants the same
capability to the replacement account. Control username rename and reuse, and
audit username-to-user-ID mappings on every managed Coder instance.

### Migration from allowed user IDs

Username lists replace UUID allowlists. Migrate the former privileged list from
`CODER_RBAC_ALLOWED_USER_IDS`, `--allowed-user-ids`, or
`config.allowedUserIDs` to `CODER_RBAC_ADMIN_USERNAMES`,
`--admin-usernames`, or `config.adminUsernames`, replacing every UUID with the
canonical Coder username used on each managed instance.

The binary and Helm chart reject the legacy ID-based names instead of silently
ignoring them. For upgrades from the previous username-based release, move the
old privileged `CODER_RBAC_ALLOWED_USERNAMES` value to
`CODER_RBAC_ADMIN_USERNAMES`, then populate `CODER_RBAC_ALLOWED_USERNAMES` with
the standard users who may access Coder. Both defaults are empty, so omitting
both lists admits no authenticated user.

## Expected version

The compatibility check is a prefix comparison, not a semantic-version range:

```text
v2.35.  accepts v2.35.0, v2.35.2, and their build metadata
v2.35.2 accepts only versions beginning exactly with v2.35.2
```

A more specific prefix reduces compatibility drift but requires more frequent
redeployments.

## Timeouts and body sizes

`CODER_RBAC_UPSTREAM_TIMEOUT` applies to auxiliary policy calls for identity,
workspace data, parameter schemas, presets, and the initial build-information
check. The streaming reverse proxy has no equivalent global timeout, so it does
not interrupt WebSockets, agents, or long-lived streams.

`CODER_RBAC_MAX_INSPECTION_BODY_BYTES` applies only to protected request bodies
that require inspection. The body is restored before forwarding to Coder.

Coder API JSON responses used for policy decisions are independently limited to
4 MiB and fail closed if they exceed that bound or contain additional JSON.

## CLI commands

```shell
coder-rbac serve
coder-rbac version
coder-rbac completion bash
coder-rbac completion zsh
coder-rbac completion fish
coder-rbac completion powershell
```

`version` prints the version, commit, and build date injected at build time.

## Helm mapping

| Helm value | Generated configuration |
| --- | --- |
| `config.upstreamURL` | `CODER_RBAC_UPSTREAM_URL` |
| `config.publicURL` | `CODER_RBAC_PUBLIC_URL` |
| `config.wildcardAccessHost` | `CODER_RBAC_WILDCARD_ACCESS_HOST` |
| `config.internalHosts` | Added to calculated internal Service names |
| `config.adminUsernames` | `CODER_RBAC_ADMIN_USERNAMES` in a Secret |
| `config.allowedUsernames` | `CODER_RBAC_ALLOWED_USERNAMES` in a Secret |
| `config.expectedVersionPrefix` | `CODER_RBAC_EXPECTED_VERSION_PREFIX` |
| `config.versionCheckInterval` | `CODER_RBAC_VERSION_CHECK_INTERVAL` |
| `config.upstreamTimeout` | `CODER_RBAC_UPSTREAM_TIMEOUT` |
| `config.shutdownTimeout` | `CODER_RBAC_SHUTDOWN_TIMEOUT` |
| `config.maxInspectionBodyBytes` | `CODER_RBAC_MAX_INSPECTION_BODY_BYTES` |
| `service.proxyPort` | Proxy listener and Service port `http` |
| `service.healthPort` | Health listener and Service port `health` |
| `service.metricsPort` | Prometheus listener and Service port `metrics` |

The default values and descriptions are also documented directly in
[`values.yaml`](../charts/coder-rbac/values.yaml).
