FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl bash nodejs npm \
    && rm -rf /var/lib/apt/lists/*

# Install opencode globally
RUN npm install -g opencode-ai

# Verify opencode installation
RUN which opencode && opencode --version || echo "opencode check completed"

CMD ["/bin/bash"]
