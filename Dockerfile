FROM golang:1.24-bookworm

# Install Node.js (required for Claude Code)
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Claude Code
RUN npm install -g @anthropic-ai/claude-code

# Trust bind-mounted repos owned by a different uid
RUN git config --global --add safe.directory '*'

WORKDIR /app
