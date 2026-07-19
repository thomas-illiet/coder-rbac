# Security policy

Please report vulnerabilities privately to the repository owner instead of opening a public issue.

The security boundary assumes:

- all public, wildcard, agent, and workspace-side Coder traffic traverses `coder-rbac`;
- no other Ingress, LoadBalancer, port-forward, Service, or additive NetworkPolicy lets untrusted workspaces reach `coderd` directly;
- the configured Coder version matches the policy pin;
- administrator and allowed usernames are reviewed configuration and mapped to
  the intended account on every Coder instance.

The intentionally allowed deprecated organization workspace-creation endpoint is a documented exception, not a protected operation.
