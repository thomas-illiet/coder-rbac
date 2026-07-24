# Local validation with Docker Desktop

## Purpose

`compose.yaml` builds a self-contained environment that validates the proxy from
outside Coder and from a real workspace container.

| Service | Role | Host exposure |
| --- | --- | --- |
| `postgres` | Coder database | None |
| `coder` | Coder 2.35.2 | None |
| `coder-policy` | Policy proxy | `7080`, `7081`, and `7090` |
| `bootstrap` | Creates users, template, and workspace | None; exits successfully |
| `coder-admin-demo` | Workspace created by Coder | None |

## Prerequisites

- Docker Desktop is running;
- the active Docker context targets Docker Desktop;
- the Docker Compose plugin is installed;
- `curl` and `jq` are available.

```shell
docker context show
docker compose version
jq --version
```

## Start the lab

```shell
docker compose up --build -d
docker compose logs -f bootstrap
```

The first startup may take several minutes while Coder downloads Terraform
providers and the workspace image.

The `bootstrap` service must finish with `Exited (0)`. It:

1. waits for Coder;
2. creates the first administrator when necessary;
3. creates the allowed and blocked validation users;
4. pushes the `docker-local` template;
5. creates or updates the `demo` workspace.

## Access

- Coder: [http://coder.localtest.me:7080](http://coder.localtest.me:7080)
- Prometheus: [http://127.0.0.1:7090/metrics](http://127.0.0.1:7090/metrics)
- Administrator email: `admin@example.com`
- Administrator password: `CoderPolicy123!`
- Administrator username: `admin`
- Allowed restricted account: `user@example.com` / `CoderUser123!`
- Blocked account: `blocked@example.com` / `CoderBlocked123!`

These credentials are only for the local lab. The `admin` account is configured
in `CODER_POLICY_ADMIN_USERNAMES` and can create, delete, and configure
workspaces. The `user` account is admitted through
`CODER_POLICY_ALLOWED_USERNAMES` and remains subject to the complete workspace
policy. The `blocked` account can authenticate natively with Coder but every
subsequent authenticated request through the proxy is denied.

The template normally exposes its wildcard application at:

```text
http://wildcard--demo--admin.apps.localtest.me:7080
```

## Automated validation

```shell
./dev/local/validate.sh
```

The script checks:

- login through the proxy;
- an unlisted authenticated user rejected with `403 user_access_forbidden`;
- the same denied user receives the themed `403` page on browser navigation;
- a denied browser session can submit a new allowed-user login;
- workspace creation rejected with `403`;
- renaming rejected with `403`;
- TTL changes rejected with `403`;
- password and profile updates rejected for the allowed standard user;
- password and profile APIs still reachable by the administrator;
- the deprecated creation route remains unblocked;
- the workspace is absent from the control network;
- the workspace cannot resolve `coder` directly;
- the workspace has `CODER_URL=http://coder.localtest.me:7080`;
- the former internal Service host returns `421`;
- `coder list` works from inside the workspace;
- the CLI identity is resolved as `user` and its password/profile API requests
  are rejected from inside the workspace;
- `coder rename` is rejected through the internal CLI;
- a real wildcard application is reachable;
- the unlisted user is rejected on the wildcard application;
- wildcard API access is hidden with `404`;
- an unknown host is rejected with `421`;
- coder-policy metrics exist only on the dedicated listener.

Optional script variables are:

| Variable | Default |
| --- | --- |
| `CODER_POLICY_LOCAL_URL` | `http://coder.localtest.me:7080` |
| `CODER_POLICY_LOCAL_METRICS_URL` | `http://127.0.0.1:7090/metrics` |
| `CODER_POLICY_LOCAL_WILDCARD_URL` | Calculated from the Coder application |
| `CODER_POLICY_ADMIN_USERNAMES` | Value of `CODER_BOOTSTRAP_USERNAME`, normally `admin` |
| `CODER_POLICY_ALLOWED_USERNAMES` | Value of `CODER_RESTRICTED_USERNAME`, normally `user` |
| `CODER_BOOTSTRAP_EMAIL` | `admin@example.com` |
| `CODER_BOOTSTRAP_USERNAME` | `admin` |
| `CODER_BOOTSTRAP_PASSWORD` | `CoderPolicy123!` |
| `CODER_BLOCKED_EMAIL` | `blocked@example.com` |
| `CODER_BLOCKED_USERNAME` | `blocked` |
| `CODER_BLOCKED_PASSWORD` | `CoderBlocked123!` |

## Network isolation

```shell
docker inspect coder-admin-demo \
  --format '{{json .NetworkSettings.Networks}}' | jq
```

The result must contain `coder-policy-workspace` and must not contain
`coder-policy-control`.

Direct-resolution test:

```shell
docker exec coder-admin-demo sh -lc 'getent hosts coder'
```

This command must fail.

## Manual UI tests

After signing in as `user@example.com`:

1. open the `demo` workspace;
2. open the `Wildcard validation` application;
3. try to rename the workspace;
4. try to change automatic shutdown;
5. start and stop the workspace;
6. verify that changing an established persistent or ephemeral parameter is
   rejected;
7. verify that an upgrade without a persistent-schema or value change works.

Then sign in as `blocked@example.com` and verify that the native login completes
but the first authenticated browser navigation displays the Code Studio-themed
`403` access-denied page. An API request from the same session must still return
the JSON code `user_access_forbidden`.

Blocked workspace and account actions must receive a JSON policy response, not
an HTML redirect generated by the proxy.

## Logs and state

```shell
docker compose ps -a
docker compose logs --since=10m coder-policy coder
docker ps --filter label=coder.workspace_name=demo
curl -fsS http://127.0.0.1:7081/readyz | jq
```

## Rebuild only the proxy

```shell
docker compose up --build -d --no-deps coder-policy
./dev/local/validate.sh
```

This avoids restarting bootstrap and republishing the template.

## Reset

Terraform, not Compose, creates the workspace container and volume. Remove them
before removing Compose volumes:

```shell
workspace_id=$(docker inspect coder-admin-demo \
  --format '{{index .Config.Labels "coder.workspace_id"}}')
docker rm -f coder-admin-demo
docker volume rm "coder-${workspace_id}-home"
docker compose down --volumes --remove-orphans
```

The final command removes PostgreSQL data and resources managed directly by
Compose. If the workspace no longer exists, skip the two workspace-specific
commands.

## Differences from production

- HTTP instead of TLS;
- secure cookies disabled;
- Docker networks instead of Kubernetes NetworkPolicies;
- the Docker socket mounted in Coder for local provisioning;
- known credentials;
- a single instance of each service;
- no high availability.

A successful local validation does not replace testing from a real workspace
pod in the target cluster.
