<p align="center">
  <img src="web/src/kumbuka.svg" alt="Kumbuka" width="480" />
</p>

<p align="center">
  <strong>Keep your knowledge close.</strong><br />
  A small, self-hosted, Markdown-first wiki for documentation.
</p>

Kumbuka is a self-hosted knowledge platform backed by PostgreSQL. It provides a clean place for documentation, notes, and team knowledge while keeping your data under your control.

For installation, configuration, authentication, plugins, and administration, see the **[Kumbuka documentation](https://kumbuka.me/)**.

## Quick start

Start Kumbuka with Docker Compose:

```sh
docker compose -f deploy/compose.yaml up -d
```

Then open:

```text
http://localhost:8080
```

On a fresh database, Kumbuka will guide you through creating the first administrator.

Example Kubernetes manifests are available in [`deploy/kubernetes`](deploy/kubernetes).

You can also run the server directly:

```sh
kumbuka \
  --database-url 'postgres://USER:PASS@HOST:5432/kumbuka?sslmode=disable' \
  --listen-address 0.0.0.0:8080 \
  --public-url http://localhost:8080
```

## Configuration

Kumbuka can be configured using command-line flags or environment variables. Environment variables use the `KUMBUKA__` prefix.

| Flag | Environment variable | Default |
| --- | --- | --- |
| `--listen-address` | `KUMBUKA__LISTEN_ADDRESS` | `127.0.0.1:8080` |
| `--management-listen-address` | `KUMBUKA__MANAGEMENT_LISTEN_ADDRESS` | `127.0.0.1:8081` |
| `--public-url` | `KUMBUKA__PUBLIC_URL` | `http://localhost:8080` |
| `--route-prefix` | `KUMBUKA__ROUTE_PREFIX` | empty |
| `--database-url` | `KUMBUKA__DATABASE_URL` | — |
| `--read-only` | `KUMBUKA__READ_ONLY` | `false` |

The management listener exposes `GET /healthz`, `GET /readyz`, and, unless disabled, `GET /metrics`. It is separate from the public application listener and is not mounted below `--route-prefix`.

For all available settings and deployment options, see **[kumbuka.me](https://kumbuka.me/configuration/runtime/)**.

## Ecosystem

- [Kumbuka CLI](https://github.com/kumbuka-me/cli) — builds, Markdown mirrors, and plugin management
- [Plugin SDK](https://github.com/kumbuka-me/sdk) — build Go/WASI plugins for Kumbuka
- [Official plugins](https://github.com/kumbuka-me/plugins) — first-party Kumbuka plugins

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
