#!/bin/sh
set -eu

compose_file=${1:?compose file is required}
interval=${2:-5}
output=${3:?output file is required}

kumbuka_id=$(docker compose -f "$compose_file" ps -q kumbuka)
postgres_id=$(docker compose -f "$compose_file" ps -q postgres)

if [ -z "$kumbuka_id" ] || [ -z "$postgres_id" ]; then
  echo "load-test containers are not running" >&2
  exit 1
fi

printf 'timestamp\tname\tcpu_percent\tmemory_percent\tmemory_usage\tnet_io\tblock_io\tpids\n' > "$output"

while :; do
  timestamp=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  docker stats --no-stream \
    --format "$timestamp\t{{.Name}}\t{{.CPUPerc}}\t{{.MemPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}\t{{.PIDs}}" \
    "$kumbuka_id" "$postgres_id" >> "$output"
  sleep "$interval"
done
