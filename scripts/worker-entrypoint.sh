#!/bin/bash
set -e

# Configure user from env vars
SSH_USER=${SSH_USER:-root}
SSH_PASSWORD=${SSH_PASSWORD:-}

if [ -n "$SSH_PASSWORD" ]; then
    # Enable password authentication
    sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config
    sed -i 's/^#*PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config

    if [ "$SSH_USER" != "root" ]; then
        # Create user if not root
        adduser -D -s /bin/bash "$SSH_USER" 2>/dev/null || true
    fi

    # Set password
    echo "$SSH_USER:$SSH_PASSWORD" | chpasswd
    echo "SSH user '$SSH_USER' configured with password"
else
    echo "Warning: SSH_PASSWORD not set, password authentication disabled"
fi

# Start SSH daemon
exec /usr/sbin/sshd -D
