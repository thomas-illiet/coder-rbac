#!/bin/sh
set -eu

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required to run this validation script" >&2
  exit 1
fi

base_url=${CODER_POLICY_LOCAL_URL:-http://coder.localtest.me:7080}
metrics_url=${CODER_POLICY_LOCAL_METRICS_URL:-http://127.0.0.1:7090/metrics}
admin_email=${CODER_BOOTSTRAP_EMAIL:-admin@example.com}
admin_username=${CODER_BOOTSTRAP_USERNAME:-admin}
admin_password=${CODER_BOOTSTRAP_PASSWORD:-CoderPolicy123!}
workspace=${CODER_BOOTSTRAP_WORKSPACE:-demo}
email=${CODER_RESTRICTED_EMAIL:-user@example.com}
password=${CODER_RESTRICTED_PASSWORD:-CoderUser123!}
blocked_email=${CODER_BLOCKED_EMAIL:-blocked@example.com}
blocked_password=${CODER_BLOCKED_PASSWORD:-CoderBlocked123!}

login_response=$(curl --fail --silent \
  --request POST \
  --header 'Content-Type: application/json' \
  --data "{\"email\":\"$email\",\"password\":\"$password\"}" \
  "$base_url/api/v2/users/login")
session_token=$(printf '%s' "$login_response" | jq -r '.session_token // empty')
if [ -z "$session_token" ]; then
  echo "unable to log in through coder-policy" >&2
  exit 1
fi

admin_login_response=$(curl --fail --silent \
  --request POST \
  --header 'Content-Type: application/json' \
  --data "{\"email\":\"$admin_email\",\"password\":\"$admin_password\"}" \
  "$base_url/api/v2/users/login")
admin_session_token=$(printf '%s' "$admin_login_response" | jq -r '.session_token // empty')
if [ -z "$admin_session_token" ]; then
  echo "unable to log in as the local administrator" >&2
  exit 1
fi

blocked_login_response=$(curl --fail --silent \
  --request POST \
  --header 'Content-Type: application/json' \
  --data "{\"email\":\"$blocked_email\",\"password\":\"$blocked_password\"}" \
  "$base_url/api/v2/users/login")
blocked_session_token=$(printf '%s' "$blocked_login_response" | jq -r '.session_token // empty')
if [ -z "$blocked_session_token" ]; then
  echo "unable to complete native Coder authentication for the blocked user" >&2
  exit 1
fi
blocked_status=$(curl --silent --output /tmp/coder-policy-local-response.json --write-out '%{http_code}' \
  --header "Coder-Session-Token: $blocked_session_token" \
  "$base_url/api/v2/users/me")
if [ "$blocked_status" != "403" ] || [ "$(jq -r '.code // empty' /tmp/coder-policy-local-response.json)" != "user_access_forbidden" ]; then
  echo "blocked user unexpectedly reached Coder (HTTP $blocked_status)" >&2
  cat /tmp/coder-policy-local-response.json >&2
  exit 1
fi
echo "PASS: unlisted user is denied after native Coder authentication (HTTP 403)"

blocked_browser_status=$(curl --silent --output /tmp/coder-policy-local-access-denied.html --write-out '%{http_code}' \
  --header 'Accept: text/html' \
  --header "Coder-Session-Token: $blocked_session_token" \
  "$base_url/")
if [ "$blocked_browser_status" != "403" ] || ! grep -Fq 'data-error-page="403"' /tmp/coder-policy-local-access-denied.html; then
  echo "blocked browser request did not receive the themed access-denied page (HTTP $blocked_browser_status)" >&2
  cat /tmp/coder-policy-local-access-denied.html >&2
  exit 1
fi
echo "PASS: unlisted browser session receives the themed access-denied page (HTTP 403)"

switch_login_response=$(curl --fail --silent \
  --request POST \
  --header 'Content-Type: application/json' \
  --header "Coder-Session-Token: $blocked_session_token" \
  --data "{\"email\":\"$email\",\"password\":\"$password\"}" \
  "$base_url/api/v2/users/login")
if [ -z "$(printf '%s' "$switch_login_response" | jq -r '.session_token // empty')" ]; then
  echo "blocked session prevented switching to an allowed account" >&2
  exit 1
fi
echo "PASS: blocked browser session can submit a new native login"

admin_create_status=$(curl --silent --output /tmp/coder-policy-local-response.json --write-out '%{http_code}' \
  --request POST \
  --header 'Content-Type: application/json' \
  --header "Coder-Session-Token: $admin_session_token" \
  --data '{}' \
  "$base_url/api/v2/users/me/workspaces")
if [ "$admin_create_status" = "403" ]; then
  echo "the local administrator is not exempt from workspace restrictions" >&2
  cat /tmp/coder-policy-local-response.json >&2
  exit 1
fi
echo "PASS: administrator reaches the Coder workspace creation API (HTTP $admin_create_status)"

# api_request sends an authenticated JSON request through the restricted session.
api_request() {
  method=$1
  path=$2
  body=$3
  curl --silent --output /tmp/coder-policy-local-response.json --write-out '%{http_code}' \
    --request "$method" \
    --header 'Content-Type: application/json' \
    --header "Coder-Session-Token: $session_token" \
    --data "$body" \
    "$base_url$path"
}

# assert_status verifies an HTTP status and prints the response when it differs.
assert_status() {
  expected=$1
  actual=$2
  label=$3
  if [ "$actual" != "$expected" ]; then
    echo "$label: expected HTTP $expected, got $actual" >&2
    cat /tmp/coder-policy-local-response.json >&2
    exit 1
  fi
  echo "PASS: $label (HTTP $actual)"
}

workspace_json=$(curl --fail --silent \
  --header "Coder-Session-Token: $admin_session_token" \
  "$base_url/api/v2/users/$admin_username/workspace/$workspace")
workspace_id=$(printf '%s' "$workspace_json" | jq -r '.id // empty')
if [ -z "$workspace_id" ]; then
  echo "unable to find the demo workspace" >&2
  exit 1
fi
app_subdomain=$(printf '%s' "$workspace_json" | jq -r '[.latest_build.resources[].agents[]?.apps[]? | select(.slug == "wildcard")][0].subdomain_name // empty')
if [ -z "$app_subdomain" ]; then
  echo "unable to find the wildcard validation application" >&2
  exit 1
fi

status=$(api_request POST /api/v2/users/me/workspaces '{}')
assert_status 403 "$status" "workspace creation is blocked"

status=$(api_request PATCH "/api/v2/workspaces/$workspace_id" '{"name":"renamed"}')
assert_status 403 "$status" "workspace rename is blocked"

status=$(api_request PUT "/api/v2/workspaces/$workspace_id/ttl" '{"ttl_ms":3600000}')
assert_status 403 "$status" "workspace TTL is blocked"

status=$(api_request PUT /api/v2/users/me/password '{}')
assert_status 403 "$status" "user password updates are blocked"
if [ "$(jq -r '.code // empty' /tmp/coder-policy-local-response.json)" != "user_password_update_forbidden" ]; then
  echo "password update returned an unexpected denial" >&2
  cat /tmp/coder-policy-local-response.json >&2
  exit 1
fi

status=$(api_request PUT /api/v2/users/me/profile '{}')
assert_status 403 "$status" "user profile updates are blocked"
if [ "$(jq -r '.code // empty' /tmp/coder-policy-local-response.json)" != "user_profile_update_forbidden" ]; then
  echo "profile update returned an unexpected denial" >&2
  cat /tmp/coder-policy-local-response.json >&2
  exit 1
fi

for admin_path in /api/v2/users/me/password /api/v2/users/me/profile; do
  admin_status=$(curl --silent --output /tmp/coder-policy-local-response.json --write-out '%{http_code}' \
    --request PUT \
    --header 'Content-Type: application/json' \
    --header "Coder-Session-Token: $admin_session_token" \
    --data '{}' \
    "$base_url$admin_path")
  if [ "$admin_status" = "403" ]; then
    echo "administrator was blocked from $admin_path" >&2
    cat /tmp/coder-policy-local-response.json >&2
    exit 1
  fi
done
echo "PASS: administrator reaches password and profile update APIs"

status=$(api_request POST "/api/v2/workspaces/$workspace_id/builds" '{"transition":"start","rich_parameter_values":[{"name":"debug","value":"true"}]}')
assert_status 403 "$status" "workspace parameter changes are blocked"
if [ "$(jq -r '.code // empty' /tmp/coder-policy-local-response.json)" != "workspace_parameter_change_forbidden" ]; then
  echo "workspace parameter change returned an unexpected denial" >&2
  cat /tmp/coder-policy-local-response.json >&2
  exit 1
fi

status=$(api_request POST '/api/v2/organizations/not-an-organization/members/me/workspaces' '{}')
if [ "$status" = "403" ]; then
  echo "legacy organization route was unexpectedly blocked" >&2
  cat /tmp/coder-policy-local-response.json >&2
  exit 1
fi
echo "PASS: legacy organization creation route reaches Coder (HTTP $status)"

workspace_container=$(docker ps \
  --filter "label=coder.workspace_name=$workspace" \
  --filter "label=coder.owner=$admin_username" \
  --format '{{.Names}}' | head -1)
if [ -z "$workspace_container" ]; then
  echo "demo workspace container is not running" >&2
  exit 1
fi

workspace_networks=$(docker inspect "$workspace_container" --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}')
case " $workspace_networks " in
  *" coder-policy-workspace "*) ;;
  *) echo "workspace is not attached to coder-policy-workspace" >&2; exit 1 ;;
