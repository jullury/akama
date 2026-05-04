FROM alpine:3.20 AS git-stage
RUN apk add --no-cache git bash ca-certificates

FROM n8nio/n8n:latest

USER root


COPY --from=git-stage /usr/bin/git /usr/bin/git
COPY --from=git-stage /bin/bash /bin/bash
COPY --from=git-stage /usr/lib/ /usr/lib/
COPY --from=git-stage /etc/ssl/certs/ /etc/ssl/certs/

# Copy helper scripts and workflows into image
COPY scripts/ /usr/local/bin/akama-scripts/
RUN chmod +x /usr/local/bin/akama-scripts/*.sh

COPY workflows/ /workflows/

# Override entrypoint: import workflows on first startup, then start n8n
ENTRYPOINT ["/usr/local/bin/akama-scripts/entrypoint.sh"]
