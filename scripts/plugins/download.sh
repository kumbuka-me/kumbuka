#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
lock="$root/plugins.lock"
destination="$root/plugins"

repository="${KUMBUKA_PLUGINS_REPOSITORY:-kumbuka-me/plugins}"
base_url="https://github.com/$repository/releases/download"

download_attempts=30
download_delay=2

if [ ! -f "$lock" ]; then
  echo "plugin lock file not found: $lock" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

if ! command -v unzip >/dev/null 2>&1; then
  echo "unzip is required" >&2
  exit 1
fi

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
  exit 1
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
      echo "failed to download after $download_attempts attempts: $url" >&2
      exit 1
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

temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT INT TERM

mkdir -p "$destination"
mkdir -p "$temporary/packages"

wanted="$temporary/wanted"
: >"$wanted"

count=0
downloaded=0
reused=0

while IFS='=' read -r plugin version; do
  case "$plugin" in
  '' | '#'*)
    continue
    ;;
  esac

  if ! printf '%s\n' "$plugin" | grep -Eq '^[a-z0-9][a-z0-9-]*$'; then
    echo "invalid plugin name: $plugin" >&2
    exit 1
  fi

  if ! printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "invalid version for $plugin: $version" >&2
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

  download \
    "$base_url/$tag/$asset" \
    "$package"

  download \
    "$base_url/$tag/$asset.sha256" \
    "$checksum"

  expected=$(awk 'NR == 1 { print $1 }' "$checksum")
  actual=$(sha256 "$package")

  if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
    echo "checksum mismatch for $asset" >&2
    echo "expected: $expected" >&2
    echo "actual:   $actual" >&2
    exit 1
  fi

  if ! package_matches "$package" "$plugin" "$version"; then
    echo "downloaded package contains unexpected plugin id or version: $asset" >&2
    exit 1
  fi

  cp "$package" "$temporary/packages/$plugin.kumbukaplugin"

  downloaded=$((downloaded + 1))
  count=$((count + 1))
done <"$lock"

if [ "$count" -eq 0 ]; then
  echo "no plugins configured in $lock" >&2
  exit 1
fi

# Publish new packages only after every download and verification succeeded.
for package in "$temporary/packages"/*.kumbukaplugin; do
  [ -f "$package" ] || continue
  cp "$package" "$destination/"
done

# Remove packages that are no longer pinned in plugins.lock.
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

