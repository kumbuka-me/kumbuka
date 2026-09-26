#!/bin/sh
set -eu

base_url=${BASE_URL:-http://kumbuka:8080}
interval=${SAMPLE_INTERVAL:-5}

while :; do
  timestamp=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  printf '# sample %s\n' "$timestamp"
  if ! wget -qO- "${base_url}/metrics"; then
    printf '# scrape_error %s\n' "$timestamp"
  fi
  printf '\n'
  sleep "$interval"
done
