.DEFAULT_GOAL := help

## Frontend

WEB_BUILD := scripts/web/build.sh
CSS_BUILD := scripts/web/build-css.sh
CSS_ENTRY := web/src/css/app.css
CSS_OUTPUT := web/dist/css/app.css
NODE ?= node
NPM ?= npm
NPX ?= npx
TSC ?= ./node_modules/.bin/tsc
NODE_MODULES := node_modules/.package-lock.json

## Plugins

PLUGIN_LOCK := plugins.lock
PLUGIN_DOWNLOAD := scripts/plugins/download.sh
PLUGIN_UPDATE := scripts/plugins/update.sh
PLUGIN_STAMP := plugins/.downloaded
PLUGIN_REPOSITORY ?= kumbuka-me/plugins
PLUGIN_UPDATE_COMMIT ?= chore: update plugins
GH ?= gh

## Tool Versions

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.14.0

# renovate: datasource=github-releases depName=gi8lino/dev-tools
DEV_TOOLS_VERSION ?= v0.9.0

## Shared development tools

include bin/dev-tools.mk
include $(call dev-tools-module,tag)
include $(call dev-tools-module,port)
include $(call dev-tools-module,browser)
include $(call dev-tools-module,help)

## Project-local tools

GOLANGCI_LINT := bin/golangci-lint
FAVICON_GENERATE := $(DEV_TOOLS_BIN)/favicon-generate
SVG_TO_PNG := $(DEV_TOOLS_BIN)/svg-to-png

## Build Configuration

BINARY ?= kumbuka
COMMAND ?= ./cmd/kumbuka
GO_TEST_RACE_FLAGS ?= -p=2 -parallel=4
BROWSER_TEST_CONCURRENCY ?= 2
RACE_TEST_PACKAGES := ./pkg/markdown ./pkg/plugin ./pkg/plugin/wasm ./internal/postgres
RACE_TEST_PATTERN := ^( \
	TestMacroCapabilitiesStayRequestLocal| \
	TestRegistryConcurrentSnapshotsAndRemoval| \
	TestWASMRequestsAreIsolatedAndSerialized| \
	TestCapabilitiesUseCurrentRequestAndRecoverFromHostPanic| \
	TestUpgradeDuringRenderingKeepsWholeSnapshotAlive| \
	TestConcurrentStartupMigrations \
)$$
RUN_ARGS ?=
BUILD_VERSION ?= dev
BUILD_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS ?= -s -w -X main.Version=$(BUILD_VERSION) -X main.Commit=$(BUILD_COMMIT)

## Debugging

COMPOSE_PROJECT ?= $(notdir $(CURDIR))
COMPOSE_FILE := deploy/compose.yaml
DB_CONTAINER_NAME ?= postgres
PDF_CONTAINER_NAME ?= html2pdf
MAILBRIDGE_CONTAINER_NAME ?= mailbridge

KUMBUKA_ASSIGNED_PORT ?= $(call dev-port,app)
DB_ASSIGNED_PORT ?= $(call dev-port,postgres)
PDF_ASSIGNED_PORT ?= $(call dev-port,pdf)
MAILBRIDGE_ASSIGNED_PORT ?= $(call dev-port,mailbridge)

## Assets

FAVICON_SOURCE ?= web/src/favicon.svg
FAVICON_OUTPUT ?= web/src
FAVICON_SIZES ?= 16x16 32x32

LOGO_SOURCE ?= web/src/favicon.svg
LOGO_PNG ?= build/kumbuka.png
LOGO_PNG_WIDTH ?= 1280

## Formatting

PRETTIER_MD_SOURCES := README.md "**/*.md"


## Migration

MIGRATION_DIR := internal/postgres/migrations
MIGRATION_OUTPUT_DIR := build/migrations
MIGRATION_MERGE := scripts/db/merge_migrations.sh
MIGRATION_CLEANUP := scripts/db/cleanup_migrations.sh
MIGRATION_HELPERS := scripts/db/migration_helpers.sh

