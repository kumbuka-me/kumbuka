#!/bin/bash
# Exit immediately if a command exits with a non-zero status
set -e

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
  --refresh-command)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    refresh_command=$2
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

echo "===================================================="
echo "Checking dependencies and repository status..."
echo "===================================================="

# Check if GitHub CLI is installed
if ! command -v "$gh_bin" >/dev/null 2>&1; then
  echo "ERROR: Missing $gh_bin. Install GitHub CLI first." >&2
  exit 1
fi

# Prevent running if changes are already staged
if ! git diff --cached --quiet; then
  echo "ERROR: Refusing to update plugins while other changes are staged." >&2
  exit 1
fi

# Prevent overwriting modified but uncommitted lockfiles
if ! git diff --quiet -- "$plugin_lock"; then
  echo "ERROR: Refusing to overwrite uncommitted changes in $plugin_lock." >&2
  exit 1
fi

# Create temporary files for processing
releases=$(mktemp)
next=$(mktemp)

cleanup() {
  status=$?
  trap - EXIT INT TERM
  rm -f "$releases" "$next"
  exit "$status"
}
trap cleanup EXIT INT TERM

echo "Fetching releases from GitHub..."
"$gh_bin" api \
  --paginate \
  "repos/$plugin_repository/releases?per_page=100" \
  --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name' \
  >"$releases"

# Initialize next lockfile as empty
: >"$next"
updated=0

echo "Processing $plugin_lock entries..."
echo "----------------------------------------------------"

while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
  "" | \#*)
    printf '%s\n' "$line" >>"$next"
    continue
    ;;
  esac

  plugin=${line%%=*}
  current=${line#*=}

  if [ "$plugin" = "$line" ] || [ -z "$plugin" ] || [ -z "$current" ]; then
    echo "ERROR: Invalid $plugin_lock entry: $line" >&2
    exit 1
  fi

  tag=$(awk -v prefix="$plugin/v" \
    'index($0, prefix) == 1 { print; exit }' \
    "$releases")

  if [ -z "$tag" ]; then
    echo "ERROR: No stable release found for $plugin in $plugin_repository." >&2
    exit 1
  fi

  latest=${tag#"$plugin"/v}
  printf '%s=%s\n' "$plugin" "$latest" >>"$next"

  if [ "$current" != "$latest" ]; then
    printf '%-22s %s -> %s\n' "$plugin" "$current" "$latest"
    updated=1
  fi
done <"$plugin_lock"

echo "----------------------------------------------------"

if [ "$updated" -eq 0 ]; then
  echo "All pinned plugins already use the latest stable releases."
  exit 0
fi

echo "Updating $plugin_lock and running refresh..."
mv "$next" "$plugin_lock"

# Execute the refresh logic (previously $(MAKE) plugins-refresh)
if [ -x "$refresh_command" ]; then
  "$refresh_command"
else
  echo "Warning: $refresh_command not found or not executable. Skipping refresh step."
fi

echo "Staging changes and committing..."
git add "$plugin_lock"
git add -u -- plugins
git commit -m "$plugin_update_commit"

echo "===================================================="
echo "SUCCESS: Plugins updated and committed."
echo "===================================================="
