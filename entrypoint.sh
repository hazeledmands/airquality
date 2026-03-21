#!/bin/bash
# Container entrypoint — runs as root, sets up firewall and config,
# then drops to the host user via gosu.

set -e

TARGET_UID="${HOST_UID:-1000}"
TARGET_GID="${HOST_GID:-1000}"

# Set up iptables firewall (requires root + NET_ADMIN)
/usr/local/bin/init-firewall.sh

# Add a passwd entry so the shell shows a name instead of "I have no name!"
if ! getent passwd "$TARGET_UID" >/dev/null 2>&1; then
    echo "dev:x:${TARGET_UID}:${TARGET_GID}::/home/dev:/bin/bash" >> /etc/passwd
fi

# Copy .claude.json from read-only staging mount to writable $HOME.
# We cannot bind-mount .claude.json directly because Docker single-file
# bind mounts break when the process does atomic writes (write tmp + rename).
if [ -f /host-config/.claude.json ] && [ ! -f /home/dev/.claude.json ]; then
    cp /host-config/.claude.json /home/dev/.claude.json
    chown "${TARGET_UID}:${TARGET_GID}" /home/dev/.claude.json
fi

# Drop to host user and exec the command
exec gosu "${TARGET_UID}:${TARGET_GID}" "$@"