##@ Migration

.PHONY: merge-migrations
merge-migrations: $(MIGRATION_MERGE) $(MIGRATION_HELPERS) ## Merge all migrations into the next validated baseline.
	$(MIGRATION_MERGE) --migrations "$(MIGRATION_DIR)" --output-dir "$(MIGRATION_OUTPUT_DIR)"

.PHONY: cleanup-migrations
cleanup-migrations: merge-migrations $(MIGRATION_CLEANUP) $(MIGRATION_HELPERS) ## Replace existing migrations with the validated generated baseline.
	$(MIGRATION_CLEANUP) --migrations "$(MIGRATION_DIR)" --generated-dir "$(MIGRATION_OUTPUT_DIR)"


##@ Development

.PHONY: ports-reset
ports-reset: $(DEV_PORT) ## Clear saved ports after stopping local services.
	$(call run-tool,$(DEV_PORT),--reset)

.PHONY: ports
ports: $(DEV_PORT) ## Print selected local development ports.
	@$(DEV_PORT) app --port "$(KUMBUKA_ASSIGNED_PORT)" > /dev/null
	@$(DEV_PORT) postgres --port "$(DB_ASSIGNED_PORT)" > /dev/null
	@$(DEV_PORT) pdf --port "$(PDF_ASSIGNED_PORT)" > /dev/null
	@echo "Kumbuka: http://127.0.0.1:$(KUMBUKA_ASSIGNED_PORT)/"
	@echo "Postgres: 127.0.0.1:$(DB_ASSIGNED_PORT)"
	@echo "PDF: http://127.0.0.1:$(PDF_ASSIGNED_PORT)/render"

.PHONY: dev-build
dev-build: ports
	$(MAKE) generate web

.PHONY: generate
generate: plugins ## Generate application source files.
	go generate ./pkg/icons

.PHONY: check-generated
check-generated: generate ## Verify committed generated files are current.
	@test -z "$$(git status --porcelain -- pkg/icons/catalog_gen.go)"

.PHONY: css
css: ## Bundle split CSS sources into web/dist/css/app.css.
	@CSS_ENTRY="$(CSS_ENTRY)" CSS_OUTPUT="$(CSS_OUTPUT)" $(CSS_BUILD)

.PHONY: web
web: $(NODE_MODULES) ## Build the frontend distribution from web/src.
	@CSS_BUILD="$(CSS_BUILD)" CSS_ENTRY="$(CSS_ENTRY)" CSS_OUTPUT="$(CSS_OUTPUT)" TSC="$(TSC)" $(WEB_BUILD)

.PHONY: check-web
check-web: web ## Build the frontend and verify browser assets.
	@test -s "$(CSS_OUTPUT)"
	@test -s web/dist/sw.js
	@test -z "$$(find web/dist -type f -name '*.ts' -print -quit)"
	@find web/dist/js -type f -name '*.js' -exec $(NODE) --check {} \;
	@$(NODE) --check web/dist/sw.js

.PHONY: typecheck
typecheck: $(NODE_MODULES) ## Type-check all authored TypeScript without emitting files.
	$(TSC) -p tsconfig.json --noEmit
	$(TSC) -p web/src/ts/service-worker/tsconfig.json --noEmit
	$(TSC) -p test/ts/tsconfig.json --noEmit

.PHONY: download
download: $(NODE_MODULES) dev-tools plugins ## Download all project dependencies.
	go mod download

.PHONY: postgres
postgres: ports ## Run postgres locally.
	@echo "Starting Postgres on dynamic host port: $(DB_ASSIGNED_PORT)"
	@KUMBUKA_POSTGRES_PORT=$(DB_ASSIGNED_PORT) docker compose -f $(COMPOSE_FILE) -p $(COMPOSE_PROJECT) up -d --wait --wait-timeout 60 $(DB_CONTAINER_NAME)

