# Observability

## Listeners

| Endpoint | Default listener | Purpose |
| --- | --- | --- |
| `GET /healthz` | `:8081` | Process liveness |
| `GET /readyz` | `:8081` | Version-gate compatibility and availability |
| `GET /metrics` | `:9090` | Prometheus scrape |

Every other path on the Prometheus listener returns `404`.

## Liveness

A healthy response is:

```json
{"status":"ok"}
```

Liveness indicates only that the HTTP process is running. It does not guarantee
access to Coder or compatibility with its version.

## Readiness

A ready response is:

```json
{
  "status": "ready",
  "coder_version": "v2.35.2+5c2838a"
}
```

An unready response uses HTTP `503`:

```json
{"status":"unready"}
```

The main proxy listener also rejects requests with `503` while readiness is
negative.

## Logs

The process writes JSON logs to stderr through `slog`. Important messages are:

| Message | Meaning |
| --- | --- |
| `Coder version policy accepted` | Initial compatibility check succeeded |
| `server listening` | A proxy, health, or metrics listener started |
| `Coder version gate became unready` | A periodic compatibility check failed |
| `request rejected while version gate is unready` | A request was rejected globally |
| `policy dependency failed` | Required Coder policy data was unavailable |
| `upstream proxy request failed` | Transport to `coderd` failed |

Kubernetes example:

```shell
kubectl logs -n coder deployment/coder-policy --since=15m
```

Compose example:

```shell
docker compose logs --since=15m coder-policy
```

## Standard metrics

The registry contains Go and process collectors, including:

- `go_goroutines`;
- `go_memstats_*`;
- `process_cpu_seconds_total`;
- `process_resident_memory_bytes`;
- `process_open_fds` when supported by the platform.

## coder-policy metrics

### Build information

```text
coder_policy_build_info{version="...",commit="..."} 1
```

Use this metric to inventory the versions actually deployed.

### Version gate

```text
coder_policy_coder_version_ready 1
```

`1` means that the latest check was compatible. `0` means that the proxy is
failing closed.

### Requests

```text
coder_policy_http_requests_total{method="POST",host_kind="main",outcome="rejected"} 12
```

| Label | Values |
| --- | --- |
| `method` | Standard HTTP methods or `OTHER` |
| `host_kind` | `main`, `internal`, `wildcard`, `unknown`, or `unclassified` |
| `outcome` | `forwarded` or `rejected` |

`forwarded` means the request was handed to the reverse proxy. It does not mean
that Coder returned a success status.

### Latency

```text
coder_policy_http_request_duration_seconds_bucket
coder_policy_http_request_duration_seconds_sum
coder_policy_http_request_duration_seconds_count
```

Latency covers policy evaluation and, for a forwarded request, upstream
processing time.

### Rejections

```text
coder_policy_policy_rejections_total{code="workspace_rename_forbidden"} 4
```

The `code` label uses the stable codes in
[Authorization policy](policy.md#rejection-responses). Wildcard control-path
rejections use `wildcard_control_path`.

## Prometheus configuration

Static scrape example:

```yaml
scrape_configs:
  - job_name: coder-policy
    metrics_path: /metrics
    static_configs:
      - targets:
          - coder-policy.coder.svc.cluster.local:9090
```

The chart does not install a `ServiceMonitor` or `PodMonitor`. When using
Prometheus Operator, create the relevant resource against the `metrics` Service
port. For example:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: coder-policy
  namespace: coder
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: coder-policy
      app.kubernetes.io/instance: coder-policy
  endpoints:
    - port: metrics
      path: /metrics
      interval: 30s
```

Verify the rendered chart labels before applying this example.

## Suggested PromQL alerts

### Unavailable target

```promql
up{job="coder-policy"} == 0
```

### Incompatible or unknown Coder version

```promql
max by (instance) (coder_policy_coder_version_ready) == 0
```

### Rejection increase

```promql
sum by (code) (rate(coder_policy_policy_rejections_total[10m])) > 0.1
```

Tune the threshold to normal usage. Creation rejections may be expected when
the UI still displays actions that the proxy denies.

### p95 latency

```promql
histogram_quantile(
  0.95,
  sum by (le, host_kind, outcome) (
    rate(coder_policy_http_request_duration_seconds_bucket[5m])
  )
)
```

### Restarts

```promql
increase(kube_pod_container_status_restarts_total{container="coder-policy"}[15m]) > 0
```

## Minimum dashboard

An operational dashboard should show:

- scrape availability;
- version-gate state;
- deployed versions and commits;
- forwarded and rejected request rates by host kind;
- rejections by code;
- p50, p95, and p99 latency;
- CPU, memory, goroutines, and file descriptors;
- Kubernetes restarts;
- `upstream proxy request failed` events from logs.

## Cardinality

Metrics never use a workspace ID, user ID, username, raw host, path, or
free-form message as a label. This keeps cardinality bounded despite untrusted
user input.
