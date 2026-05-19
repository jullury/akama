FROM golang:1.23-bookworm AS builder
WORKDIR /src
COPY . .
ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG BUILD_PLATFORM=linux/amd64
ARG GITHUB_CLIENT_ID
ARG GITHUB_CLIENT_SECRET
ARG GITLAB_CLIENT_ID
ARG GITLAB_CLIENT_SECRET
RUN go build -ldflags "\
	-X github.com/jullury/akama/internal/config.GitHubClientID=${GITHUB_CLIENT_ID} \
	-X github.com/jullury/akama/internal/config.GitHubClientSecret=${GITHUB_CLIENT_SECRET} \
	-X github.com/jullury/akama/internal/config.GitLabClientID=${GITLAB_CLIENT_ID} \
	-X github.com/jullury/akama/internal/config.GitLabClientSecret=${GITLAB_CLIENT_SECRET} \
	-X github.com/jullury/akama/internal/config.Version=${VERSION} \
	-X github.com/jullury/akama/internal/config.BuildTime=${BUILD_TIME} \
	-X github.com/jullury/akama/internal/config.BuildPlatform=${BUILD_PLATFORM}" \
	-o akama .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
	git curl ca-certificates bash nodejs npm \
	&& rm -rf /var/lib/apt/lists/*

RUN curl https://mise.run/ | sh

ENV NPM_CONFIG_PREFIX=/home/akama/.akama/.npm-global
ENV PATH="/home/akama/.akama/.npm-global/bin:${PATH}"

RUN npm install -g @anthropic-ai/claude-code opencode-ai 2>/dev/null || true

COPY --from=builder /src/akama /usr/local/bin/akama

ENTRYPOINT ["akama", "--daemon"]
