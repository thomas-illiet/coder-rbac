# Architecture

## Purpose

`coder-rbac` is a transparent HTTP reverse proxy between clients and `coderd`.
It preserves Coder authentication while denying selected workspace-management
operations for users who are not in the configured username allowlist.

The process exposes three independent listeners:

| Listener | Default | Purpose |
| --- | --- | --- |
| Proxy | `:8080` | UI, API, CLI, agents, WebSockets, DERP, and wildcard applications |
| Health | `:8081` | `GET /healthz` and `GET /readyz` |
| Prometheus | `:9090` | `GET /metrics` only |

All three listeners must use different ports.

## System overview

```mermaid
flowchart LR
    Browser["Browser and external CLI"] --> MainIngress["Main Ingress"]
    AppClient["Workspace application client"] --> WildcardIngress["Wildcard Ingress"]
    Workspace["Workspace, agent, and internal CLI"] --> InternalService["Internal coder-rbac Service"]

    MainIngress --> Proxy["coder-rbac :8080"]
    WildcardIngress --> Proxy
    InternalService --> Proxy

    Proxy --> Gate["Coder 2.35.x version gate"]
    Gate --> Host["Host classification"]
    Host --> Policy["Path normalization and policy"]
    Policy --> Coder["coderd"]
    Coder --> Database["PostgreSQL"]

    Prometheus["Prometheus"] --> Metrics["coder-rbac :9090 /metrics"]
    Kubernetes["Kubernetes probes"] --> Health["coder-rbac :8081"]
```

## Access paths

### Main public URL

The main Ingress sends the URL configured by `CODER_RBAC_PUBLIC_URL` to the
`coder-rbac` Service. Protected routes pass through policy evaluation. Other
routes are forwarded without an additional `/api/v2/users/me` request.

### Application wildcard

The wildcard Ingress targets the same Service. The proxy preserves the original
`Host` header so Coder can resolve the application and workspace represented by
the subdomain.

The following paths are hidden on wildcard hosts before they reach Coder:

- `/api/v2` and its descendants;
- `/cli-auth` and its descendants;
- `/setup` and its descendants.

They receive `404 Not Found`. Other paths are forwarded to Coder.

### Workspaces and internal CLI traffic

Every workspace must receive:

```text
CODER_URL=http://coder-rbac.<namespace>.svc.cluster.local:8080
```

Workspace pods must not be able to reach the `coderd` Service directly. The
Helm chart provides baseline NetworkPolicies, but their selectors must match the
labels used by the actual Coder deployment and workspace templates.

## Request processing

```mermaid
sequenceDiagram
    participant Client
    participant Proxy as coder-rbac
    participant Gate as Version gate
    participant API as Coder API

    Client->>Proxy: HTTP request
    Proxy->>Gate: Check compatibility state
    alt Gate is not ready
        Proxy-->>Client: 503 coder_version_unavailable
    else Gate is ready
        Proxy->>Proxy: Decode and normalize path
        Proxy->>Proxy: Classify Host
        alt Host is unknown
            Proxy-->>Client: 421 unknown_host
        else Wildcard control path
            Proxy-->>Client: 404
        else Protected route
            Proxy->>API: GET /api/v2/users/me
            API-->>Proxy: Canonical username
            alt Username is in the allowlist
                Proxy->>API: Forward request
            else Inspection is required
                Proxy->>API: Read workspace, build, schema, or preset
                API-->>Proxy: Policy data
                Proxy->>Proxy: Make a fail-closed decision
            end
        else Unprotected route
            Proxy->>API: Forward directly
        end
    end
```

The processing order is:

1. check version-gate readiness;
2. repeatedly decode and normalize the path;
3. classify `Host` as `main`, `internal`, `wildcard`, or `unknown`;
4. hide control-plane paths on wildcard hosts;
5. classify the method and route on main and internal hosts;
6. identify the user only for a protected route;
7. apply the allowlisted-username exemption, when present;
8. inspect JSON and load additional Coder data when required;
9. forward the request through `httputil.ReverseProxy`.

## HTTP reverse proxy

The proxy uses Go's `Rewrite` API:

- the destination becomes `CODER_RBAC_UPSTREAM_URL`;
- the incoming `Host` is preserved;
- `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` are rebuilt;
- immediate flushing supports streaming responses;
- HTTP upgrades used by WebSockets are relayed;
- transport errors become JSON `502 upstream_unavailable` responses.

## Version gate

At startup, the proxy requests `/api/v2/buildinfo`. The reported version must
start with `CODER_RBAC_EXPECTED_VERSION_PREFIX`, such as `v2.35.`.

- An initial check failure prevents the servers from starting.
- The version is checked again at the configured interval.
- A network error or incompatible version makes readiness fail.
- While the gate is not ready, the proxy listener returns `503` for every
  request.
- `/healthz` remains available to distinguish a live process from a ready proxy.

## Authentication

The proxy has no user database. For a protected route, it calls
`/api/v2/users/me` and forwards only these authentication headers:

- `Authorization`;
- `Coder-Session-Token`;
- `Cookie`.

Coder `401` and `403` responses become an authentication-required rejection.
Any other identity lookup error fails closed with `503`.

Username matching is exact and case-sensitive against the canonical `username`
returned by Coder. The same configuration is portable across Coder instances
only when that username maps to the intended account on every instance.

## Network boundaries

### Docker Compose

The local lab uses separate control and workspace networks:

```mermaid
flowchart TB
    subgraph Control["coder-rbac-control"]
        PostgreSQL
        Coder
        Bootstrap
        ProxyControl["coder-rbac"]
    end
    subgraph WorkspaceNet["coder-rbac-workspace"]
        WorkspaceDemo["demo workspace"]
        ProxyWorkspace["coder-rbac"]
    end
    ProxyControl --- ProxyWorkspace
```

The workspace cannot resolve the `coder` service. Only `coder-rbac` is attached
to both networks.

### Kubernetes

Kubernetes isolation requires two complementary directions:

- workspace egress permits cluster DNS and the `coder-rbac` proxy port;
- `coderd` ingress permits `coder-rbac` and explicitly required components.

NetworkPolicies are additive. Another permissive policy can invalidate the
intended boundary.
