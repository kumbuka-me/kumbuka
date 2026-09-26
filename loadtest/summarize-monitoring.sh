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
      if (samples == 1) first_requests = sample_requests
      last_requests = sample_requests
    }
    /^# sample / {
      finish_sample()
      in_sample = 1
      sample_requests = 0
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
    END {
      finish_sample()
      request_delta = last_requests - first_requests
      if (request_delta < 0) request_delta = 0
      printf "  Kumbuka max goroutines:     %.0f\n", max_goroutines
      printf "  Kumbuka max open FDs:       %.0f\n", max_open_fds
      printf "  Kumbuka peak RSS:           %.1f MiB\n", max_rss / 1048576
      printf "  Kumbuka peak Go heap:       %.1f MiB\n", max_heap / 1048576
      printf "  HTTP requests during run:   %.0f\n", request_delta
    }
  ' "$metrics_file"
fi
