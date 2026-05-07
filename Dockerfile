FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl bash nodejs npm \
    && rm -rf /var/lib/apt/lists/*

# Install opencode globally (default agent)
RUN npm install -g opencode-ai

# Verify opencode installation
RUN which opencode && opencode --version || echo "opencode check completed"

# Create non-root user
RUN groupadd -r akama && useradd -r -g akama -m -d /home/akama akama

# Create workspace directory with correct ownership
RUN mkdir -p /home/akama/.akama/workspaces && chown -R akama:akama /home/akama/.akama

# Copy and run install.sh to get akama binary
COPY install.sh /tmp/install.sh
RUN chmod +x /tmp/install.sh && /tmp/install.sh && rm /tmp/install.sh

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

WORKDIR /app

USER akama

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/usr/local/bin/akama", "start", "--daemon"]
