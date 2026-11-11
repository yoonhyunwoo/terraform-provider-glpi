#!/usr/bin/env bash
# Starts a disposable GLPI, enables the REST API and prints the env vars the acceptance tests need.
# Usage: eval "$(testenv/bootstrap.sh)"
set -euo pipefail

cd "$(dirname "$0")"
PORT="${GLPI_PORT:-8088}"

export GLPI_PORT="$PORT"
docker compose up -d --wait >&2

echo "waiting for GLPI auto-install" >&2
for _ in $(seq 1 120); do
  if docker compose exec -T db mysql -uglpi -pglpi glpi -N -e "select count(*) from glpi_users where name='glpi'" 2>/dev/null | grep -q '^1$'; then
    break
  fi
  sleep 3
done

sql() { docker compose exec -T db mysql -uglpi -pglpi glpi -e "$1" 2>/dev/null; }

sql "UPDATE glpi_configs SET value='1' WHERE context='core' AND name IN ('enable_api','enable_api_login_credentials','enable_api_login_external_token');
     UPDATE glpi_apiclients SET is_active=1, ipv4_range_start=NULL, ipv4_range_end=NULL, ipv6=NULL, app_token=NULL;"

# GLPI stores API tokens encrypted, so let GLPI generate one instead of writing the column.
USER_TOKEN=$(docker compose exec -T glpi php -r 'define("GLPI_ROOT","/var/www/glpi"); include "/var/www/glpi/inc/includes.php"; $u=new User(); $u->getFromDBbyName("glpi"); echo "TOKEN=".$u->getAuthToken("api_token", true)."\n";' 2>/dev/null | sed -n 's/^TOKEN=//p')
[ -n "$USER_TOKEN" ] || { echo "could not generate user token" >&2; exit 1; }

for _ in $(seq 1 30); do
  code=$(curl -s -o /dev/null -w '%{http_code}' \
    -H "Authorization: user_token ${USER_TOKEN}" \
    "http://127.0.0.1:${PORT}/apirest.php/initSession" || true)
  [ "$code" = "200" ] && break
  sleep 2
done
[ "$code" = "200" ] || { echo "initSession returned ${code}" >&2; exit 1; }

cat <<EOF
export TF_ACC=1
export GLPI_URL=http://127.0.0.1:${PORT}/apirest.php
export GLPI_USER_TOKEN=${USER_TOKEN}
EOF
