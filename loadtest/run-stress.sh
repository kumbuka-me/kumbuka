#!/bin/sh
set -eu

compose_file=${1:?compose file is required}
profile=${2:?profile name is required}
k6_script=${3:?k6 script is required}
interval=${LOADTEST_MONITOR_INTERVAL:-5}
restart_app=${LOADTEST_RESTART_APP:-true}
results_root=${LOADTEST_RESULTS_DIR:-build/loadtest}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
timestamp=$(date -u '+%Y%m%dT%H%M%SZ')
run_name="${profile}-${timestamp}"
output_dir="${results_root}/${run_name}"
stats_file="${output_dir}/docker-stats.tsv"
metrics_file="${output_dir}/metrics.prom"
metrics_log_pid=
stats_pid=
cleaned=0

mkdir -p "$output_dir"
rm -f "${results_root}/latest"
ln -s "$run_name" "${results_root}/latest"

compose() {
  docker compose -f "$compose_file" "$@"
}

wait_for_kumbuka() {
  compose --profile loadtest run --rm --no-deps --entrypoint /bin/sh metrics-sampler -c '
    attempts=0
    until wget -qO- "$BASE_URL/healthz" >/dev/null 2>&1; do
      attempts=$((attempts + 1))
      if [ "$attempts" -ge 60 ]; then
        echo "Kumbuka did not become healthy after restart." >&2
        exit 1
      fi
      sleep 1
    done
  ' >/dev/null
}

cleanup() {
  if [ "$cleaned" -eq 1 ]; then
    return
  fi
  cleaned=1

  set +e
  if [ -n "$stats_pid" ]; then
    kill "$stats_pid" 2>/dev/null
    wait "$stats_pid" 2>/dev/null
  fi
  compose --profile loadtest stop metrics-sampler >/dev/null 2>&1
  if [ -n "$metrics_log_pid" ]; then
    wait "$metrics_log_pid" 2>/dev/null
  fi
  compose --profile loadtest rm -f metrics-sampler >/dev/null 2>&1

  "$script_dir/summarize-monitoring.sh" "$stats_file" "$metrics_file"
  printf '\nMonitoring data: %s\n' "$output_dir"
  printf '  Docker stats: %s\n' "$stats_file"
  printf '  Prometheus:   %s\n' "$metrics_file"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ "$restart_app" != "false" ]; then
  printf 'Restarting Kumbuka for a clean process baseline...\n'
  compose restart kumbuka >/dev/null
  wait_for_kumbuka
fi

export LOADTEST_MONITOR_INTERVAL="$interval"
compose --profile loadtest rm -sf metrics-sampler >/dev/null 2>&1 || true
compose --profile loadtest up -d --no-deps metrics-sampler
compose --profile loadtest logs --no-color --no-log-prefix -f metrics-sampler > "$metrics_file" 2>&1 &
metrics_log_pid=$!
"$script_dir/docker-stats.sh" "$compose_file" "$interval" "$stats_file" &
stats_pid=$!

printf 'Monitoring every %ss in %s\n' "$interval" "$output_dir"
printf 'Running %s\n\n' "$profile"

set +e
compose --profile loadtest run --rm k6 run "$k6_script"
status=$?
set -e
exit "$status"
