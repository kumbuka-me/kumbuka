#!/bin/sh
set -eu

usage() {
  cat <<'USAGE'
Usage: scripts/screenshots/run.sh --content DIR --output DIR --editor-slug SLUG [--visits PATHS]

PATHS is a comma-separated list of application paths to visit before the dashboard capture.
USAGE
}

repository=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$repository/scripts/screenshots/compose.yaml"
content_dir=""
output=""
editor_slug=""
visits=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --content)
      [ "$#" -ge 2 ] || { usage >&2; exit 2; }
      content_dir=$2
      shift 2
      ;;
    --output)
      [ "$#" -ge 2 ] || { usage >&2; exit 2; }
      output=$2
      shift 2
      ;;
    --editor-slug)
      [ "$#" -ge 2 ] || { usage >&2; exit 2; }
      editor_slug=$2
      shift 2
      ;;
    --visits)
      [ "$#" -ge 2 ] || { usage >&2; exit 2; }
      visits=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[ -n "$content_dir" ] || { echo "--content is required" >&2; exit 2; }
[ -n "$output" ] || { echo "--output is required" >&2; exit 2; }
[ -n "$editor_slug" ] || { echo "--editor-slug is required" >&2; exit 2; }
[ -d "$content_dir" ] || { echo "Screenshot content directory not found: $content_dir" >&2; exit 1; }

case "$content_dir" in
  /*) ;;
  *) content_dir="$PWD/$content_dir" ;;
esac

case "$output" in
  /*) ;;
  *) output="$PWD/$output" ;;
esac

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-screenshots.XXXXXX")
archive="$work_dir/content.zip"
binary="$work_dir/kumbuka"
server_log="$work_dir/kumbuka.log"
server_pid=""
compose_project="kumbuka-screenshots-$$"

screenshot_port=${SCREENSHOT_PORT:-18080}
database_port=${SCREENSHOT_DB_PORT:-55432}
base_url="http://127.0.0.1:$screenshot_port"

cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  SCREENSHOT_DB_PORT="$database_port" docker compose -f "$compose_file" -p "$compose_project" down --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$work_dir"
}
trap cleanup EXIT INT TERM

mkdir -p "$output"
rm -f "$output/dashboard.png" "$output/editor.png"

cd "$repository"
make generate web

if [ "${SCREENSHOT_SKIP_BROWSER_INSTALL:-0}" != "1" ]; then
  npx playwright install chromium
fi

SCREENSHOT_DB_PORT="$database_port" docker compose -f "$compose_file" -p "$compose_project" up -d --wait

(
  cd "$content_dir"
  find . -type f -name '*.md' -print | LC_ALL=C sort | zip -q "$archive" -@
)

commit=$(git rev-parse --short HEAD 2>/dev/null || printf '%s' docs)
go build -ldflags="-s -w -X main.Version=screenshots -X main.Commit=$commit" -o "$binary" ./cmd/kumbuka
"$binary" \
  --auth-mode=none \
  --listen-address="127.0.0.1:$screenshot_port" \
  --public-url="$base_url" \
  --database-url="postgres://kumbuka:kumbuka@127.0.0.1:$database_port/kumbuka?sslmode=disable" \
  --log-format=text >"$server_log" 2>&1 &
server_pid=$!

attempt=0
until curl --fail --silent "$base_url/healthz" >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    sed -n '1,160p' "$server_log" >&2
    exit 1
  fi
  sleep 0.25
done

SCREENSHOT_BASE_URL="$base_url" \
  SCREENSHOT_ARCHIVE="$archive" \
  SCREENSHOT_OUTPUT="$output" \
  SCREENSHOT_EDITOR_SLUG="$editor_slug" \
  SCREENSHOT_VISITS="$visits" \
  SCREENSHOT_BROWSER_CHANNEL="${SCREENSHOT_BROWSER_CHANNEL:-}" \
  node "$repository/scripts/screenshots/capture.mjs"

printf '%s\n' "Screenshots written to $output"
