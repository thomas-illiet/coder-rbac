# Local validation with Docker Desktop

## Purpose

`compose.yaml` builds a self-contained environment that validates the proxy from
outside Coder and from a real workspace container.

| Service | Role | Host exposure |
| --- | --- | --- |
| `postgres` | Coder database | None |
| `coder` | Coder 2.35.2 | None |
| `coder-rbac` | Policy proxy | `7080`, `7081`, and `7090` |
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
3. pushes the `docker-local` template;
4. creates or updates the `demo` workspace.

## Access

- Coder: [http://coder.localhost:7080](http://coder.localhost:7080)
- Prometheus: [http://127.0.0.1:7090/metrics](http://127.0.0.1:7090/metrics)
- Administrator email: `admin@example.com`
- Administrator password: `CoderRbac123!`
- Allowlisted administrator username: `admin`
- Restricted validation account: `user@example.com` / `CoderUser123!`

These credentials are only for the local lab. The `admin` account is allowlisted
and can create, delete, and configure workspaces. Automated restriction checks
use the `user` account, which remains subject to the complete policy.

The template normally exposes its wildcard application at:

```text
http://wildcard--demo--admin.apps.localhost:7080
```

## Automated validation

```shell
./dev/local/validate.sh
```

The script checks:

- login through the proxy;
- workspace creation rejected with `403`;
- renaming rejected with `403`;
- TTL changes rejected with `403`;
- the deprecated creation route remains unblocked;
- the workspace is absent from the control network;
- the workspace cannot resolve `coder` directly;
- the workspace has `CODER_URL=http://coder-rbac:8080`;
- `coder list` works from inside the workspace;
- `coder rename` is rejected through the internal CLI;
- a real wildcard application is reachable;
- wildcard API access is hidden with `404`;
- an unknown host is rejected with `421`;
- coder-rbac metrics exist only on the dedicated listener.

Optional script variables are:

| Variable | Default |
| --- | --- |
| `CODER_RBAC_LOCAL_URL` | `http://coder.localhost:7080` |
| `CODER_RBAC_LOCAL_METRICS_URL` | `http://127.0.0.1:7090/metrics` |
| `CODER_RBAC_LOCAL_WILDCARD_URL` | Calculated from the Coder application |
| `CODER_RBAC_ALLOWED_USERNAMES` | Value of `CODER_BOOTSTRAP_USERNAME`, normally `admin` |
| `CODER_BOOTSTRAP_EMAIL` | `admin@example.com` |
| `CODER_BOOTSTRAP_USERNAME` | `admin` |
| `CODER_BOOTSTRAP_PASSWORD` | `CoderRbac123!` |

## Network isolation

```shell
docker inspect coder-admin-demo \
  --format '{{json .NetworkSettings.Networks}}' | jq
```

The result must contain `coder-rbac-workspace` and must not contain
`coder-rbac-control`.

Direct-resolution test:

```shell
docker exec coder-admin-demo sh -lc 'getent hosts coder'
```

This command must fail.

## Manual UI tests

After signing in:

1. open the `demo` workspace;
2. open the `Wildcard validation` application;
3. try to rename the workspace;
4. try to change automatic shutdown;
5. start and stop the workspace;
6. verify that changing an established persistent or ephemeral parameter is
   rejected;
7. verify that an upgrade without a persistent-schema or value change works.

Blocked actions must receive a policy response, not an HTML redirect generated
by the proxy.

## Logs and state

```shell
docker compose ps -a
docker compose logs --since=10m coder-rbac coder
docker ps --filter label=coder.workspace_name=demo
curl -fsS http://127.0.0.1:7081/readyz | jq
```

## Rebuild only the proxy

```shell
docker compose up --build -d --no-deps coder-rbac
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
