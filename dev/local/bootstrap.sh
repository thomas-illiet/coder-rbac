#!/bin/sh
set -eu

coder_url=http://coder:3000
email=${CODER_BOOTSTRAP_EMAIL:-admin@example.com}
username=${CODER_BOOTSTRAP_USERNAME:-admin}
password=${CODER_BOOTSTRAP_PASSWORD:-CoderPolicy123!}
workspace=${CODER_BOOTSTRAP_WORKSPACE:-demo}
restricted_email=${CODER_RESTRICTED_EMAIL:-user@example.com}
restricted_username=${CODER_RESTRICTED_USERNAME:-user}
restricted_password=${CODER_RESTRICTED_PASSWORD:-CoderUser123!}
blocked_email=${CODER_BLOCKED_EMAIL:-blocked@example.com}
blocked_username=${CODER_BLOCKED_USERNAME:-blocked}
blocked_password=${CODER_BLOCKED_PASSWORD:-CoderBlocked123!}

until curl --fail --silent "$coder_url/api/v2/buildinfo" >/dev/null; do
  sleep 2
done

first_user_status=$(curl --silent --output /dev/null --write-out '%{http_code}' "$coder_url/api/v2/users/first")
if [ "$first_user_status" = "404" ]; then
  curl --fail --silent \
    --request POST \
    --header 'Content-Type: application/json' \
    --data "{\"email\":\"$email\",\"username\":\"$username\",\"name\":\"Local Admin\",\"password\":\"$password\",\"trial\":false,\"trial_info\":{}}" \
    "$coder_url/api/v2/users/first" >/dev/null
fi

login_response=$(curl --fail --silent \
  --request POST \
  --header 'Content-Type: application/json' \
  --data "{\"email\":\"$email\",\"password\":\"$password\"}" \
  "$coder_url/api/v2/users/login")
session_token=$(printf '%s' "$login_response" | sed -n 's/.*"session_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [ -z "$session_token" ]; then
  echo "unable to extract the bootstrap session token" >&2
  exit 1
fi

export CODER_URL="$coder_url"
export CODER_SESSION_TOKEN="$session_token"

if ! /opt/coder users show "$restricted_username" >/dev/null 2>&1; then
  /opt/coder users create \
    --email "$restricted_email" \
    --username "$restricted_username" \
    --full-name "Restricted User" \
    --password "$restricted_password"
fi

if ! /opt/coder users show "$blocked_username" >/dev/null 2>&1; then
  /opt/coder users create \
    --email "$blocked_email" \
    --username "$blocked_username" \
    --full-name "Blocked User" \
    --password "$blocked_password"
fi

/opt/coder templates push docker-local --directory /template --yes

if ! /opt/coder show "$workspace" >/dev/null 2>&1; then
  /opt/coder create "$workspace" \
    --template docker-local \
    --parameter cpu=2 \
    --use-parameter-defaults \
    --yes
else
  /opt/coder update "$workspace"
fi

/opt/coder sharing add "$username/$workspace" --user "$restricted_username:use"

echo "local Coder environment is ready"
echo "URL: http://coder.localtest.me:7080"
echo "Login: $email"
echo "Password: $password"
echo "Administrator username for CODER_POLICY_ADMIN_USERNAMES: $username"
echo "Restricted validation login: $restricted_email"
echo "Restricted validation password: $restricted_password"
echo "Blocked validation login: $blocked_email"
echo "Blocked validation password: $blocked_password"
