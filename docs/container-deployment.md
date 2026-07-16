# Container deployment

This guide covers the OCI image, a standalone container, and the repository's
Docker Compose validation environment. For production Kubernetes, use the
[Kubernetes deployment guide](kubernetes-deployment.md).

## Build the OCI image

The multi-stage Dockerfile builds a static Go binary and copies it into a
non-root distroless image.

```shell
docker build \
  --build-arg VERSION=v0.1.0 \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t registry.example.com/platform/coder-rbac:v0.1.0 .
```

The image conventionally exposes ports `8080`, `8081`, and `9090`, runs as the
distroless `nonroot` user, and requires no writable filesystem.

## Run a standalone container

```shell
docker run --rm \
  -p 8080:8080 \
  -p 127.0.0.1:8081:8081 \
  -p 127.0.0.1:9090:9090 \
  -e CODER_RBAC_UPSTREAM_URL=http://host.docker.internal:3000 \
  -e CODER_RBAC_PUBLIC_URL=http://coder.localhost:8080 \
  -e 'CODER_RBAC_WILDCARD_ACCESS_HOST=*.apps.localhost' \
  -e CODER_RBAC_INTERNAL_HOSTS=coder-rbac \
  -e CODER_RBAC_ALLOWED_USERNAMES=platform-admin \
  -e CODER_RBAC_EXPECTED_VERSION_PREFIX=v2.35. \
  registry.example.com/platform/coder-rbac:v0.1.0
```

Bind the health and metrics ports only to trusted interfaces. This example uses
`127.0.0.1`; the proxy port remains available on all interfaces.

The container still requires a network design that prevents workspaces from
reaching `coderd` directly. A shared unrestricted container network does not
provide that boundary.

## Docker Compose lab

The repository includes a complete local environment:

```shell
docker compose up --build -d
docker compose logs -f bootstrap
./dev/local/validate.sh
```

The lab separates `coderd` and the workspace into control and workspace
networks. `coder-rbac` is the only service attached to both. It is intended for
functional validation, not as a production topology.

See [Local validation](local-validation.md) for prerequisites, credentials,
manual tests, network checks, and the safe reset procedure.

## Runtime hardening

Keep the image defaults when adapting the deployment:

- run as a non-root user;
- drop Linux capabilities;
- prevent privilege escalation;
- use a read-only root filesystem;
- provide no Docker socket or host namespace access;
- keep the metrics listener private;
- use TLS at the external reverse proxy or load balancer;
- terminate the process with SIGTERM and allow at least the configured shutdown
  timeout for graceful connection draining.

## Post-deployment checks

```shell
curl -fsS http://127.0.0.1:8081/healthz
curl -fsS http://127.0.0.1:8081/readyz
curl -fsS http://127.0.0.1:9090/metrics \
  | grep '^coder_rbac_coder_version_ready'
```

Also test an upgraded connection, a wildcard application, an allowed workspace
operation, and a blocked operation. Run the direct-`coderd` isolation check from
the actual workspace network, not from the container host.
