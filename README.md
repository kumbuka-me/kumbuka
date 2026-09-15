<p align="center">
  <img src="web/src/kumbuka.svg" alt="Kumbuka" width="480" />
</p>

<p align="center">
  <strong>Keep your knowledge close.</strong><br />
  A small, self-hosted, Markdown-first wiki for documentation.
</p>

Kumbuka gives you a focused place to write, organize, search, and share documentation. It runs as a single Go application backed by PostgreSQL and includes authentication, revision history, search, media, collaboration, and administration.

The same binary can also publish Markdown as a read-only static documentation site without PostgreSQL or a running Kumbuka server.

## Plugin development

Create, test and build Go/WASI plugins with the [Kumbuka Plugin SDK and CLI](https://github.com/kumbuka-me/sdk). First-party plugins live in [kumbuka-me/plugins](https://github.com/kumbuka-me/plugins); bundled and installed packages use the same public API and sandboxed runtime.

## Screenshots

### Documentation dashboard

![Kumbuka dashboard populated with the project documentation](docs/assets/screenshots/dashboard.png)

### Markdown editor

![Kumbuka Markdown editor showing documentation in split view](docs/assets/screenshots/editor.png)

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

For production deployments, authentication, Kubernetes, static sites, and other configuration, see the [documentation](https://kumbuka.me/).

## Common settings

Environment variables use the `KUMBUKA__` prefix.

| Setting                   | Default                 | What it changes                                                                |
| ------------------------- | ----------------------- | ------------------------------------------------------------------------------ |
| `KUMBUKA__LISTEN_ADDRESS` | `127.0.0.1:8080`        | Address and port Kumbuka listens on.                                           |
| `KUMBUKA__PUBLIC_URL`     | `http://localhost:8080` | Externally visible URL of the Kumbuka installation.                            |
| `KUMBUKA__DATABASE_URL`   | —                       | PostgreSQL connection URL.                                                     |
| `KUMBUKA__LOCAL_LOGIN`    | `false`                 | Enables the local recovery login alongside the configured authentication mode. |

See the [configuration guide](https://kumbuka.me/configuration/) for all settings and authentication options.

## Documentation

The full documentation is published at:

**[gi8lino.github.io/kumbuka](https://kumbuka.me/)**

## License

Kumbuka is licensed under the [Apache License, Version 2.0](LICENSE).

## Static-site plugins

Static builds use a project-level `.kumbukaplugins` file to pin the plugin packages that the site may use. The file is independent from Kumbuka's `plugins.lock`: `plugins.lock` pins packages bundled into the Kumbuka distribution, while `.kumbukaplugins` pins dependencies of one static content project.

```toml
format = 1

[[plugin]]
id = "me.kumbuka.mermaid"
repository = "kumbuka-me/plugins"
tag_prefix = "mermaid/v"
asset = "mermaid"
version = "1.0.1"

[[plugin]]
id = "com.example.chart"
repository = "example/kumbuka-chart"
tag_prefix = "v"
asset = "kumbuka-chart"
version = "2.3.0"
```

`repository` is a GitHub `owner/repository`, `tag_prefix` is prepended to the version to form the release tag, and `asset` is the base name of `<asset>-<version>.kumbukaplugin` and its `.sha256` file. Matching bundled packages are reused directly; other packages are verified and cached below the operating system's user cache directory.

Manage the file with:

```bash
kumbuka plugins list
kumbuka plugins sync
kumbuka plugins add \
  --id com.example.chart \
  --repository example/kumbuka-chart \
  --plugin-version 2.3.0
kumbuka plugins remove --id com.example.chart
```

`kumbuka build` reads all declared package manifests first, analyzes the Markdown using each module's declarative `usage` rules, adds transitive plugin dependencies, and starts only that selected package set. Unused declared WASM plugins are therefore not compiled or initialized, and unused browser assets are not copied into the generated site. Modules without usage rules are intentionally treated as global and are loaded whenever their plugin is declared.

## License

Licensed under the [Apache License 2.0](./LICENSE).
