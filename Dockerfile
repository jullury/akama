FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl bash nodejs npm gosu \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -r akama && useradd -r -g akama -m -d /home/akama akama

RUN mkdir -p /opt/akama/bin /opt/akama/.npm-global

RUN npm install -g opencode-ai @anthropic-ai/claude-code --prefix /opt/akama/.npm-global && \
    chown -R akama:akama /opt/akama

RUN /opt/akama/.npm-global/bin/opencode --version || echo "opencode check completed"
RUN /opt/akama/.npm-global/bin/claude --version || echo "claude check completed"

RUN mkdir -p /home/akama/.akama/workspaces && chown -R akama:akama /home/akama/.akama

ENV NPM_CONFIG_PREFIX=/home/akama/.akama/.npm-global
ENV PATH="/home/akama/.akama/bin:/home/akama/.akama/.npm-global/bin:/opt/akama/bin:/opt/akama/.npm-global/bin:${PATH}"

COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

WORKDIR /app

ENTRYPOINT ["/app/entrypoint.sh"]

COPY install.sh /tmp/install.sh
RUN chmod +x /tmp/install.sh && /tmp/install.sh && rm /tmp/install.sh && \
    mv /usr/local/bin/akama /opt/akama/bin/akama && \
    chown akama:akama /opt/akama/bin/akama
