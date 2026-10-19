#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-e2e.XXXXXXXX")
project="kumbuka-e2e-$(basename "$temp_dir" | tr '[:upper:].' '[:lower:]-')"
compose_file="$root/dev/e2e/compose.yaml"
proxy_pid='' untrusted_pid='' oidc_pid=''

# Compose interpolation must remain valid if setup fails before port allocation.
export E2E_KEYCLOAK_PORT=1 E2E_KEYCLOAK_ORIGIN=http://127.0.0.1:1
export E2E_OIDC_REDIRECT_URI=http://127.0.0.1:1/oidc-e2e/auth/callback

compose() {
  docker compose --project-name "$project" --file "$compose_file" "$@"
}

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  for pid in "$proxy_pid" "$untrusted_pid" "$oidc_pid"; do
    if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; fi
  done
  for pid in "$proxy_pid" "$untrusted_pid" "$oidc_pid"; do
    if [ -n "$pid" ]; then wait "$pid" 2>/dev/null || true; fi
  done
  if [ "$status" -ne 0 ]; then
    for log in "$temp_dir"/*.log; do
      if [ -f "$log" ]; then
        printf '\n--- %s ---\n' "$(basename "$log")" >&2
        tail -n 80 "$log" >&2
      fi
    done
    compose logs keycloak postgres >&2 || true
  fi
  if ! compose down --volumes --remove-orphans; then
    printf 'Failed to clean up E2E Docker project %s\n' "$project" >&2
    status=1
  fi
  rm -r -- "$temp_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

allocate_ports() {
  ports=$(node <<'NODE'
const net = require("node:net");
(async () => {
  const servers = [];
  for (let i = 0; i < 8; i++) {
    const server = net.createServer();
    await new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(0, "127.0.0.1", resolve);
    });
    servers.push(server);
  }
  console.log(servers.map((server) => server.address().port).join(" "));
  await Promise.all(servers.map((server) => new Promise((resolve) => server.close(resolve))));
})().catch((error) => { console.error(error); process.exitCode = 1; });
NODE
  )
  # shellcheck disable=SC2086
  set -- $ports
  proxy_port=$1 proxy_front_port=$2 proxy_management_port=$3
  untrusted_port=$4 untrusted_management_port=$5
  oidc_port=$6 oidc_management_port=$7 E2E_KEYCLOAK_PORT=$8
  export E2E_KEYCLOAK_PORT E2E_KEYCLOAK_ORIGIN="http://127.0.0.1:$E2E_KEYCLOAK_PORT"
  export E2E_OIDC_REDIRECT_URI="http://127.0.0.1:$oidc_port/oidc-e2e/auth/callback"
}

build_application() {
  printf 'Building Kumbuka and preparing Chromium...\n'
  make web plugins
  if [ -z "${E2E_SKIP_BROWSER_INSTALL:-}" ] && [ -z "${E2E_CHROMIUM_PATH:-}" ]; then
    npx playwright install chromium
  fi
  go build -o "$temp_dir/kumbuka" ./cmd/kumbuka
}

wait_url() {
  url=$1
  pid=${2:-}
  attempt=0
  while [ "$attempt" -lt 120 ]; do
    if curl --silent --fail "$url" >/dev/null; then return 0; fi
    if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
      printf 'E2E service exited before becoming ready: %s\n' "$url" >&2
      return 1
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  printf 'Timed out waiting for %s\n' "$url" >&2
  return 1
}

complete_setup() {
  base=$1
  username=$2
  curl --silent --show-error --fail --output /dev/null \
    --data-urlencode "username=$username" \
    --data-urlencode 'display_name=E2E Bootstrap Administrator' \
    --data-urlencode "email=$username@example.test" \
    --data-urlencode 'password=local-test-password' \
    --data-urlencode 'password_confirm=local-test-password' \
    "$base/setup"
}

start_dependencies() {
  printf 'Starting disposable PostgreSQL and Keycloak services (%s)...\n' "$project"
  compose up -d --wait postgres
  compose exec -T postgres createdb -U kumbuka kumbuka_oidc_e2e
  compose up -d keycloak
  database_port=$(compose port postgres 5432)
  database_port=${database_port##*:}
  database_base="postgres://kumbuka:local-test-password@127.0.0.1:$database_port"
  wait_url "$E2E_KEYCLOAK_ORIGIN/realms/kumbuka/.well-known/openid-configuration"
}

start_proxy() {
  KUMBUKA__DATABASE_URL="$database_base/kumbuka_proxy_e2e?sslmode=disable" \
  KUMBUKA__AUTH_MODE=proxy KUMBUKA__ALLOW_USER_REGISTRATION=true \
  KUMBUKA__TRUSTED_PROXY_CIDRS=127.0.0.1/32 \
  KUMBUKA__TRUSTED_ADMIN_GROUP=kumbuka-admins \
  KUMBUKA__PUBLIC_URL="http://127.0.0.1:$proxy_front_port/proxy-e2e" \
  KUMBUKA__ROUTE_PREFIX=/proxy-e2e KUMBUKA__PLUGIN_UPDATE_CHECK_INTERVAL=0 \
    "$temp_dir/kumbuka" --listen-address "127.0.0.1:$proxy_port" \
      --management-listen-address "127.0.0.1:$proxy_management_port" >"$temp_dir/proxy.log" 2>&1 &
  proxy_pid=$!
  wait_url "http://127.0.0.1:$proxy_management_port/healthz" "$proxy_pid"
  complete_setup "http://127.0.0.1:$proxy_port/proxy-e2e" bootstrap-proxy

  KUMBUKA__DATABASE_URL="$database_base/kumbuka_proxy_e2e?sslmode=disable" \
  KUMBUKA__AUTH_MODE=proxy KUMBUKA__ALLOW_USER_REGISTRATION=true \
  KUMBUKA__TRUSTED_PROXY_CIDRS=192.0.2.0/24 \
  KUMBUKA__TRUSTED_ADMIN_GROUP=kumbuka-admins \
  KUMBUKA__PUBLIC_URL="http://127.0.0.1:$untrusted_port/proxy-e2e" \
  KUMBUKA__ROUTE_PREFIX=/proxy-e2e KUMBUKA__PLUGIN_UPDATE_CHECK_INTERVAL=0 \
    "$temp_dir/kumbuka" --listen-address "127.0.0.1:$untrusted_port" \
      --management-listen-address "127.0.0.1:$untrusted_management_port" >"$temp_dir/untrusted.log" 2>&1 &
  untrusted_pid=$!
  wait_url "http://127.0.0.1:$untrusted_management_port/healthz" "$untrusted_pid"
}

start_oidc() {
  KUMBUKA__DATABASE_URL="$database_base/kumbuka_oidc_e2e?sslmode=disable" \
  KUMBUKA__AUTH_MODE=oidc KUMBUKA__ALLOW_USER_REGISTRATION=true \
  KUMBUKA__PUBLIC_URL="http://127.0.0.1:$oidc_port/oidc-e2e" \
  KUMBUKA__ROUTE_PREFIX=/oidc-e2e KUMBUKA__PLUGIN_UPDATE_CHECK_INTERVAL=0 \
  KUMBUKA__OIDC_ISSUER="$E2E_KEYCLOAK_ORIGIN/realms/kumbuka" \
  KUMBUKA__OIDC_CLIENT_ID=kumbuka-test \
  KUMBUKA__OIDC_CLIENT_SECRET=local-test-client-secret \
  KUMBUKA__OIDC_SESSION_SECRET=0123456789abcdef0123456789abcdef \
  KUMBUKA__OIDC_ADMIN_GROUP=kumbuka-admins \
    "$temp_dir/kumbuka" --listen-address "127.0.0.1:$oidc_port" \
      --management-listen-address "127.0.0.1:$oidc_management_port" >"$temp_dir/oidc.log" 2>&1 &
  oidc_pid=$!
  wait_url "http://127.0.0.1:$oidc_management_port/healthz" "$oidc_pid"
  complete_setup "http://127.0.0.1:$oidc_port/oidc-e2e" bootstrap-oidc
}

run_tests() {
  printf 'Running trusted-proxy and OIDC browser tests...\n'
  E2E_PROXY_URL="http://127.0.0.1:$proxy_front_port/" \
  E2E_PROXY_UPSTREAM="http://127.0.0.1:$proxy_port/" \
  E2E_PROXY_UNTRUSTED_UPSTREAM="http://127.0.0.1:$untrusted_port/" \
  E2E_OIDC_URL="http://127.0.0.1:$oidc_port/oidc-e2e/" \
    npm run test:e2e
}

cd "$root"
allocate_ports
build_application
start_dependencies
start_proxy
start_oidc
run_tests
