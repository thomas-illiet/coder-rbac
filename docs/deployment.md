# Deployment overview

Production deployment is a security-boundary exercise, not only a proxy
installation. Choose the platform-specific guide after reviewing the invariants
below:

- [Container deployment](container-deployment.md) covers the OCI image,
  standalone containers, and Docker Compose.
- [Kubernetes deployment](kubernetes-deployment.md) covers Helm, the official
  Coder chart, Ingress, TLS, workspace configuration, and NetworkPolicies.

## Required invariants

Every deployment must satisfy all of these conditions:

1. The public Coder URL and application wildcard route through `coder-policy`.
2. Every workspace and bundled Coder CLI uses a proxy URL in `CODER_URL`.
3. Workspace traffic cannot reach `coderd` directly.
4. `coderd` has no alternate public Ingress, LoadBalancer, or equivalent path.
5. The version gate accepts the exact Coder release in use.
6. Health and metrics listeners are not exposed as public Coder endpoints.
7. Long-lived HTTP upgrades and the original `Host` header reach the proxy
   unchanged.
8. Administrator and allowed-user lists contain the intended canonical usernames.

## Production prerequisites

Before deployment, prepare:

- a Coder version compatible with the configured prefix, normally `v2.35.`;
- DNS for the main URL and wildcard DNS for applications;
- main and wildcard TLS certificates;
- a published `coder-policy` image built from an identified commit;
- the exact labels and network identities used by Coder, workspaces,
  provisioners, and operators;
- monitoring for readiness, transport errors, and policy rejections.

## Validation sequence

After any deployment or configuration change:

1. confirm `/healthz`, `/readyz`, and the version-ready metric;
2. test browser login for an administrator, an allowed user, and a denied user;
3. test navigation, an agent, a terminal, and a wildcard application;
4. verify that start, stop, wake, and compatible upgrades still work;
5. verify that password, profile, workspace creation, deletion, renaming,
   lifecycle settings, and parameter changes are rejected for a standard user;
6. test from a real workspace that its CLI works through `coder-policy` and a
   direct `coderd` request fails at the network layer.

The last test is essential. Static configuration alone does not prove that the
security boundary is effective.

## Upgrade and rollback

Before upgrading Coder:

1. review API changes for protected routes and payloads;
2. run the proxy tests against the exact target version;
3. update the proxy when necessary;
4. change `expectedVersionPrefix` only after validation;
5. deploy the compatible proxy before or with Coder to avoid an extended
   fail-closed interval.

After a rollback, repeat the validation sequence. A proxy whose configured
version prefix does not match the active Coder version intentionally remains
unready.
