# Development

## Repository structure

| Area | Responsibility |
| --- | --- |
| `cmd/coder-rbac` | Binary entry point |
| `internal/command` | Cobra commands, flags, server wiring, and signals |
| `internal/config` | Configuration loading and validation |
| `internal/coder` | Minimal, bounded Coder API client |
| `internal/versiongate` | Initial and periodic compatibility checks |
| `internal/proxy` | Host routing, policy evaluation, reverse proxy, and metrics |
| `internal/server` | HTTP server construction and lifecycle |
| `charts/coder-rbac` | Helm chart and NetworkPolicies |
| `dev/local` | Compose bootstrap, workspace template, and validation |

Keep files cohesive: split a file when it combines independent
responsibilities, not when it crosses an arbitrary line count. Extract a shared
helper only when multiple consumers have the same contract; avoid speculative
generic abstractions.

## Prerequisites

- Go `1.26.5`;
- Node.js `22` or newer for the pinned Markdown checks;
- Docker Desktop for local integration;
- Helm for chart validation;
- `jq` for the local scenario;
- ShellCheck for shell validation;
- the Go quality tools declared in `go.mod`.

## Common commands

```shell
make build
make test
make test-race
make docs-check
make quality
make check
```

`make check` is the repository-wide entry point. It covers Go formatting, vet,
race tests, Staticcheck, vulnerability analysis, ShellCheck, Markdown style and
links, Helm lint and rendering, and Compose configuration validation.

## Code documentation

Every named Go function or method, exported or unexported, must have an
immediately associated comment. Exported declarations follow GoDoc convention
and start with the declaration name. Anonymous callbacks are excluded from this
rule.

Every named shell function must also have an immediately preceding explanatory
comment. Comments should describe the contract, responsibility, or side effects
instead of merely restating the signature.

Repository-authored prose, help text, examples, diagrams, comments, and scripts
must be written in English. Preserve technical identifiers and protocol values.

## Tests

The suite covers:

- environment and flag precedence and configuration validation;
- distinct listener ports and server cancellation;
- main, internal, wildcard, and unknown hosts;
- encoded-path normalization and wildcard isolation;
- HTTP and WebSocket upgrades;
- fail-closed version gating and Coder response decoding;
- static routes, dormancy, and build transitions;
- persistent and ephemeral parameters and presets;
- allowlisted-username behavior and identity failures;
- bounded Prometheus labels.

## Add a protected route

1. Add a route kind when necessary.
2. Match the method, segment count, and fixed segments explicitly.
3. Choose a static decision or a focused inspector.
4. Use a stable rejection code.
5. Test allowed, blocked, allowlisted-username, and encoded-path cases.
6. Update the matrix in [policy.md](policy.md).
7. Validate the route through the Coder CLI when workspaces use it.

Never allow an unknown transition or parameter implicitly.

## Add configuration

1. Register the Cobra flag and environment binding in the centralized flag
   definition.
2. Add a default only for optional settings.
3. Load and validate the value in the configuration package.
4. Add the typed `Config` field.
5. Test precedence, defaults, and invalid input.
6. Wire Compose, Helm ConfigMap or Secret, and chart values.
7. Update [configuration.md](configuration.md) and the comments in
   [`values.yaml`](../charts/coder-rbac/values.yaml).

## Dependencies and deprecations

After changing Go dependencies:

```shell
go mod tidy
make test-race
make quality
```

Use the repository-pinned Staticcheck configuration, including `SA1019`, for Go
deprecations. The reverse proxy uses `httputil.ProxyRequest.Rewrite`, `SetURL`,
and `SetXForwarded`, not the deprecated `Director` field.

Avoid global Prometheus registries in tests and server construction. The product
uses an explicit registry per process.

## Image invariants

The build must retain:

- `CGO_ENABLED=0`;
- `-trimpath`;
- injected `Version`, `Commit`, and `Date` metadata;
- a non-root distroless runtime;
- no shell or package manager in the final image.

## Definition of done

A policy or transport change is complete when:

- `make check` passes;
- no reachable known vulnerability is reported;
- the Compose scenario passes;
- the internal CLI, direct isolation, and wildcard application are revalidated;
- policy, operations, and limitations documentation reflects the change.