esac
case " $workspace_networks " in
  *" coder-policy-control "*) echo "workspace can directly reach the control network" >&2; exit 1 ;;
esac
echo "PASS: workspace is isolated from the coderd control network"

if docker exec "$workspace_container" sh -lc 'getent hosts coder >/dev/null 2>&1'; then
  echo "workspace can resolve coderd directly" >&2
  exit 1
fi
echo "PASS: direct coderd resolution fails from the workspace"

# The command must expand CODER_URL inside the workspace container.
# shellcheck disable=SC2016
docker exec "$workspace_container" sh -lc 'test "$CODER_URL" = "http://coder.localtest.me:7080"'
echo "PASS: workspace CODER_URL targets the public proxy URL"

internal_status=$(docker exec "$workspace_container" sh -lc \
  'curl --silent --output /tmp/coder-policy-internal-host.json --write-out "%{http_code}" http://coder-policy:7080/api/v2/buildinfo')
if [ "$internal_status" != "421" ]; then
  echo "the internal Service host was not rejected (HTTP $internal_status)" >&2
  docker exec "$workspace_container" cat /tmp/coder-policy-internal-host.json >&2
  exit 1
fi
echo "PASS: internal Service host is rejected (HTTP 421)"

# The command discovers and expands coder_bin inside the workspace container.
# shellcheck disable=SC2016
coder_command='coder_bin=$(command -v coder || find /tmp -type f -name coder -perm -111 2>/dev/null | head -1); test -n "$coder_bin"'

