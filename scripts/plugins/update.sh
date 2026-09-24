#!/bin/sh
set -eu

usage() {
  cat <<'USAGE'
Usage: scripts/plugins/update.sh [OPTIONS]

Update plugins.lock to the newest stable release of each pinned plugin, verify
that every updated package can be downloaded, and commit the lock-file change.

Options:
  --gh-bin COMMAND                GitHub CLI command. Default: gh
  --plugin-lock FILE              Plugin lock file. Default: plugins.lock
  --plugin-repository REPO        GitHub repository. Default: kumbuka-me/plugins
  --plugin-update-commit MESSAGE  Commit message. Default: chore: update plugins
  --plugin-stamp FILE             Download stamp. Default: plugins/.downloaded
  -h, --help                      Show this help.
USAGE
}

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
gh_bin=${GH:-gh}
plugin_lock="$root/plugins.lock"
plugin_repository=${KUMBUKA_PLUGINS_REPOSITORY:-kumbuka-me/plugins}
plugin_update_commit=${PLUGIN_UPDATE_COMMIT:-chore: update plugins}
plugin_stamp="$root/plugins/.downloaded"
plugin_download="$root/scripts/plugins/download.sh"
temporary=""
backup_lock=""
restore_lock=false

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM

  if [ "$restore_lock" = true ] && [ -n "$backup_lock" ] && [ -f "$backup_lock" ]; then
    cp "$backup_lock" "$plugin_lock"
  fi

  [ -n "$temporary" ] && rm -rf "$temporary"
  exit "$status"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
  --gh-bin)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    gh_bin=$2
    shift 2
    ;;
  --plugin-lock)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    plugin_lock=$2
    shift 2
    ;;
  --plugin-repository)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    plugin_repository=$2
    shift 2
    ;;
  --plugin-update-commit)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    plugin_update_commit=$2
    shift 2
    ;;
  --plugin-stamp)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    plugin_stamp=$2
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

case "$plugin_lock" in
/*) ;;
*) plugin_lock="$root/$plugin_lock" ;;
esac
case "$plugin_stamp" in
/*) ;;
*) plugin_stamp="$root/$plugin_stamp" ;;
esac

[ -f "$plugin_lock" ] || {
  echo "Plugin lock file not found: $plugin_lock" >&2
  exit 1
}
[ -x "$plugin_download" ] || {
  echo "Plugin download script is not executable: $plugin_download" >&2
  exit 1
}
command -v "$gh_bin" >/dev/null 2>&1 || {
  echo "GitHub CLI not found: $gh_bin" >&2
  exit 1
}
command -v git >/dev/null 2>&1 || {
  echo "git is required" >&2
  exit 1
}

git -C "$root" rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  echo "Repository root is not a Git work tree: $root" >&2
  exit 1
}

case "$plugin_lock" in
"$root"/*) plugin_lock_path=${plugin_lock#"$root"/} ;;
*)
  echo "Plugin lock file must be inside the repository: $plugin_lock" >&2
  exit 1
  ;;
esac

if ! git -C "$root" diff --cached --quiet; then
  echo "Refusing to update plugins while other changes are staged." >&2
  exit 1
fi
if ! git -C "$root" diff --quiet -- "$plugin_lock_path"; then
  echo "Refusing to overwrite uncommitted changes in $plugin_lock_path." >&2
  exit 1
fi

latest_release() {
  plugin=$1
  releases_file=$2

  awk -v prefix="$plugin/v" '
    index($0, prefix) == 1 {
      version = substr($0, length(prefix) + 1)
      if (split(version, part, ".") != 3) {
        next
      }
      if (part[1] !~ /^[0-9]+$/ || part[2] !~ /^[0-9]+$/ || part[3] !~ /^[0-9]+$/) {
        next
      }

      major = part[1] + 0
      minor = part[2] + 0
      patch = part[3] + 0

      if (!found || major > best_major ||
          (major == best_major && minor > best_minor) ||
          (major == best_major && minor == best_minor && patch > best_patch)) {
        found = 1
        best_major = major
        best_minor = minor
        best_patch = patch
      }
    }
    END {
      if (found) {
        printf "%d.%d.%d\n", best_major, best_minor, best_patch
      }
    }
  ' "$releases_file"
}

temporary=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-plugin-update.XXXXXX")
releases="$temporary/releases"
next_lock="$temporary/plugins.lock"
seen="$temporary/seen"
backup_lock="$temporary/plugins.lock.original"
: >"$seen"

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Fetching stable plugin releases from $plugin_repository..."
"$gh_bin" api \
  --paginate \
  "repos/$plugin_repository/releases?per_page=100" \
  --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name' \
  >"$releases"

: >"$next_lock"
updated=0
count=0

while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
  '' | '#'*)
    printf '%s\n' "$line" >>"$next_lock"
    continue
    ;;
  esac

  plugin=${line%%=*}
  current=${line#*=}

  if [ "$plugin" = "$line" ] || [ -z "$plugin" ] || [ -z "$current" ]; then
    echo "Invalid $plugin_lock_path entry: $line" >&2
    exit 1
  fi
  if ! printf '%s\n' "$plugin" | grep -Eq '^[a-z0-9][a-z0-9-]*$'; then
    echo "Invalid plugin name in $plugin_lock_path: $plugin" >&2
    exit 1
  fi
  if ! printf '%s\n' "$current" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "Invalid version for $plugin in $plugin_lock_path: $current" >&2
    exit 1
  fi
  if grep -Fxq "$plugin" "$seen"; then
    echo "Duplicate plugin in $plugin_lock_path: $plugin" >&2
    exit 1
  fi
  printf '%s\n' "$plugin" >>"$seen"

  latest=$(latest_release "$plugin" "$releases")
  if [ -z "$latest" ]; then
    echo "No stable release found for $plugin in $plugin_repository." >&2
    exit 1
  fi

  printf '%s=%s\n' "$plugin" "$latest" >>"$next_lock"
  count=$((count + 1))

  if [ "$current" != "$latest" ]; then
    printf '%-22s %s -> %s\n' "$plugin" "$current" "$latest"
    updated=$((updated + 1))
  fi
done <"$plugin_lock"

[ "$count" -gt 0 ] || {
  echo "No plugins configured in $plugin_lock_path" >&2
  exit 1
}

if [ "$updated" -eq 0 ]; then
  echo "All pinned plugins already use the latest stable releases."
  exit 0
fi

cp "$plugin_lock" "$backup_lock"
mv "$next_lock" "$plugin_lock"
restore_lock=true

echo "Verifying updated plugin packages..."
"$plugin_download" \
  --plugin-lock "$plugin_lock" \
  --destination "$root/plugins" \
  --plugin-repository "$plugin_repository"

mkdir -p "$(dirname "$plugin_stamp")"
touch "$plugin_stamp"
restore_lock=false

git -C "$root" add -- "$plugin_lock_path"
git -C "$root" commit -m "$plugin_update_commit" -- "$plugin_lock_path"

echo "Updated $updated plugin(s) and committed $plugin_lock_path."
