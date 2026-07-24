# Limitations and accepted risks

## Deprecated creation route

The following route is intentionally allowed:

```text
POST /api/v2/organizations/{organization}/members/{user}/workspaces
```

On Coder 2.35.x, it can still invoke workspace-creation logic and bypass the
primary route restriction. This explicit product decision has a regression test
that verifies `coder-policy` does not block the route.

Consequences include:

- the product cannot guarantee an absolute creation prohibition on Coder
  2.35.x;
- a client that knows the route can attempt to use it;
- the deployment owner must accept the risk;
- removing the exception requires a policy and test change.

## Route-based coverage

The policy recognizes specific methods and paths. A new Coder route that offers
an equivalent operation is not protected automatically.

For every Coder upgrade:

- compare creation, deletion, and mutation routes;
- review new payloads and transitions;
- maintain integration tests;
- change the expected version prefix only after the audit.

## Coder compatibility

The product targets Coder 2.35.x. The version gate performs a textual prefix
check and does not prove that two patch releases have identical API contracts.

The gate prevents obvious drift but does not replace tests against the deployed
version.

## Global username lists

The administrator and allowed-user lists:

- applies across all organizations;
- admit users globally across all proxy hosts;
- let only administrators bypass every workspace restriction;
- cannot grant exemptions per operation;
- has no expiration time;
- accepts only canonical Coder usernames;
- requires a redeployment to change.

There is no dynamic RBAC engine, group policy, or synchronization with Coder
roles.

Matching is exact and case-sensitive. Usernames make the same policy
configuration portable across Coder instances, but they are mutable and can
identify different accounts on different instances. Renaming a configured user
removes access or the exemption. Reusing that username grants the same
capability to the replacement account, so every deployment must govern username
assignment and audit its username-to-user-ID mapping.

## No independent authentication

`coder-policy` depends entirely on Coder cookies and tokens. It does not validate
their signatures locally or issue sessions.

Every request outside native authentication and public assets requires an
auxiliary identity lookup. Non-authentication lookup failures therefore cause
fail-closed rejections across UI, API, CLI, agent, and application traffic. A
native Coder `401` or `403` is treated as an unresolved user identity and
forwarded so anonymous and non-user flows remain available.

## Latest-build dependency

Parameter validation depends on:

- the workspace's latest build;
- its template version;
- its parameter values;
- current and target schemas available through the API.

A workspace with incomplete data cannot submit a build that requires parameter
validation.

## Textual value comparison

Submitted values are compared as strings. Two semantically equivalent but
textually different representations can be rejected.

An ephemeral parameter introduced only by a target template version may receive
its first value during that upgrade. Established ephemeral values remain subject
to textual equality. See [Workspace parameters](policy.md#workspace-parameters).

## Bounded JSON in memory

Protected request bodies are read into memory up to the configured inspection
limit. A high limit increases memory use; a low limit may reject a legitimate
Coder payload.

Coder API JSON used for identity or policy decisions is read into memory with a
fixed 4 MiB limit. Oversized, malformed, or multi-value responses fail closed.

## Metrics

- `/metrics` has no authentication.
- The chart provides no `ServiceMonitor`.
- `outcome="forwarded"` does not represent Coder's final response status.
- User, workspace, path, and raw-host labels are omitted to bound cardinality.
- Operations from an administrator are not reported as rejections because they
  are forwarded.

## NetworkPolicies

The chart policies:

- depend on exact, configurable labels;
- cover one workspace namespace per release;
- are additive with other policies;
- require a compatible CNI;
- do not automatically allow Git, registry, or Internet egress;
- can block provisioners or operators unless `additionalIngress` is configured.

## Wildcard hosts

The proxy recognizes a non-empty hostname prefix ending in the suffix configured
with either `*.` or `*-`. Coder remains responsible for validating the exact
application hostname format and access permissions. Kubernetes Ingress accepts
only the `*.` form in `spec.rules.host`; `*-` requires an external route that
supports suffix wildcards.

`/api/v2`, `/cli-auth`, and `/setup` are hidden, but the proxy does not inspect
the content of each workspace application.

## High availability

The proxy is stateless and supports multiple replicas. However:

- each replica performs its own version check;
- long-lived connections depend on Ingress and load-balancer behavior;
- replicas do not coordinate;
- all multi-replica deployments must share `CODER_POLICY_LOGOUT_CSRF_SECRET`;
  the Helm chart requires this value when `replicaCount` is greater than one;
- overall availability still depends on Coder and its storage.

## Immutable configuration

There is no configuration file, administration API, or hot reload. Changes are
provided through flags or environment variables and require a restart.

## Docker lab

The Compose environment is a local functional proof, not a production security
model. Mounting the Docker socket in Coder gives that container substantial
privileges and must not be copied into a multi-tenant environment without a
separate risk assessment.