docker exec \
  --env "CODER_SESSION_TOKEN=$session_token" \
  --env 'CODER_URL=http://coder.localtest.me:7080' \
  "$workspace_container" sh -lc "$coder_command; \"\$coder_bin\" list >/dev/null"
echo "PASS: the Coder CLI works through the public proxy URL"

# Coder 2.35.2 has no self-service CLI commands for these mutations. The test
# authenticates with the CLI, then exercises the exact API routes from the same
# container, token, and CODER_URL that the CLI uses.
# shellcheck disable=SC2016
docker exec \
  --env "CODER_SESSION_TOKEN=$session_token" \
  --env 'CODER_URL=http://coder.localtest.me:7080' \
  --env "EXPECTED_CODER_USERNAME=${CODER_RESTRICTED_USERNAME:-user}" \
  "$workspace_container" sh -lc '
    set -eu
    coder_bin=$(command -v coder || find /tmp -type f -name coder -perm -111 2>/dev/null | head -1)
    "$coder_bin" users show me | grep -q "Username:.*$EXPECTED_CODER_USERNAME"
    for route_and_code in \
      "password user_password_update_forbidden" \
      "profile user_profile_update_forbidden"
    do
      set -- $route_and_code
      route=$1
      expected_code=$2
      status=$(curl --silent --output "/tmp/coder-policy-cli-$route.json" --write-out "%{http_code}" \
        --request PUT \
        --header "Content-Type: application/json" \
        --header "Coder-Session-Token: $CODER_SESSION_TOKEN" \
        --data "{}" \
        "$CODER_URL/api/v2/users/me/$route")
      test "$status" = "403"
      grep -q "\"code\":\"$expected_code\"" "/tmp/coder-policy-cli-$route.json"
    done
  '
