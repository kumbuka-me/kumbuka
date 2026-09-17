<p align="center">
  <img src="web/src/kumbuka.svg" alt="Kumbuka" width="480" />
</p>

<p align="center">
  <strong>Keep your knowledge close.</strong><br />
  A small, self-hosted, Markdown-first wiki for documentation.
</p>

Kumbuka is the PostgreSQL-backed server and web application. It includes authentication, revision history, search, media, collaboration, administration, themes, and the plugin runtime.

Project-oriented tooling lives in the separate [Kumbuka CLI](https://github.com/kumbuka-me/cli). Use `kumbuka-cli` for static-site builds, Markdown mirrors, and `.kumbukaplugins` management.

## Plugin development

Create, test, and build Go/WASI plugins with the [Kumbuka Plugin SDK and CLI](https://github.com/kumbuka-me/sdk). First-party plugins live in [kumbuka-me/plugins](https://github.com/kumbuka-me/plugins); bundled and installed packages use the same public API and sandboxed runtime.

## Quick start

Start Kumbuka with Docker Compose:

```sh
docker compose -f deploy/compose.yaml up -d
```

By default, Kumbuka is available at:

```text
http://localhost:8080
```

On a fresh database, open Kumbuka and create the first administrator through the setup page.

The server binary has no subcommands. Running `kumbuka` starts the server directly:

```sh
kumbuka \
  --database-url 'postgres://USER:PASS@HOST:5432/kumbuka?sslmode=disable' \
  --listen-address 0.0.0.0:8080 \
  --public-url http://localhost:8080
```

## Common settings

Environment variables use the `KUMBUKA__` prefix.

| Setting                   | Default                 | What it changes                                                                |
| ------------------------- | ----------------------- | ------------------------------------------------------------------------------ |
| `KUMBUKA__LISTEN_ADDRESS` | `127.0.0.1:8080`        | Address and port Kumbuka listens on.                                           |
| `KUMBUKA__PUBLIC_URL`     | `http://localhost:8080` | Externally visible URL of the Kumbuka installation.                            |
| `KUMBUKA__DATABASE_URL`   | —                       | PostgreSQL connection URL.                                                     |
| `KUMBUKA__LOCAL_LOGIN`    | `false`                 | Enables the local recovery login alongside the configured authentication mode. |

See the [documentation](https://kumbuka.me/) for all settings and authentication options.

## Reusable packages

Runtime packages that are intentionally shared with the standalone CLI live under `pkg/`. Server-only HTTP, authentication, routing, service composition, and deployment details remain under `internal/`.

The public packages are implementation building blocks for Kumbuka tooling; the server remains the primary application in this repository.

## CLI

Install or build [kumbuka-me/cli](https://github.com/kumbuka-me/cli) for offline/project commands:

```sh
kumbuka-cli build --help
kumbuka-cli mirror --help
kumbuka-cli plugins --help
```

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
