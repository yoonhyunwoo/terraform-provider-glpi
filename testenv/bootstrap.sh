#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
PORT="${GLPI_PORT:-8088}"

export GLPI_PORT="$PORT"
docker compose up -d --wait >&2

sql() { docker compose exec -T db mysql -uglpi -pglpi glpi "$@"; }

echo "waiting for GLPI auto-install" >&2
ready=""
for _ in $(seq 1 120); do
  users=$(sql -N -e "select count(*) from glpi_users where name='glpi'" 2>/dev/null || true)
  if [ "$users" = "1" ]; then
    ready=1
    break
  fi
  sleep 3
done
[ -n "$ready" ] || { echo "GLPI did not finish its auto-install in time" >&2; docker compose logs --tail 50 glpi >&2; exit 1; }

sql -e "UPDATE glpi_configs SET value='1' WHERE context='core' AND name IN ('enable_api','enable_api_login_credentials','enable_api_login_external_token');
        UPDATE glpi_apiclients SET is_active=1, ipv4_range_start=NULL, ipv4_range_end=NULL, ipv6=NULL, app_token=NULL;" 2>/dev/null

# The Fields plugin accepts values through the REST API only on GLPI 11, from plugin version 1.23.3.
case "${GLPI_VERSION:-10.0}" in
  11*) FIELDS_VERSION="${GLPI_FIELDS_VERSION-1.24.7}" ;;
  *) FIELDS_VERSION="${GLPI_FIELDS_VERSION-}" ;;
esac
install_fields_plugin() {
  docker compose exec -T glpi sh -c "cd /tmp && curl -fsSL -o fields.tar.bz2 https://github.com/pluginsGLPI/fields/releases/download/${FIELDS_VERSION}/glpi-fields-${FIELDS_VERSION}.tar.bz2 && tar xjf fields.tar.bz2 -C /var/www/glpi/plugins && chown -R www-data /var/www/glpi/plugins/fields; cd /var/www/glpi && php bin/console plugin:install fields -u glpi; php bin/console plugin:activate fields" >&2 || true
  docker compose exec -T glpi php /var/www/glpi/bin/console plugin:list 2>/dev/null | grep -E '^\| fields .*Enabled' >/dev/null
}
if [ -n "$FIELDS_VERSION" ]; then
  fields_ready=""
  for _ in $(seq 1 5); do
    install_fields_plugin && { fields_ready=1; break; }
    sleep 5
  done
  [ -n "$fields_ready" ] || { echo "could not enable the Fields plugin ${FIELDS_VERSION}" >&2; exit 1; }
fi

generate_token_through_glpi() {
  local php10='define("GLPI_ROOT","/var/www/glpi"); include "/var/www/glpi/inc/includes.php"; $u=new User(); $u->getFromDBbyName("glpi"); echo "TOKEN=".$u->getAuthToken("api_token", true)."\n";'
  local php11='require "/var/www/glpi/vendor/autoload.php"; $kernel=new Glpi\Kernel\Kernel("production"); $kernel->boot(); $kernel->getContainer()->get("request_stack")->push(Symfony\Component\HttpFoundation\Request::create("/")); $u=new User(); $u->getFromDBbyName("glpi"); echo "TOKEN=".$u->getAuthToken("api_token", true)."\n";'
  local code token
  for code in "$php11" "$php10"; do
    token=$(docker compose exec -T glpi php -r "$code" 2>/dev/null | sed -n 's/^TOKEN=//p')
    [ -n "$token" ] && { echo "$token"; return; }
  done
}
USER_TOKEN=""
for _ in $(seq 1 20); do
  USER_TOKEN=$(generate_token_through_glpi || true)
  [ -n "$USER_TOKEN" ] && break
  sleep 3
done
[ -n "$USER_TOKEN" ] || { echo "could not generate user token" >&2; exit 1; }

code=""
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
export GLPI_PURGE_ON_DESTROY=true
EOF
