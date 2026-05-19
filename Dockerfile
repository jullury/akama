# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS builder
WORKDIR /src
COPY . .
ARG VERSION=dev
ARG BUILD_TIME
ARG BUILD_PLATFORM
RUN --mount=type=secret,id=github_client_id \
    --mount=type=secret,id=github_client_secret \
    --mount=type=secret,id=gitlab_client_id \
    --mount=type=secret,id=gitlab_client_secret \
    CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags "-s -w \
          -X github.com/jullury/akama/internal/config.GitHubClientID=$(cat /run/secrets/github_client_id) \
          -X github.com/jullury/akama/internal/config.GitHubClientSecret=$(cat /run/secrets/github_client_secret) \
          -X github.com/jullury/akama/internal/config.GitLabClientID=$(cat /run/secrets/gitlab_client_id) \
          -X github.com/jullury/akama/internal/config.GitLabClientSecret=$(cat /run/secrets/gitlab_client_secret) \
          -X github.com/jullury/akama/internal/config.Version=${VERSION} \
          -X github.com/jullury/akama/internal/config.BuildTime=${BUILD_TIME} \
          -X github.com/jullury/akama/internal/config.BuildPlatform=${BUILD_PLATFORM}" \
        -o /akama .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git docker.io nodejs npm curl bash \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 -s /bin/bash worker
USER worker
ENV NPM_CONFIG_PREFIX=/home/worker/.npm-global
ENV PATH="/home/worker/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
RUN npm install -g @anthropic-ai/claude-code opencode-ai
USER root
COPY --from=builder /akama /usr/local/bin/akama
WORKDIR /workspaces
ENTRYPOINT ["akama", "--daemon"]
