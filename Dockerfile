FROM golang:1.24-bookworm

# Install base tooling needed by the app and containerized agents
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        dnsutils \
        git \
        iputils-ping \
        jq \
        less \
        net-tools \
        ripgrep \
        tree \
        unzip \
        wget \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js (required for Claude Code and Codex CLI)
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Claude Code and Codex CLI
RUN npm install -g @anthropic-ai/claude-code @openai/codex playwright

# Install Playwright browser binaries to a persistent path (not on tmpfs)
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/playwright
RUN npx playwright install --with-deps chromium

# Create home dir writable by the host user (uid 1000 by default)
# Allow /etc/passwd append so entrypoint can add a user entry (fixes "I have no name!")
RUN mkdir -p /home/dev && chmod 777 /home/dev \
    && chmod a+w /etc/passwd

# Trust bind-mounted repos owned by a different uid
RUN git config --global --add safe.directory '*'

COPY entrypoint.sh /usr/local/bin/
ENTRYPOINT ["entrypoint.sh"]

WORKDIR /app
