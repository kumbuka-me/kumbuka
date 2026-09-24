#!/bin/sh
set -eu

usage() {
  cat <<'USAGE'
Usage: scripts/plugins/download.sh [OPTIONS]

Download and verify the plugin packages pinned in plugins.lock.

Options:
  --plugin-lock FILE         Plugin lock file. Default: plugins.lock
  --destination DIR         Package destination. Default: plugins
  --plugin-repository REPO  GitHub repository. Default: kumbuka-me/plugins
  -h, --help                Show this help.
USAGE
}

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
lock="$root/plugins.lock"
destination="$root/plugins"
repository=${KUMBUKA_PLUGINS_REPOSITORY:-kumbuka-me/plugins}
download_attempts=${KUMBUKA_PLUGIN_DOWNLOAD_ATTEMPTS:-30}
download_delay=${KUMBUKA_PLUGIN_DOWNLOAD_DELAY:-2}
temporary=""

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  [ -n "$temporary" ] && rm -rf "$temporary"
  exit "$status"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
  --plugin-lock)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    lock=$2
    shift 2
    ;;
  --destination)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    destination=$2
    shift 2
    ;;
  --plugin-repository)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    repository=$2
    shift 2
    ;;
  -h | --help)
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

case "$lock" in
/*) ;;
*) lock="$root/$lock" ;;
esac
case "$destination" in
/*) ;;
*) destination="$root/$destination" ;;
esac

[ -f "$lock" ] || {
  echo "Plugin lock file not found: $lock" >&2
  exit 1
}

case "$download_attempts" in
'' | *[!0-9]*)
  echo "KUMBUKA_PLUGIN_DOWNLOAD_ATTEMPTS must be a positive integer" >&2
  exit 2
  ;;
esac
[ "$download_attempts" -gt 0 ] || {
  echo "KUMBUKA_PLUGIN_DOWNLOAD_ATTEMPTS must be greater than zero" >&2
  exit 2
}

case "$download_delay" in
'' | *[!0-9]*)
  echo "KUMBUKA_PLUGIN_DOWNLOAD_DELAY must be a non-negative integer" >&2
  exit 2
  ;;
esac

command -v curl >/dev/null 2>&1 || {
  echo "curl is required" >&2
  exit 1
}
command -v unzip >/dev/null 2>&1 || {
  echo "unzip is required" >&2
  exit 1
}

base_url="https://github.com/$repository/releases/download"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
    return
  fi

  echo "sha256sum or shasum is required" >&2
  return 1
}

download() {
  url=$1
  output=$2
  attempt=1

  while :; do
    if curl \
      --fail \
      --location \
      --silent \
      --show-error \
      --output "$output" \
      "$url"; then
      return
    fi

    rm -f "$output"

    if [ "$attempt" -ge "$download_attempts" ]; then
      echo "Failed to download after $download_attempts attempts: $url" >&2
      return 1
    fi

    echo "Download not available yet; retrying in ${download_delay}s ($attempt/$download_attempts)"
    sleep "$download_delay"
    attempt=$((attempt + 1))
  done
}

package_manifest() {
  package=$1

  [ -f "$package" ] || return 1
  unzip -tqq "$package" >/dev/null 2>&1 || return 1
  unzip -p "$package" plugin.yaml 2>/dev/null
}

package_field() {
  package=$1
  field=$2

  manifest=$(package_manifest "$package") || return 1
  printf '%s\n' "$manifest" |
    awk -v field="$field" '$1 == field ":" { print $2; exit }'
}

package_matches() {
  package=$1
  plugin=$2
  version=$3

  installed_id=$(package_field "$package" id) || return 1
  installed_version=$(package_field "$package" version) || return 1

  [ "$installed_id" = "me.kumbuka.$plugin" ] &&
    [ "$installed_version" = "$version" ]
}

temporary=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-plugins.XXXXXX")
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$destination" "$temporary/packages"
wanted="$temporary/wanted"
: >"$wanted"

count=0
downloaded=0
reused=0

while IFS='=' read -r plugin version || [ -n "$plugin$version" ]; do
  case "$plugin" in
  '' | '#'*)
    continue
    ;;
  esac

  if ! printf '%s\n' "$plugin" | grep -Eq '^[a-z0-9][a-z0-9-]*$'; then
    echo "Invalid plugin name: $plugin" >&2
    exit 1
  fi

  if ! printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "Invalid version for $plugin: $version" >&2
    exit 1
  fi

  if grep -Fxq "$plugin" "$wanted"; then
    echo "Duplicate plugin in $lock: $plugin" >&2
    exit 1
  fi
  printf '%s\n' "$plugin" >>"$wanted"

  installed="$destination/$plugin.kumbukaplugin"

  if package_matches "$installed" "$plugin" "$version"; then
    echo "Using $plugin v$version"
    reused=$((reused + 1))
    count=$((count + 1))
    continue
  fi

  asset="$plugin-$version.kumbukaplugin"
  tag="$plugin/v$version"
  package="$temporary/$asset"
  checksum="$temporary/$asset.sha256"

  if [ -f "$installed" ]; then
    current_version=$(package_field "$installed" version 2>/dev/null || true)
    if [ -n "$current_version" ]; then
      echo "Updating $plugin v$current_version -> v$version"
    else
      echo "Replacing invalid $plugin package with v$version"
    fi
  else
    echo "Downloading $plugin v$version"
  fi

  download "$base_url/$tag/$asset" "$package"
  download "$base_url/$tag/$asset.sha256" "$checksum"

  expected=$(awk 'NR == 1 { print $1 }' "$checksum" | tr '[:upper:]' '[:lower:]')
  actual=$(sha256 "$package" | tr '[:upper:]' '[:lower:]')

  if ! printf '%s\n' "$expected" | grep -Eq '^[0-9a-f]{64}$'; then
    echo "Invalid checksum file for $asset" >&2
    exit 1
  fi
  if [ "$actual" != "$expected" ]; then
    echo "Checksum mismatch for $asset" >&2
    echo "expected: $expected" >&2
    echo "actual:   $actual" >&2
    exit 1
  fi

  if ! package_matches "$package" "$plugin" "$version"; then
    echo "Downloaded package contains unexpected plugin id or version: $asset" >&2
    exit 1
  fi

  cp "$package" "$temporary/packages/$plugin.kumbukaplugin"
  downloaded=$((downloaded + 1))
  count=$((count + 1))
done <"$lock"

if [ "$count" -eq 0 ]; then
  echo "No plugins configured in $lock" >&2
  exit 1
fi

# Publish only after every new package has downloaded and validated successfully.
for package in "$temporary/packages"/*.kumbukaplugin; do
  [ -f "$package" ] || continue
  cp "$package" "$destination/"
done

# Remove packages that are no longer pinned in the lock file.
for package in "$destination"/*.kumbukaplugin; do
  [ -f "$package" ] || continue

  plugin=${package##*/}
  plugin=${plugin%.kumbukaplugin}

  if ! grep -Fxq "$plugin" "$wanted"; then
    echo "Removing $plugin"
    rm -f "$package"
  fi
done

echo "Plugins: $count total, $downloaded downloaded, $reused unchanged"
