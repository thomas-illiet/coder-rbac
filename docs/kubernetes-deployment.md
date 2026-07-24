# Kubernetes deployment

This guide installs `coder-policy` with Helm and establishes the required network
boundary around Coder.

## Prerequisites

- DNS for the main Coder hostname and application wildcard;
- TLS certificates covering both hostnames;
- a published `coder-policy` image accessible to the cluster;
- platform networking that makes the public Coder URL reachable from workspaces
  while blocking direct access to `coderd`;
- an inventory of provisioners, operators, monitoring, and other components
  that require direct Coder access.

## Install with Helm

Create `values.production.yaml`:

```yaml
replicaCount: 2

image:
  repository: registry.example.com/platform/coder-policy
  tag: v0.1.0

config:
  upstreamURL: http://coder.coder.svc.cluster.local:80
  publicURL: https://coder.example.com
  wildcardAccessHost: "*.apps.example.com"
  expectedVersionPrefix: "v2.35."
  adminUsernames:
    - platform-admin
  allowedUsernames:
    - developer

service:
  type: ClusterIP
  proxyPort: 8080
  healthPort: 8081
  metricsPort: 9090

ingress:
  main:
    enabled: true
    className: nginx
    host: coder.example.com
    tls:
      enabled: true
      secretName: coder-main-tls
  wildcard:
    enabled: true
    className: nginx
    tls:
      enabled: true
      secretName: coder-wildcard-tls
```

Review all defaults and descriptions in
[`values.yaml`](../charts/coder-policy/values.yaml), then validate and install:

```shell
helm lint charts/coder-policy -f values.production.yaml
helm template coder-policy charts/coder-policy \
  --namespace coder \
  -f values.production.yaml > /tmp/coder-policy-rendered.yaml

helm upgrade --install coder-policy charts/coder-policy \
  --namespace coder \
  --create-namespace \
  -f values.production.yaml \
  --wait \
  --timeout 10m
```

Kubernetes Ingress host rules only accept a wildcard as a complete leftmost
label (`*.apps.example.com`). When using the supported
`*-apps.example.com` form, disable `ingress.wildcard` and configure that route
on an ingress controller or external proxy that supports suffix wildcards.

## Configure the official Coder chart

The official Coder chart must not expose its own Ingress or public Service.
Exact value names may vary by chart version; the required outcome is equivalent
to:

```yaml
coder:
  envUseClusterAccessURL: false
  ingress:
    enable: false
  service:
    type: ClusterIP
  env:
    - name: CODER_ACCESS_URL
      value: https://coder.example.com
    - name: CODER_WILDCARD_ACCESS_URL
      value: "*.apps.example.com"
```

After rendering both charts, confirm that no public Ingress or Service targets
`coderd` directly.

## DNS and TLS

Create both DNS records against the same Ingress load balancer:

```text
coder.example.com        -> Ingress load balancer
*.apps.example.com       -> Ingress load balancer
```

Certificates must cover these exact names:

```text
coder.example.com
*.apps.example.com
```

A certificate for `*.example.com` does not necessarily cover
`*.apps.example.com`. Validate the complete certificate chain from an external
client. Ensure the Ingress preserves the original `Host` and supports upgraded,
long-lived HTTP connections.

## Configure workspaces

Every workspace container must use the public proxy URL:

```hcl
resource "coder_agent" "main" {
  # Other agent settings are omitted.
  env = {
    CODER_URL = "https://coder.example.com"
  }
}
```

When a template defines container variables directly:

```hcl
resource "docker_container" "workspace" {
  # Other container settings are omitted.
  env = [
    "CODER_URL=https://coder.example.com",
  ]
}
```

Agent initialization must also reach Coder through the public proxy URL, never
through the direct `coderd` Service. Confirm that workspace DNS, routing, and
TLS trust make `CODER_POLICY_PUBLIC_URL` reachable before migrating existing
templates.

## External network isolation

The chart intentionally creates no cluster network-isolation resources.
Operators must enforce the required boundary through their platform:

- workspaces can resolve and reach `CODER_POLICY_PUBLIC_URL`;
- untrusted clients and workspaces cannot reach the direct `coderd` Service;
- `coderd` has no alternate public Ingress, LoadBalancer, or port-forward;
- provisioners, operators, and monitoring receive only the direct access they
  require.

Verify the result from a real workspace rather than relying only on rendered
manifests.

## Post-deployment checks

```shell
kubectl rollout status deployment/coder-policy -n coder
kubectl get pods,svc,ingress -n coder
kubectl port-forward -n coder svc/coder-policy 18081:8081 19090:9090
curl -fsS http://127.0.0.1:18081/readyz
curl -fsS http://127.0.0.1:19090/metrics \
  | grep coder_policy_coder_version_ready
```

From a real workspace:

```shell
test "$CODER_URL" = "https://coder.example.com"
coder list
coder rename <workspace> <new-name>  # Must fail.
curl -fsS https://coder.example.com/api/v2/buildinfo
curl --connect-timeout 3 http://coder.coder.svc.cluster.local  # Must fail at the network layer.
```

Also test UI login and navigation, a terminal WebSocket, an agent connection, a
wildcard application, start and stop, a compatible upgrade, all blocked
operations, metrics, and alerts.

## Rollback

```shell
helm history coder-policy -n coder
helm rollback coder-policy <revision> -n coder --wait
```

A rolled-back proxy whose policy is incompatible with the active Coder version
intentionally remains unready. Repeat the post-deployment and workspace
connectivity checks after rollback.