.PHONY: html-pdf
html-pdf: ports ## Run html2pdf locally.
	@KUMBUKA_PDF_PORT=$(PDF_ASSIGNED_PORT) docker compose -f $(COMPOSE_FILE) -p $(COMPOSE_PROJECT) up -d $(PDF_CONTAINER_NAME)

.PHONY: mailbridge
mailbridge: ports ## Run mailbridge locally.
	@KUMBUKA_MAILBRIDGE_PORT=$(MAILBRIDGE_ASSIGNED_PORT) docker compose -f $(COMPOSE_FILE) -p $(COMPOSE_PROJECT) up -d $(MAILBRIDGE_CONTAINER_NAME)

.PHONY: serve
serve: ports ## Run Kumbuka using the saved ports.
	@echo "Starting Kumbuka application..."
	@KUMBUKA_POSTGRES_PORT=$(DB_ASSIGNED_PORT) go run $(COMMAND) \
		--debug \
		--access-log \
		--listen-address="127.0.0.1:$(KUMBUKA_ASSIGNED_PORT)" \
		--log-format text \
		--pdf-url="http://127.0.0.1:$(PDF_ASSIGNED_PORT)/render" \
		--database-url="postgres://kumbuka:kumbuka@127.0.0.1:$(DB_ASSIGNED_PORT)/kumbuka?sslmode=disable" \
		$(RUN_ARGS)

.PHONY: open
open: ports $(OPEN_BROWSER) ## Open Kumbuka in the browser.
	$(call run-tool,$(OPEN_BROWSER),"http://127.0.0.1:$(KUMBUKA_ASSIGNED_PORT)/")

.PHONY: run
run: dev-build html-pdf mailbridge postgres $(OPEN_BROWSER) ## Build, start services, and run Kumbuka.
	@$(OPEN_BROWSER) "http://127.0.0.1:$(KUMBUKA_ASSIGNED_PORT)/" & \
	browser_pid=$$!; \
	trap 'kill "$$browser_pid" 2>/dev/null || true' EXIT; \
	$(MAKE) serve

.PHONY: build
build: generate web ## Build the Kumbuka binary.
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(COMMAND)

.PHONY: vet
vet: generate web ## Run Go static analysis.
	go vet ./...

