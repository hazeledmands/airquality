#!/bin/bash
# Seed container with host Claude config on first start.
# We copy instead of bind-mounting .claude.json because Docker file
# bind mounts break when the process does atomic writes (write tmp + rename).

# Add a passwd entry for the container UID so the shell shows a name
# instead of "I have no name!" and tools that look up the current user work.
if ! getent passwd "$(id -u)" >/dev/null 2>&1; then
    echo "dev:x:$(id -u):$(id -g)::/home/dev:/bin/bash" >> /etc/passwd
fi

if [ -f /host-config/.claude.json ] && [ ! -f "$HOME/.claude.json" ]; then
    cp /host-config/.claude.json "$HOME/.claude.json"
fi

exec "$@"
