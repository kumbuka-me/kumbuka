# syntax=docker/dockerfile:1.27

FROM node:24-alpine AS frontend
WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

COPY tsconfig.json ./
COPY scripts/web ./scripts/web
COPY web/src ./web/src

RUN ./scripts/web/build.sh


FROM alpine:3.24 AS plugins
WORKDIR /src

RUN apk add --no-cache curl unzip

COPY plugins.lock ./
COPY scripts/plugins/download.sh ./scripts/plugins/download.sh

RUN ./scripts/plugins/download.sh

# Build the Kumbuka server binary.
FROM golang:1.27 AS prep

ENV CGO_ENABLED=0

WORKDIR /workspace

# Copy the Go module manifests first so dependency downloads can be cached.
COPY go.mod go.sum ./

# Download modules before introducing build-specific arguments or source files.
# This keeps the dependency layer stable across normal source and version changes.
RUN --mount=type=cache,target=/go/pkg/mod \
  go mod download

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG LDFLAGS="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}"

# Copy the Go source and templates.
COPY cmd/kumbuka/ cmd/kumbuka/
COPY internal/ internal/
COPY pkg/ pkg/
COPY scripts/generate-icons/ scripts/generate-icons/
COPY web/ web/
COPY plugins/packages.go plugins/packages.go
COPY --from=plugins /src/plugins/*.kumbukaplugin plugins/
COPY --from=frontend /src/web/dist web/dist

# Build the binary.
# TARGETARCH defaults to the builder architecture for regular Docker builds,
# but can be set by buildx for cross-platform builds.
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go generate ./pkg/icons \
  && GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} \
  go build \
  -ldflags="$LDFLAGS" \
  -a \
  -o kumbuka \
  cmd/kumbuka/main.go

# Create writable runtime directories owned by the root group.
# The setgid bit keeps new files/directories in group 0, which supports
# OpenShift's arbitrary UID model while still running as a non-root user.
RUN install -d -o 0 -g 0 -m 2775 /outfs/app /outfs/tmp


# Use distroless as a minimal base image for the Kumbuka server binary.
# Refer to https://github.com/GoogleContainerTools/distroless for more details.
FROM gcr.io/distroless/static:nonroot

COPY --from=prep /workspace/kumbuka /kumbuka
COPY --from=prep /outfs/app /app
COPY --from=prep /outfs/tmp /tmp

ENV HOME=/tmp

WORKDIR /app

# Run as a non-root user by default.
# Use GID 0 so the process can write to root-group-owned writable paths,
# which keeps the image compatible with OpenShift's arbitrary UID model.
USER 65532:0

ENTRYPOINT ["/kumbuka"]
