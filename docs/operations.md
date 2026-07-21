# Operations

## Normal startup

At startup, the process:

1. validates configuration;
2. requests `/api/v2/buildinfo` with the upstream timeout;
3. compares the version with the expected prefix;
4. starts the proxy, health, and metrics listeners;
5. starts periodic build-information checks.

A failure in steps 1 through 3 terminates the process. Kubernetes restarts it
according to the Deployment strategy.

## Shutdown

SIGINT or SIGTERM initiates graceful shutdown. All three HTTP servers use
`CODER_POLICY_SHUTDOWN_TIMEOUT`, after which idle upstream transport connections
are closed.

Helm `terminationGracePeriodSeconds` must exceed the shutdown timeout. Their
defaults are `30s` and `15s`, respectively.

## Routine checks

```shell
curl -fsS http://coder-policy:8081/healthz
curl -fsS http://coder-policy:8081/readyz
curl -fsS http://coder-policy:9090/metrics \
  | grep '^coder_policy_coder_version_ready'
```

In Kubernetes:

```shell
kubectl get pods -n coder -l app.kubernetes.io/name=coder-policy
kubectl logs -n coder deployment/coder-policy --since=1h
kubectl get events -n coder --sort-by=.lastTimestamp
```

## Change administrator or allowed usernames

Configuration does not reload dynamically. Change the Helm values and redeploy:

```shell
helm upgrade coder-policy charts/coder-policy \
  -n coder \
  -f values.production.yaml \
  --wait
```

The Secret checksum in the pod template triggers a rollout.

Before adding a username to either list:

- copy the canonical username exactly as Coder reports it;
- confirm that it maps to the intended account and immutable user ID on every
  managed Coder instance;
- decide whether the account needs standard access or an administrator exemption;
- record the justification and intended duration;
- obtain review;
- protect the username from unreviewed rename or reuse;
- schedule removal when the exemption is no longer needed.

Matching is exact and case-sensitive. A username rename removes access or the
exemption; reuse grants the configured capability to the replacement account.
The same configuration is portable only when every Coder instance assigns each
listed username to the intended principal.

## Change the Coder version

Recommended procedure:

1. identify the exact target version;
2. compare its protected routes and payloads with the policy;
3. run integration tests against that version;
4. adapt the proxy when necessary;
5. update `expectedVersionPrefix`;
6. deploy progressively;
7. verify readiness, CLI, agents, WebSockets, and wildcard applications.

Do not widen the prefix only to silence a readiness error.

## Diagnose by symptom

### The pod does not start

Likely causes include:

- a missing required variable;
- a URL containing a path, query, or credentials;
- a wildcard containing a port;
- a legacy ID-based administrator setting;
- listeners sharing a port;
- unreachable Coder;
- an incompatible Coder version.

```shell
kubectl logs -n coder deployment/coder-policy --previous
kubectl get configmap,secret -n coder coder-policy -o yaml
kubectl exec -n coder deployment/coder-policy \
  -- /usr/local/bin/coder-policy version
```

The distroless image has no shell. Run network diagnostics from an authorized
diagnostic pod.

### `/readyz` returns `503`

Check:

- `Coder version gate became unready` logs;
- DNS and connectivity to the upstream;
- Coder `/api/v2/buildinfo`;
- `CODER_POLICY_EXPECTED_VERSION_PREFIX`;
- the Coder ingress NetworkPolicy.

### The proxy returns `421`

The `Host` is not the public URL, a declared internal host, or a wildcard
subdomain. Check:

- `CODER_POLICY_PUBLIC_URL`;
- `CODER_POLICY_INTERNAL_HOSTS`;
- `Host` preservation by the Ingress;
- DNS and the port present in the request;
- the URL used by the CLI.

### A wildcard application returns `404`

This is expected when the path starts with `/api/v2`, `/cli-auth`, or `/setup`.
For any other path, check:

- Coder `CODER_WILDCARD_ACCESS_URL`;
- `CODER_POLICY_WILDCARD_ACCESS_HOST`;
- wildcard DNS and TLS certificate;
- `Host` forwarding by the Ingress;
- application health in the Coder API.

### A protected route returns `503`

`identity_unavailable` or `policy_data_unavailable` means the proxy lacks the
data required to decide. Check cookies or tokens, Coder permissions, and the
auxiliary endpoint named in server logs. A malformed, oversized, or multi-value
Coder JSON response also fails closed.

### The proxy returns `403 user_access_forbidden`

Coder authenticated the request, but the canonical username is absent from
both `CODER_POLICY_ADMIN_USERNAMES` and `CODER_POLICY_ALLOWED_USERNAMES`. Check the
exact case-sensitive username with `coder users show me`, update the intended
list, and redeploy. Do not add a standard user to the administrator list merely
to restore access.

### The proxy returns `502 upstream_unavailable`

The reverse proxy could not contact Coder or lost the connection. Check the
Service, endpoints, NetworkPolicy, DNS, internal TLS, and `coderd` logs.

### The CLI works but a forbidden action succeeds

Treat this as a potential security incident:

1. inspect `CODER_URL` inside the workspace;
2. inspect reachable networks and DNS;
3. find any direct Coder Ingress or Service;
4. inspect every additive NetworkPolicy;
5. check whether the canonical username is configured as an administrator;
6. identify the actual API route used;
7. determine whether it is the intentionally allowed deprecated creation route.

## Capacity and performance

Every request outside native authentication and public assets adds one
`/users/me` admission lookup. Parameterized workspace builds for allowed
standard users can require several additional Coder reads.

Capacity planning should include:

- UI and API request rate;
- WebSocket and agent connection count;
- start and upgrade frequency;
- Coder API latency;
- inspected body sizes.

The chart defaults to two replicas. Instances are stateless and the proxy does
not require sticky sessions. Confirm Coder and Ingress requirements for
long-lived connections separately.

## Backup

`coder-policy` has no persistent state. Back up:

- Helm values and configuration manifests;
- administrator and allowed-username history and username-to-user-ID mappings;
- Ingress and NetworkPolicy rules;
- dashboards and alerts;
- the Coder database and configuration according to Coder procedures.

## Rollback

After rollback, confirm immediately:

```shell
kubectl rollout status deployment/coder-policy -n coder
curl -fsS http://coder-policy:8081/readyz
```

Then retest at least password and profile updates, workspace creation, renaming,
TTL, the internal CLI, direct Coder isolation, and a wildcard application.