.PHONY: clean
clean: ## Clean up generated application files.
	rm -f $(BINARY) coverage.out coverage.html
	rm -f plugins/*.kumbukaplugin
	rm -f "$(PLUGIN_STAMP)"
	rm -rf web/dist build

##@ testing

.PHONY: test-web
test-web: check-web ## Compile and run the TypeScript frontend unit tests.
	@set -eu; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT INT TERM; \
	$(TSC) -p test/ts/tsconfig.json --outDir "$$tmp"; \
	$(NODE) --test "$$tmp"/test/ts/*.test.js

.PHONY: test-browser
test-browser: check-web ## Run browser regressions in Chrome.
	$(NODE) --test --test-concurrency=$(BROWSER_TEST_CONCURRENCY) test/browser/*.test.mjs

.PHONY: test
test: test-web vet ## Run frontend and backend unit tests.
	go test -count=1 -timeout=3m ./...

.PHONY: test-race
test-race: ## Run concurrency-sensitive Go tests with the race detector.
	go test -race -count=1 -timeout=1m $(GO_TEST_RACE_FLAGS) $(RACE_TEST_PACKAGES) -run '$(RACE_TEST_PATTERN)'

.PHONY: cover
cover: test-web plugins ## Display Go test coverage.
	go test -coverprofile=coverage.out -covermode=set -count=1 -timeout=3m ./...
	go tool cover -html=coverage.out


##@ plugins

.PHONY: plugins
plugins: $(PLUGIN_STAMP) ## Download the pinned first-party plugin packages.

$(PLUGIN_STAMP): $(PLUGIN_LOCK) $(PLUGIN_DOWNLOAD)
	$(PLUGIN_DOWNLOAD)
	@touch "$(PLUGIN_STAMP)"

.PHONY: plugins-refresh
plugins-refresh: ## Re-download all pinned first-party plugin packages.
	rm -f "$(PLUGIN_STAMP)"
	$(MAKE) plugins

.PHONY: plugins-update
plugins-update: $(PLUGIN_UPDATE) $(PLUGIN_DOWNLOAD) ## Update pinned plugins to their latest stable releases and commit them.
	$(PLUGIN_UPDATE) \
		--gh-bin "$(GH)" \
		--plugin-lock "$(PLUGIN_LOCK)" \
		--plugin-repository "$(PLUGIN_REPOSITORY)" \
		--plugin-update-commit "$(PLUGIN_UPDATE_COMMIT)" \
		--plugin-stamp "$(PLUGIN_STAMP)"

##@ Assets

.PHONY: favicon
favicon: $(FAVICON_GENERATE) $(FAVICON_SOURCE) ## Generate PNG favicons from the canonical SVG.
	$(call run-tool,$(FAVICON_GENERATE),--apple-touch "$(FAVICON_SOURCE)" "$(FAVICON_OUTPUT)" $(FAVICON_SIZES))

.PHONY: logo-png
logo-png: $(SVG_TO_PNG) $(LOGO_SOURCE) ## Generate a PNG version of the Kumbuka logo.
	$(call run-tool,$(SVG_TO_PNG),--width "$(LOGO_PNG_WIDTH)" "$(LOGO_SOURCE)" "$(LOGO_PNG)")


##@ Formatting

.PHONY: fmt
fmt: fmt-web fmt-templates fmt-go fmt-md ## Format all supported files.

.PHONY: fmt-web
fmt-web: $(NODE_MODULES) ## Format CSS and TypeScript source files.
	$(NPX) prettier --write \
		"web/src/**/*.css" \
		"web/src/**/*.ts" \
		"test/**/*.ts"

.PHONY: fmt-templates
fmt-templates: ## Format Go HTML templates.
	djlint web/src/templates --reformat

.PHONY: fmt-go
fmt-go: generate web ## Format Go code.
	go fmt ./...

.PHONY: fmt-md
fmt-md: $(NODE_MODULES) ## Format Markdown files.
	$(NPX) prettier --write --prose-wrap never $(PRETTIER_MD_SOURCES)

.PHONY: check-templates
check-templates: ## Check Go HTML template formatting.
	djlint web/src/templates --check

.PHONY: lint
lint: typecheck check-web lint-go ## Run all linters and formatting checks.

.PHONY: lint-go
lint-go: generate web golangci-lint ## Run golangci-lint.
	$(call run-tool,$(GOLANGCI_LINT),run)

.PHONY: lint-fix
lint-fix: generate web golangci-lint ## Run golangci-lint and apply fixes.
	$(call run-tool,$(GOLANGCI_LINT),run --fix)


##@ Dependencies

$(NODE_MODULES): package.json package-lock.json
	$(NPM) ci

$(FAVICON_GENERATE): | $(DEV_TOOLS_BIN)
	$(call download-dev-tool,favicon-generate,$@)

$(SVG_TO_PNG): | $(DEV_TOOLS_BIN)
	$(call download-dev-tool,svg-to-png,$@)

.PHONY: dev-tools
dev-tools: $(DEV_PORT) $(OPEN_BROWSER) $(DEV_TAG) $(MAKE_HELP) $(GO_INSTALL_TOOL) $(FAVICON_GENERATE) $(SVG_TO_PNG) ## Download the pinned development tools.

.PHONY: golangci-lint
golangci-lint: $(GO_INSTALL_TOOL) ## Download golangci-lint locally if necessary.
	@$(GO_INSTALL_TOOL) \
		--target "$(GOLANGCI_LINT)" \
		--package github.com/golangci/golangci-lint/v2/cmd/golangci-lint \
		--tool-version "$(GOLANGCI_LINT_VERSION)"
