# syntax=docker/dockerfile:1

# Run builder natively on the host platform (amd64) and cross-compile.
# Without --platform=$BUILDPLATFORM, Docker emulates arm64 via QEMU for every
# RUN step in this stage, making go build 4-6x slower.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS builder
WORKDIR /src

# Copy dependency manifests first so this layer is only invalidated when
# go.mod/go.sum change, not on every source edit.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

COPY . .

# TARGETOS/TARGETARCH are auto-set by BuildKit for the requested platform.
ARG TARGETOS
ARG TARGETARCH
ARG TARGETPLATFORM
ARG VERSION=dev
ARG BUILD_TIME

RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=secret,id=github_client_id \
    --mount=type=secret,id=github_client_secret \
    --mount=type=secret,id=gitlab_client_id \
    --mount=type=secret,id=gitlab_client_secret \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
        -trimpath \
        -ldflags "-s -w \
          -X github.com/jullury/akama/internal/config.GitHubClientID=$(cat /run/secrets/github_client_id) \
          -X github.com/jullury/akama/internal/config.GitHubClientSecret=$(cat /run/secrets/github_client_secret) \
          -X github.com/jullury/akama/internal/config.GitLabClientID=$(cat /run/secrets/gitlab_client_id) \
          -X github.com/jullury/akama/internal/config.GitLabClientSecret=$(cat /run/secrets/gitlab_client_secret) \
          -X github.com/jullury/akama/internal/config.Version=${VERSION} \
          -X github.com/jullury/akama/internal/config.BuildTime=${BUILD_TIME} \
          -X github.com/jullury/akama/internal/config.BuildPlatform=${TARGETPLATFORM}" \
        -o /akama .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git docker.io nodejs npm curl bash \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 -s /bin/bash worker

USER worker
ENV NPM_CONFIG_PREFIX=/home/worker/.npm-global
ENV PATH="/home/worker/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
RUN --mount=type=cache,target=/home/worker/.npm,uid=1000,gid=1000 \
    npm install -g @anthropic-ai/claude-code opencode-ai

USER root
COPY --from=builder /akama /usr/local/bin/akama
WORKDIR /workspaces
ENTRYPOINT ["akama", "--daemon"]
