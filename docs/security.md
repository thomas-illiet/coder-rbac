# Security

## Security property

The product is intended to prevent a Coder user whose canonical username is not
in the allowlist from performing selected workspace mutations, including
through the Coder CLI inside that user's workspace container.

This property depends on all of the following controls:

1. the proxy's HTTP authorization policy;
2. network enforcement that prevents direct access to `coderd`;
3. compatibility with the deployed Coder version;
4. controlled allowlisted-username and Kubernetes configuration.

## Threat model

### Untrusted actors

- a user authenticated by Coder;
- a process running in a workspace;
- compromised template code or a workspace application;
- a client that forges methods, encoded paths, headers, and JSON bodies.

### Trusted components

- the `coder-rbac` binary and image;
- `coderd` and its database;
- the Ingress controller and TLS termination;
- administrators who can change Helm values, Secrets, NetworkPolicies, and
  templates;
- provisioners and operators explicitly allowed to reach Coder.

### Addressed threats

- direct protected-route calls from the UI or CLI;
- use of a valid Coder cookie or token from a workspace;
- repeated encoding and ambiguous path segments;
- changes to established persistent or ephemeral parameter values during a
  start or upgrade;
- addition or removal of a persistent parameter in a target template version;
- access to the main API through an application wildcard subdomain;
- a future, unknown build transition;
- detectable Coder API compatibility drift.

### Out-of-scope threats

- compromise of `coderd`, PostgreSQL, or the cluster;
- a malicious Kubernetes administrator;
- a user whose canonical username is in the allowlist;
- an unclassified Coder route that provides an equivalent mutation;
- direct `coderd` access enabled by another network exposure;
- security of workspace content and applications themselves.

## Trust boundaries

```mermaid
flowchart LR
    Untrusted["Untrusted clients and workspaces"] --> TLS["Ingress and TLS"]
    TLS --> Proxy["coder-rbac boundary"]
    Proxy --> Trusted["Trusted coderd and PostgreSQL network"]
    Admin["Privileged configuration"] --> Proxy
    Admin --> Network["Ingress, Services, and NetworkPolicies"]
```

The proxy is not an effective boundary if untrusted clients have another path
to the trusted network.

## Fail-closed behavior

The proxy rejects a protected operation instead of forwarding it when:

- the version gate is not ready;
- `/api/v2/users/me` cannot identify the user;
- the workspace or latest build cannot be read;
- the current or target parameter schema is unavailable;
- current values or presets cannot be loaded;
- a submitted parameter requires a current value that is unavailable;
- a Coder policy response is malformed, contains extra JSON, or exceeds 4 MiB.

Unprotected routes do not require those auxiliary lookups, but they are still
blocked while the global version gate is not ready.

See [Authorization policy](policy.md#workspace-parameters) for the narrowly
allowed first value of an ephemeral parameter introduced by a target version.

## Authentication and headers

Coder remains responsible for authentication. Auxiliary API calls copy only
`Authorization`, `Coder-Session-Token`, and `Cookie`.

The reverse proxy rebuilds `X-Forwarded-*` headers through Go's
`ProxyRequest.SetXForwarded`. It preserves the original `Host` for wildcard
routing. The Ingress must not use attacker-controlled headers to choose a route
that bypasses the proxy.

## TLS

In production:

- use HTTPS for the main and wildcard URLs;
- restrict cleartext traffic to a controlled internal network;
- protect certificates and automate renewal;
- test TLS through a real application subdomain;
- enable the relevant security controls on the Ingress controller.

The Compose lab uses HTTP and disables secure cookies only for local validation.

## Network isolation

### Forbidden exposures

- a direct Ingress to the Coder Service;
- a Coder `LoadBalancer` or `NodePort` Service reachable by users;
- a permanent port-forward distributed as an access method;
- unrestricted workspace egress to the Coder namespace;
- an additive allow-all NetworkPolicy selecting the same pods;
- alternate DNS that resolves to a direct Coder endpoint.

### Required evidence

Run these checks from a real workspace:

```shell
coder list                         # Succeeds through coder-rbac.
getent hosts coder-rbac            # Succeeds.
getent hosts coder                 # Fails or resolves to an unreachable target.
curl --connect-timeout 3 http://coder.coder.svc.cluster.local  # Fails at the network layer.
```

YAML inspection is not sufficient evidence. Test from a pod that is actually
selected by the NetworkPolicy.

## Allowlisted usernames

An allowlist entry bypasses the complete policy. Recommended controls are:

- use the exact canonical username returned by Coder;
- verify that the username maps to the intended account on every managed Coder
  instance;
- audit the corresponding immutable user ID on every instance;
- control username renames and reuse because usernames are mutable;
- require two-person review;
- audit Secret changes;
- remove obsolete accounts immediately;
- prefer a dedicated service identity when a human identity is unnecessary;
- monitor privileged protected operations in Coder logs.

Matching is exact and case-sensitive. A username rename removes the exemption;
reuse of an allowlisted username grants the exemption to the replacement
account. Portability across Coder instances therefore depends on consistent
identity governance, not only on using the same configuration value.

## Prometheus endpoint

`/metrics` has no application-level authentication. Keep the listener on a
private observability network:

- Compose publishes it on `127.0.0.1`;
- Kubernetes should restrict it through NetworkPolicy or scraper configuration;
- never create a public Ingress for the `metrics` port.

Metric labels use bounded sets to prevent user-controlled cardinality. Metrics
contain no cookies or tokens.

## Secrets and logs

The proxy needs no Coder service token; it reuses the request identity. The Helm
chart stores allowlisted usernames in a Secret even though usernames are not
cryptographic secrets.

JSON logs include infrastructure errors and limited request metadata. Internal
details for a `5xx` policy decision are logged server-side and removed from the
client response. Never add cookies, tokens, or complete request bodies to logs.

## Hardening checklist

- [ ] The image is built from an identified commit and scanned.
- [ ] The container is non-root, read-only, and has no Linux capabilities.
- [ ] Main and wildcard URLs terminate TLS.
- [ ] Coder has no direct public endpoint.
- [ ] Every workspace receives the internal proxy `CODER_URL`.
- [ ] NetworkPolicies are tested from a real workspace.
- [ ] Provisioners and operators are allowed explicitly, without a global rule.
- [ ] Allowlisted usernames and their account mappings are reviewed on every
      Coder instance.
- [ ] The Coder version is pinned and validated.
- [ ] The Prometheus listener is private.
- [ ] Readiness, error, and rejection alerts are configured.
- [ ] The deprecated-route exception is formally accepted.

## Reporting a vulnerability

Report vulnerabilities privately to the repository owner. Do not publish
tokens, cookies, production configuration, or an actionable bypass procedure in
a public issue.
