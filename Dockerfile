FROM golang:1.24-bookworm

# Install base tooling needed by the app and containerized agents
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        ripgrep \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js (required for Claude Code and Codex CLI)
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Claude Code and Codex CLI
RUN npm install -g @anthropic-ai/claude-code @openai/codex

# Trust bind-mounted repos owned by a different uid
RUN git config --global --add safe.directory '*'

WORKDIR /app
