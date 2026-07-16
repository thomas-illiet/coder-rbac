# Kubernetes deployment

This guide installs `coder-rbac` with Helm and establishes the required network
boundary around Coder.

## Prerequisites

- DNS for the main Coder hostname and application wildcard;
- TLS certificates covering both hostnames;
- a published `coder-rbac` image accessible to the cluster;
- a CNI that enforces Kubernetes NetworkPolicies;
- verified labels for Coder and workspace pods;
- an inventory of provisioners, operators, monitoring, registries, Git servers,
  package mirrors, and other required network peers.

## Install with Helm

Create `values.production.yaml`:

```yaml
replicaCount: 2

image:
  repository: registry.example.com/platform/coder-rbac
  tag: v0.1.0

config:
  upstreamURL: http://coder.coder.svc.cluster.local:80
  publicURL: https://coder.example.com
  wildcardAccessHost: "*.apps.example.com"
  expectedVersionPrefix: "v2.35."
  allowedUsernames:
    - platform-admin

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

networkPolicy:
  workspaceEgress:
    enabled: true
    podSelector:
      matchExpressions:
        - key: coder.com/workspace_id
          operator: Exists
  coderIngress:
    enabled: true
    podSelector:
      matchLabels:
        app.kubernetes.io/name: coder
```

Review all defaults and descriptions in
[`values.yaml`](../charts/coder-rbac/values.yaml), then validate and install:

```shell
helm lint charts/coder-rbac -f values.production.yaml
helm template coder-rbac charts/coder-rbac \
  --namespace coder \
  -f values.production.yaml > /tmp/coder-rbac-rendered.yaml

helm upgrade --install coder-rbac charts/coder-rbac \
  --namespace coder \
  --create-namespace \
  -f values.production.yaml \
  --wait \
  --timeout 10m
```

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

Every workspace container must use the internal proxy Service:

```hcl
resource "coder_agent" "main" {
  # Other agent settings are omitted.
  env = {
    CODER_URL = "http://coder-rbac.coder.svc.cluster.local:8080"
  }
}
```

When a template defines container variables directly:

```hcl
resource "docker_container" "workspace" {
  # Other container settings are omitted.
  env = [
    "CODER_URL=http://coder-rbac.coder.svc.cluster.local:8080",
  ]
}
```

Agent initialization must also reach Coder through the public or internal proxy
URL, never through the direct `coderd` Service.

## NetworkPolicies

### Workspace egress

By default, the chart selects pods with the `coder.com/workspace_id` label and
allows:

- TCP traffic to the `coder-rbac` proxy port;
- UDP and TCP port 53 to the selected cluster DNS pods;
- entries configured in `additionalEgress`.

Add registries, Git servers, package mirrors, Internet proxies, and every other
required destination explicitly. A correctly selected workspace is otherwise
intentionally restricted.

### Coder ingress

By default, the chart selects `app.kubernetes.io/name=coder` and permits
`coder-rbac`. Add external provisioners, operators, and other trusted Coder
components through `additionalIngress`.

### Multiple namespaces

The chart creates a workspace policy in one namespace per release. If
workspaces run in multiple namespaces, install an equivalent egress policy in
each namespace.

### Additive policy behavior

List every policy that selects Coder or workspace pods:

```shell
kubectl get networkpolicy -A
kubectl describe networkpolicy -n coder
```

NetworkPolicies are additive. An existing allow-all policy can make the chart's
policy insufficient. Verify enforcement from a real selected workspace pod.

## Post-deployment checks

```shell
kubectl rollout status deployment/coder-rbac -n coder
kubectl get pods,svc,ingress,networkpolicy -n coder
kubectl port-forward -n coder svc/coder-rbac 18081:8081 19090:9090
curl -fsS http://127.0.0.1:18081/readyz
curl -fsS http://127.0.0.1:19090/metrics \
  | grep coder_rbac_coder_version_ready
```

From a real workspace:

```shell
test "$CODER_URL" = "http://coder-rbac.coder.svc.cluster.local:8080"
coder list
coder rename <workspace> <new-name>  # Must fail.
curl --connect-timeout 3 http://coder.coder.svc.cluster.local  # Must fail at the network layer.
```

Also test UI login and navigation, a terminal WebSocket, an agent connection, a
wildcard application, start and stop, a compatible upgrade, all blocked
operations, metrics, and alerts.

## Rollback

```shell
helm history coder-rbac -n coder
helm rollback coder-rbac <revision> -n coder --wait
```

A rolled-back proxy whose policy is incompatible with the active Coder version
intentionally remains unready. Repeat the post-deployment and workspace network
checks after rollback.
