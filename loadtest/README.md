# Kumbuka load tests

These tests run only against `deploy/compose.loadtest.yaml`. That Compose project has a dedicated PostgreSQL volume, does not publish Kumbuka on a host port, and must never be pointed at a production deployment.

`make loadtest-prepare` starts the isolated stack and seeds a documentation site: the repository README and plugin-development README, plus 48 linked operations guides. The generated guides contain tables, checklists, code blocks, tags, internal links, and references to a reused uploaded PNG fixture. Then run one of:

```sh
make loadtest-smoke
make loadtest-read
make loadtest-stress
make loadtest-search-stress
make loadtest-render-stress
```

The stack uses Kumbuka's trusted-proxy authentication override solely inside its private Compose network. The k6 scripts provide synthetic identity headers; `seed` is an administrator and workload users are ordinary registered users. The seed reads documentation from this checkout rather than fetching a mutable public site, so benchmark inputs stay reproducible.

## Stress profiles

The stress profiles use k6's arrival-rate executors. Each iteration sends exactly one HTTP request, so a target of 1000 means approximately 1000 requested HTTP requests per second. Because the arrival rate is independent of response time, Kumbuka becoming slower does not automatically reduce the requested load.

`make loadtest-stress` is the general read profile. It focuses tightly on the observed saturation knee: after entering at 2000 requests per second, it holds 2400, 2500, 2600, 2700, 2800, 2900, and 3000 requests per second for 30 seconds each, with short ramps between levels. The VU ceiling is 3500: high enough to expose the knee, but deliberately lower than the earlier 5000-VU run so an overloaded server cannot accumulate an unnecessarily large queue. Its deterministic request mix remains 70% individual page API reads, 20% page-list requests, and 10% searches.

`make loadtest-search-stress` isolates search and ramps through 100, 250, 500, 1000, and 2000 search requests per second. Use it to expose database and search-path saturation without cheaper page reads hiding the result.

`make loadtest-render-stress` requests the browser-facing `/pages/{slug}` route and ramps through 100, 250, 500, 1000, and 2000 rendered pages per second. This exercises substantially more of the page-serving path than the JSON page API, including browser context, Markdown/plugin rendering, and HTML templates.

All stress profiles reuse one synthetic reader identity per profile instead of creating users as k6 allocates additional VUs. They also discard response bodies inside k6 after receiving them; Kumbuka still produces and transfers the complete response, but the load generator avoids retaining bodies it does not inspect.

The `dropped_iterations` threshold requires k6 to sustain the requested arrival rate. A non-zero value means k6 could not start every scheduled iteration with the configured VU ceiling. Interpret that together with latency, failures, Kumbuka metrics, PostgreSQL resource use, and host/container CPU before deciding where the bottleneck is.

## Automatic monitoring

Every stress target starts two background samplers before k6:

- a lightweight Alpine sidecar scrapes Kumbuka's `/metrics` endpoint every 5 seconds;
- a host-side sampler records `docker stats` for the Kumbuka and PostgreSQL containers every 5 seconds.

Before each stress profile, the runner restarts only the Kumbuka container and waits for `/healthz`. PostgreSQL and its seeded data stay running, while Kumbuka starts with fresh process counters, connections, goroutines, and heap state. Set `LOADTEST_RESTART_APP=false` only when you intentionally want to continue from the current process state.

Each run writes to a timestamped directory under `build/loadtest/` and updates `build/loadtest/latest` to point at the current run. The two files are:

```text
build/loadtest/latest/metrics.prom
build/loadtest/latest/docker-stats.tsv
```

The Prometheus log keeps complete scrape snapshots separated by `# sample` timestamps. The Docker stats log records CPU, memory, network I/O, block I/O, and process counts for both application containers. When k6 exits, including when a stress threshold fails, the wrapper stops both samplers and prints peak container CPU/memory together with Kumbuka's peak RSS, Go heap, goroutine count, open file descriptors, and the HTTP request-counter delta observed during the run. The request count is a delta between the first and final scrape rather than a process-lifetime counter.

Change the sampling interval when needed:

```sh
make loadtest-stress LOADTEST_MONITOR_INTERVAL=2
```

The monitoring traffic is intentionally low and `/metrics` is fetched from inside the isolated Compose network. No Kumbuka host port is required.

`make loadtest-reset` removes the load-test database volume and recreates it with fresh seed data. `make loadtest-down` stops the stack while preserving its data.
