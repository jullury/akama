FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl bash nodejs npm gosu \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r akama && useradd -r -g akama -m -d /home/akama akama

# Seed directory: binaries land here in the image and are copied into the
# volume on first run so that subsequent `akama update` / `npm install -g`
# writes to the volume and survives container recreation
RUN mkdir -p /opt/akama/bin /opt/akama/.npm-global

# Install opencode and claude into the seed npm prefix
RUN npm install -g opencode-ai @anthropic-ai/claude-code --prefix /opt/akama/.npm-global && \
    chown -R akama:akama /opt/akama

RUN /opt/akama/.npm-global/bin/opencode --version || echo "opencode check completed"
RUN /opt/akama/.npm-global/bin/claude --version || echo "claude check completed"

# Create workspace directory with correct ownership
RUN mkdir -p /home/akama/.akama/workspaces && chown -R akama:akama /home/akama/.akama

# Install akama binary into the seed location
COPY install.sh /tmp/install.sh
RUN chmod +x /tmp/install.sh && /tmp/install.sh && rm /tmp/install.sh && \
    mv /usr/local/bin/akama /opt/akama/bin/akama && \
    chown akama:akama /opt/akama/bin/akama

# Volume locations come first in PATH so updated binaries are preferred.
# Seed locations act as fallback when the volume is empty.
# NPM_CONFIG_PREFIX points to the volume so `npm install -g` updates persist.
ENV NPM_CONFIG_PREFIX=/home/akama/.akama/.npm-global
ENV PATH="/home/akama/.akama/bin:/home/akama/.akama/.npm-global/bin:/opt/akama/bin:/opt/akama/.npm-global/bin:${PATH}"

COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

WORKDIR /app

ENTRYPOINT ["/app/entrypoint.sh"]