echo "PASS: CLI identity cannot update its password or profile through the public proxy URL"

if docker exec \
  --env "CODER_SESSION_TOKEN=$session_token" \
  --env 'CODER_URL=http://coder.localtest.me:7080' \
  "$workspace_container" sh -lc "$coder_command; printf '%s\\n' \"\$1\" | \"\$coder_bin\" rename \"\$2/\$1\" renamed" validation "$workspace" "$admin_username" >/tmp/coder-policy-cli-rename.log 2>&1; then
  echo "workspace rename unexpectedly succeeded through the container CLI" >&2
  exit 1
fi
if ! grep -q 'Workspace renaming is not allowed' /tmp/coder-policy-cli-rename.log; then
  echo "container CLI failed, but not with the expected proxy denial" >&2
  cat /tmp/coder-policy-cli-rename.log >&2
  exit 1
fi
echo "PASS: container Coder CLI rename is blocked by coder-policy"

wildcard_url=${CODER_POLICY_LOCAL_WILDCARD_URL:-http://$app_subdomain.apps.localtest.me:7080}
wildcard_body=$(curl --fail --silent --location \
  --header "Coder-Session-Token: $admin_session_token" \
  "$wildcard_url/")
case "$wildcard_body" in
  *"Wildcard application reached through coder-policy"*) ;;
  *) echo "wildcard application did not return the expected content" >&2; exit 1 ;;
esac
echo "PASS: wildcard application is reachable through coder-policy"

blocked_wildcard_status=$(curl --silent --output /tmp/coder-policy-local-response.json --write-out '%{http_code}' \
  --header "Coder-Session-Token: $blocked_session_token" \
  "$wildcard_url/")
assert_status 403 "$blocked_wildcard_status" "unlisted user is denied on wildcard applications"

status=$(curl --silent --output /tmp/coder-policy-local-response.json --write-out '%{http_code}' \
  --header "Coder-Session-Token: $session_token" \
  "$wildcard_url/api/v2")
assert_status 404 "$status" "API paths are hidden on wildcard hosts"

status=$(curl --silent --output /tmp/coder-policy-local-response.json --write-out '%{http_code}' \
  --header 'Host: undeclared.localtest.me:7080' \
  http://127.0.0.1:7080/)
assert_status 421 "$status" "undeclared hosts are rejected"

metrics_body=$(curl --fail --silent "$metrics_url")
for metric in coder_policy_build_info coder_policy_coder_version_ready coder_policy_http_requests_total coder_policy_policy_rejections_total; do
  case "$metrics_body" in
    *"$metric"*) ;;
    *) echo "Prometheus output does not contain $metric" >&2; exit 1 ;;
  esac
done
echo "PASS: dedicated Prometheus endpoint exposes coder-policy metrics"

echo "Local validation completed successfully."
