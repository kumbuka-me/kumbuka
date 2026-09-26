#!/bin/sh
set -eu

stats_file=${1:?docker stats file is required}
metrics_file=${2:?metrics file is required}

printf '\nMonitoring summary\n'

if [ -s "$stats_file" ]; then
  awk -F '\t' '
    NR == 1 { next }
    {
      cpu = $3
      mem = $4
      gsub(/%/, "", cpu)
      gsub(/%/, "", mem)
      if (!( $2 in seen ) || cpu + 0 > max_cpu[$2]) max_cpu[$2] = cpu + 0
      if (!( $2 in seen ) || mem + 0 > max_mem[$2]) max_mem[$2] = mem + 0
      seen[$2] = 1
    }
    END {
      for (name in seen)
        printf "  %-36s peak CPU %7.2f%%   peak memory %6.2f%%\n", name, max_cpu[name], max_mem[name]
    }
  ' "$stats_file" | sort
fi

if [ -s "$metrics_file" ]; then
  awk '
    function finish_sample() {
      if (!in_sample) return
      samples++
      if (samples == 1) {
        first_requests = sample_requests
        first_empty_acquires = sample_empty_acquires
        first_canceled_acquires = sample_canceled_acquires
        first_acquire_seconds = sample_acquire_seconds
        first_empty_wait_seconds = sample_empty_wait_seconds
      }
      last_requests = sample_requests
      last_empty_acquires = sample_empty_acquires
      last_canceled_acquires = sample_canceled_acquires
      last_acquire_seconds = sample_acquire_seconds
      last_empty_wait_seconds = sample_empty_wait_seconds
    }
    /^# sample / {
      finish_sample()
      in_sample = 1
      sample_requests = 0
      sample_empty_acquires = 0
      sample_canceled_acquires = 0
      sample_acquire_seconds = 0
      sample_empty_wait_seconds = 0
      next
    }
    /^kumbuka_http_requests_total\{/ { sample_requests += $2 }
    /^go_goroutines / {
      if ($2 + 0 > max_goroutines) max_goroutines = $2 + 0
    }
    /^process_open_fds / {
      if ($2 + 0 > max_open_fds) max_open_fds = $2 + 0
    }
    /^process_resident_memory_bytes / {
      if ($2 + 0 > max_rss) max_rss = $2 + 0
    }
    /^go_memstats_heap_alloc_bytes / {
      if ($2 + 0 > max_heap) max_heap = $2 + 0
    }
    /^kumbuka_postgres_pool_connections\{state="acquired"\}/ {
      if ($2 + 0 > max_db_acquired) max_db_acquired = $2 + 0
    }
    /^kumbuka_postgres_pool_connections\{state="total"\}/ {
      if ($2 + 0 > max_db_total) max_db_total = $2 + 0
    }
    /^kumbuka_postgres_pool_max_connections / { db_max = $2 + 0 }
    /^kumbuka_postgres_pool_empty_acquires_total / { sample_empty_acquires = $2 + 0 }
    /^kumbuka_postgres_pool_canceled_acquires_total / { sample_canceled_acquires = $2 + 0 }
    /^kumbuka_postgres_pool_acquire_duration_seconds_total / { sample_acquire_seconds = $2 + 0 }
    /^kumbuka_postgres_pool_empty_acquire_wait_seconds_total / { sample_empty_wait_seconds = $2 + 0 }
    END {
      finish_sample()
      request_delta = last_requests - first_requests
      empty_delta = last_empty_acquires - first_empty_acquires
      canceled_delta = last_canceled_acquires - first_canceled_acquires
      acquire_delta = last_acquire_seconds - first_acquire_seconds
      empty_wait_delta = last_empty_wait_seconds - first_empty_wait_seconds
      if (request_delta < 0) request_delta = 0
      if (empty_delta < 0) empty_delta = 0
      if (canceled_delta < 0) canceled_delta = 0
      if (acquire_delta < 0) acquire_delta = 0
      if (empty_wait_delta < 0) empty_wait_delta = 0
      printf "  Kumbuka max goroutines:     %.0f\n", max_goroutines
      printf "  Kumbuka max open FDs:       %.0f\n", max_open_fds
      printf "  Kumbuka peak RSS:           %.1f MiB\n", max_rss / 1048576
      printf "  Kumbuka peak Go heap:       %.1f MiB\n", max_heap / 1048576
      printf "  HTTP requests during run:   %.0f\n", request_delta
      if (db_max > 0 || max_db_total > 0) {
        printf "  PostgreSQL pool peak:       %.0f acquired / %.0f total / %.0f max\n", max_db_acquired, max_db_total, db_max
        printf "  PostgreSQL empty acquires:  %.0f during run\n", empty_delta
        printf "  PostgreSQL canceled acquires: %.0f during run\n", canceled_delta
        printf "  PostgreSQL acquire time:    %.3f s during run\n", acquire_delta
        printf "  PostgreSQL empty wait time: %.3f s during run\n", empty_wait_delta
      }
    }
  ' "$metrics_file"
fi
