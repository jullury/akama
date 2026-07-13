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
# Install Node.js 22 from NodeSource (Debian bookworm ships Node 18 which is
# too old for @anthropic-ai/claude-code ≥ 2.x).
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git docker.io curl bash xz-utils \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install -g pnpm \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 -s /bin/bash worker

ARG TARGETARCH=amd64

USER worker
ENV NPM_CONFIG_PREFIX=/home/worker/.npm-global
ENV PATH="/home/worker/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
RUN --mount=type=cache,target=/home/worker/.npm,uid=1000,gid=1000 \
    npm install -g @anthropic-ai/claude-code opencode-ai

# Install RTK (Rust Token Killer) — CLI proxy that reduces LLM token usage 60-90%.
# Binary is placed in /usr/local/bin so both worker and akama users can execute it.
ARG RTK_VERSION=0.43.0
USER root
RUN if [ "${TARGETARCH}" = "arm64" ]; then RTK_ARCH="aarch64-unknown-linux-gnu"; \
    else RTK_ARCH="x86_64-unknown-linux-musl"; fi && \
    curl -fsSL "https://github.com/rtk-ai/rtk/releases/download/v${RTK_VERSION}/rtk-${RTK_ARCH}.tar.gz" \
        | tar xz -C /usr/local/bin rtk && \
    chmod +x /usr/local/bin/rtk

# Install mise — runtime version manager used by cloned repos (.mise.toml).
# Placed in /usr/local/bin so it's on the default PATH for all users.
RUN curl -fsSL https://mise.run | MISE_INSTALL_PATH=/usr/local/bin/mise sh

COPY --from=builder /akama /usr/local/bin/akama
WORKDIR /workspaces
ENTRYPOINT ["akama", "--daemon"]
