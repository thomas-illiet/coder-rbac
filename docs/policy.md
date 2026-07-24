# Authorization policy

This document is the canonical reference for the policy enforced by
`coder-policy`.

## Scope

The workspace policy applies to requests received on the configured public
host. Application wildcard hosts use a separate policy that hides control-plane
paths. Any other host receives `421 unknown_host`.

`CODER_POLICY_ADMIN_USERNAMES` contains administrators who are admitted and bypass
every workspace restriction below. `CODER_POLICY_ALLOWED_USERNAMES` contains
standard users who are admitted but remain subject to the policy. Matching is
exact and case-sensitive against Coder's canonical `username`; UUIDs and email
addresses are not accepted. Any other authenticated username receives
`403 user_access_forbidden` on main, internal, and application wildcard hosts.

## Access admission

Coder remains the authentication authority. The proxy resolves the request's
canonical user through `/api/v2/users/me` before forwarding:

- an administrator or allowed user is admitted;
- any other resolved username receives `403 user_access_forbidden`;
- a Coder `401` or `403` means no user identity was resolved, so the original
  request is forwarded for native login, anonymous, agent, or service handling;
- any other identity failure receives `503 identity_unavailable`.

Native password or SSO authentication can therefore complete before the next
authenticated request is denied. The proxy does not issue Coder sessions and
revokes one only through the explicit **Change account** flow. Wildcard
control-plane paths remain hidden with `404` before admission is evaluated. The
main host keeps the native login, logout, password reset, OAuth/OIDC, OTP,
authentication-method, and public `/assets` routes reachable so a denied
browser session can switch accounts.

## Operation matrix

| Operation | Method and route | Allowed standard user | Administrator |
| --- | --- | --- | --- |
| Change a user password | `PUT /api/v2/users/{user}/password` | Blocked | Allowed |
| Change a user profile | `PUT /api/v2/users/{user}/profile` | Blocked | Allowed |
| Create a workspace | `POST /api/v2/users/{user}/workspaces` | Blocked | Allowed |
| Deprecated creation route | `POST /api/v2/organizations/{organization}/members/{user}/workspaces` | **Intentionally allowed** | Allowed |
| Rename or modify a workspace | `PATCH /api/v2/workspaces/{workspace}` | Blocked | Allowed |
| Change autostart | `PUT /api/v2/workspaces/{workspace}/autostart` | Blocked | Allowed |
| Change automatic updates | `PUT /api/v2/workspaces/{workspace}/autoupdates` | Blocked | Allowed |
| Extend a deadline | `PUT /api/v2/workspaces/{workspace}/extend` | Blocked | Allowed |
| Change TTL or automatic shutdown | `PUT /api/v2/workspaces/{workspace}/ttl` | Blocked | Allowed |
| Write provisioner state | `PUT /api/v2/workspacebuilds/{build}/state` | Blocked | Allowed |
| Mark dormant | `PUT /api/v2/workspaces/{workspace}/dormant` with `dormant=true` | Blocked | Allowed |
| Wake from dormancy | Same route with `dormant=false` | Allowed | Allowed |
| Delete | `POST /api/v2/workspaces/{workspace}/builds` with `transition=delete` | Blocked | Allowed |
| Start | Build with `transition=start` | Conditionally allowed | Allowed |
| Stop | Build with `transition=stop` | Conditionally allowed | Allowed |
| Upgrade a template | `start` build targeting another version | Conditionally allowed | Allowed |
| Favorite, ACL, and unlisted routes | Corresponding Coder routes | Allowed | Allowed |

The entire workspace `PATCH` route is blocked. This prevents renaming and also
fails closed if Coder adds another mutation to that route.

Password and profile restrictions apply to every `{user}` representation,
including `me`, usernames, and UUIDs. Profile updates cover the username,
display name, and avatar URL fields accepted by Coder. Reads, appearance, and
preference settings remain available.

## Builds and transitions

An inspected build body may contain:

```json
{
  "transition": "start",
  "template_version_id": "00000000-0000-0000-0000-000000000000",
  "template_version_preset_id": "00000000-0000-0000-0000-000000000000",
  "rich_parameter_values": [],
  "state": null
}
```

Only `start` and `stop` are accepted for standard users. `delete` receives its
own rejection. A missing transition produces `400`; any unknown transition
produces `403`, so a transition added by Coder is never allowed implicitly.

A non-null, non-empty `state` value is always blocked.

## Workspace parameters

A `start` or `stop` request that submits no template version, preset, or rich
parameter values is forwarded without parameter-data lookups.

When any of those fields is present, the proxy:

1. loads the workspace and its latest build;
2. loads the current template-version parameter schema;
3. loads the target schema when the version changes;
4. compares the exact set of persistent parameter names;
5. loads values from the latest build;
6. resolves the requested preset, if any;
7. validates every value from the preset and request.

