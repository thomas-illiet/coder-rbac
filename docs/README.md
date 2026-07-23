# coder-policy documentation

This documentation describes the behavior implemented in this repository.
`coder-policy` targets Coder 2.35.x and adds an authorization policy in front of
`coderd` without replacing Coder's native authentication.

## Recommended reading order

1. [Architecture](architecture.md) describes components, network paths, and
   request processing.
2. [Authorization policy](policy.md) is the canonical reference for allowed and
   blocked operations and their HTTP responses.
3. [Configuration](configuration.md) lists environment variables, Cobra flags,
   validation rules, and Helm mappings.
4. [Deployment overview](deployment.md) explains the deployment invariants and
   points to the platform-specific guides.
5. [Container deployment](container-deployment.md) covers OCI images, standalone
   containers, and the local Compose environment.
6. [Kubernetes deployment](kubernetes-deployment.md) covers Helm, Coder chart
   integration, Ingress, TLS, and NetworkPolicies.
7. [Security](security.md) defines the threat model, trust boundaries, and
   hardening requirements.
8. [Observability](observability.md) covers logs, health, readiness, metrics, and
   alerts.
9. [Operations](operations.md) covers startup, upgrades, rollback, and incident
   diagnosis.
10. [Local validation](local-validation.md) documents the Docker Compose lab and
    automated validation scenario.
11. [Limitations](limitations.md) records accepted risks and functional limits.
12. [Development](development.md) explains repository structure and quality
    checks.

## Essential security contract

`coder-policy` is a security boundary only when every path to Coder passes through
it: the public URL, application wildcard, CLI, agents, and workspace-originated
traffic. Any Ingress, Service, LoadBalancer, port-forward, or platform route
that makes `coderd` directly reachable allows the proxy policy to be bypassed.

The following deprecated route is an intentional exception:

```text
POST /api/v2/organizations/{organization}/members/{user}/workspaces
```

It is forwarded to Coder and may create a workspace on Coder 2.35.x. See
[Deprecated creation route](limitations.md#deprecated-creation-route).
