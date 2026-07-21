# Architecture

## Purpose

`coder-policy` is a transparent HTTP reverse proxy between clients and `coderd`.
It preserves Coder authentication, rejects authenticated usernames outside the
configured access lists, and denies selected workspace-management operations
for admitted non-administrators.

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
    Workspace["Workspace, agent, and internal CLI"] --> InternalService["Internal coder-policy Service"]

    MainIngress --> Proxy["coder-policy :8080"]
    WildcardIngress --> Proxy
    InternalService --> Proxy

    Proxy --> Gate["Coder 2.35.x version gate"]
    Gate --> Host["Host classification"]
    Host --> Admission["User admission"]
    Admission --> Policy["Workspace policy"]
    Policy --> Coder["coderd"]
    Coder --> Database["PostgreSQL"]

    Prometheus["Prometheus"] --> Metrics["coder-policy :9090 /metrics"]
    Kubernetes["Kubernetes probes"] --> Health["coder-policy :8081"]
```

## Access paths

### Main public URL

The main Ingress sends the URL configured by `CODER_POLICY_PUBLIC_URL` to the
`coder-policy` Service. Native authentication and public asset routes remain
reachable so a denied session can switch accounts. Every other request is
checked for a Coder user identity. Anonymous requests are forwarded so Coder
can serve its login flow. Verified users must be in the administrator or
allowed list before forwarding.

### Application wildcard

The wildcard Ingress targets the same Service. The proxy preserves the original
`Host` header so Coder can resolve the application and workspace represented by
the subdomain.

The following paths are hidden on wildcard hosts before they reach Coder:

- `/api/v2` and its descendants;
- `/cli-auth` and its descendants;
- `/setup` and its descendants.

They receive `404 Not Found`. Other paths use the same admission check before
they are forwarded to Coder.

### Workspaces and internal CLI traffic

Every workspace must receive:

```text
CODER_URL=http://coder-policy.<namespace>.svc.cluster.local:8080
```

Workspace pods must not be able to reach the `coderd` Service directly. The
Helm chart provides baseline NetworkPolicies, but their selectors must match the
labels used by the actual Coder deployment and workspace templates.

## Request processing

```mermaid
sequenceDiagram
    participant Client
    participant Proxy as coder-policy
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
        else Main, internal, or application request
            Proxy->>API: GET /api/v2/users/me
            alt Request is anonymous or a non-user service flow
                API-->>Proxy: 401 or 403
                Proxy->>API: Forward request for native Coder handling
            else Identity lookup fails
                Proxy-->>Client: 503 identity_unavailable
            else Username is not configured
                API-->>Proxy: Canonical username
                Proxy-->>Client: 403 user_access_forbidden
            else Username is an administrator
                API-->>Proxy: Canonical username
                Proxy->>API: Forward request without workspace restrictions
            else Username is an allowed standard user
                alt Workspace route requires policy data
                    Proxy->>API: Read workspace, build, schema, or preset
                    API-->>Proxy: Policy data
                    Proxy->>Proxy: Make a fail-closed decision
                else Unrestricted route
                    Proxy->>API: Forward request
                end
            end
        end
    end
```

The processing order is:

1. check version-gate readiness;
2. repeatedly decode and normalize the path;
3. classify `Host` as `main`, `internal`, `wildcard`, or `unknown`;
4. hide control-plane paths on wildcard hosts;
5. forward native authentication and public asset routes on main and internal hosts;
6. resolve every other request identity through `/api/v2/users/me`;
7. forward anonymous or non-user service traffic for native Coder handling;
8. reject authenticated usernames absent from both access lists;
9. apply the administrator exemption or the standard-user workspace policy;
10. inspect JSON and load additional Coder data when required;
11. forward the request through `httputil.ReverseProxy`.

## HTTP reverse proxy

The proxy uses Go's `Rewrite` API:

- the destination becomes `CODER_POLICY_UPSTREAM_URL`;
- the incoming `Host` is preserved;
- `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` are rebuilt;
- immediate flushing supports streaming responses;
- HTTP upgrades used by WebSockets are relayed;
- transport errors become JSON `502 upstream_unavailable` responses.

## Version gate

At startup, the proxy requests `/api/v2/buildinfo`. The reported version must
start with `CODER_POLICY_EXPECTED_VERSION_PREFIX`, such as `v2.35.`.

- An initial check failure prevents the servers from starting.
- The version is checked again at the configured interval.
- A network error or incompatible version makes readiness fail.
- While the gate is not ready, the proxy listener returns `503` for every
  request.
- `/healthz` remains available to distinguish a live process from a ready proxy.

## Authentication

The proxy has no user database. Outside the native-authentication and public
asset routes, it calls `/api/v2/users/me` and forwards only these authentication
headers:

- `Authorization`;
- `Coder-Session-Token`;
- `Cookie`.

Coder `401` and `403` responses mean no user identity was resolved. The original
request is forwarded so login, agent, and other native Coder flows continue;
protected workspace mutations still receive `authentication_required`. Any
other identity lookup error fails closed with `503`.

Username matching is exact and case-sensitive against the canonical `username`
returned by Coder. Administrators are the union's privileged subset; allowed
users remain restricted. The same configuration is portable across Coder
instances only when each username maps to the intended account.

## Network boundaries

### Docker Compose

The local lab uses separate control and workspace networks:

```mermaid
flowchart TB
    subgraph Control["coder-policy-control"]
        PostgreSQL
        Coder
        Bootstrap
        ProxyControl["coder-policy"]
    end
    subgraph WorkspaceNet["coder-policy-workspace"]
        WorkspaceDemo["demo workspace"]
        ProxyWorkspace["coder-policy"]
    end
    ProxyControl --- ProxyWorkspace
```

The workspace cannot resolve the `coder` service. Only `coder-policy` is attached
to both networks.

### Kubernetes

Kubernetes isolation requires two complementary directions:

- workspace egress permits cluster DNS and the `coder-policy` proxy port;
- `coderd` ingress permits `coder-policy` and explicitly required components.

NetworkPolicies are additive. Another permissive policy can invalidate the
intended boundary.