The following rules apply:

- The target version must have exactly the same set of persistent parameter
  names as the current version. A new persistent parameter is blocked even if
  it has a default value.
- Current and target schemas must contain non-empty, unique parameter names. An
  invalid schema is treated as unavailable policy data and fails closed.
- A submitted parameter must exist in the target schema.
- A submitted persistent parameter or an ephemeral parameter already present in
  the current schema must equal its value from the latest build.
- During a template upgrade, an ephemeral parameter that exists only in the
  target schema may receive its first value. It has no current value to preserve
  and does not change the persistent workspace configuration.
- Missing current data for any other submitted parameter makes the policy
  unavailable and produces a fail-closed `503` response.
- A preset is accepted only when it exists in the target version and all of its
  values satisfy the same rules. Explicit rich parameter values are validated
  as well.

## Request bodies and path normalization

Inspected request bodies are limited by
`CODER_POLICY_MAX_INSPECTION_BODY_BYTES`, which defaults to 1 MiB. A larger body
produces `413 request_too_large`.

The body must contain exactly one valid JSON value. Trailing whitespace is
accepted; another JSON value or invalid required fields produce
`400 invalid_request`.

Before route matching, the path is:

- decoded up to four times;
- converted from `\\` separators to `/`;
- cleaned of `.` and `..` segments;
- rejected if it contains a null byte.

This normalization prevents bypasses based on repeated encoding or ambiguous
path separators.

## Rejection responses

Policy rejections use a stable JSON shape for API, CLI, and other non-browser
requests:

```json
{
  "message": "Workspace renaming is not allowed.",
  "code": "workspace_rename_forbidden"
}
```

These responses set `Content-Type: application/json; charset=utf-8` and
`Cache-Control: no-store`.

A `GET` or `HEAD` request that explicitly accepts `text/html` receives the
Code Studio-themed `403` page when its decision is `user_access_forbidden`.
The page is embedded in the proxy binary, uses no external assets, and remains
non-cacheable. Other policy decisions retain the JSON response even when the
client accepts HTML.

The page's **Change account** form posts to `/_coder-policy/logout` on the main
public URL with a five-minute signed CSRF token. After validating that token,
the proxy copies the browser's Coder session cookie into the matching
`Coder-Session-Token` header and relays the request to Coder's native
`POST /api/v2/users/logout` endpoint. Coder then validates the matching
cookie/header authentication, deletes the server-side session, expires its
browser cookie, and invalidates application tokens. The proxy preserves Coder's
`Set-Cookie` headers and redirects to `/login` with `303` only after a successful
logout. Invalid CSRF requests never reach Coder, and a failed native logout is
returned unchanged instead of being hidden by a login redirect.

| Status | Code | Primary cause |
| --- | --- | --- |
| `400` | `invalid_path` | Invalid normalized path |
| `400` | `invalid_request` | Invalid JSON or required field |
| `400` | `invalid_workspace` | Invalid workspace UUID |
| `400` | `invalid_template_preset` | Preset is absent from the target version |
| `401` or `403` | `authentication_required` | Coder rejects the supplied identity |
| `403` | `invalid_logout_csrf` | Missing, expired, or invalid account-change form token |
| `403` | `user_access_forbidden` | Authenticated username is in neither configured list |
| `403` | `user_password_update_forbidden` | User password update |
| `403` | `user_profile_update_forbidden` | Username, display name, or avatar update |
| `403` | `workspace_creation_forbidden` | Primary creation route |
| `403` | `workspace_rename_forbidden` | Workspace `PATCH` route |
| `403` | `workspace_settings_forbidden` | Autostart, updates, deadline, TTL, or state route |
| `403` | `workspace_dormancy_forbidden` | `dormant=true` |
| `403` | `workspace_deletion_forbidden` | `delete` transition |
| `403` | `workspace_transition_forbidden` | Unknown transition |
| `403` | `workspace_state_forbidden` | Provisioner state in a build request |
| `403` | `persistent_parameter_schema_changed` | Persistent parameter-name set differs |
| `403` | `workspace_parameter_change_forbidden` | Changed value or unknown parameter |
| `413` | `request_too_large` | Inspected body exceeds the configured limit |
| `421` | `unknown_host` | Host is not declared |
| `502` | `upstream_unavailable` | Reverse proxy cannot reach Coder |
| `503` | `coder_version_unavailable` | Version gate is not ready |
| `503` | `identity_unavailable` | Identity cannot be verified |
| `503` | `policy_data_unavailable` | Required Coder policy data is unavailable |

Control-plane paths on an application wildcard host intentionally use a plain
`404` response instead of a policy JSON response.
