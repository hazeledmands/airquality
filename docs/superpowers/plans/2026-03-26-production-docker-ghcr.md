# Production Docker Image & GHCR Pipeline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a production Dockerfile and GitHub Actions workflow that builds the air quality app into a lean container image and pushes it to GitHub Container Registry on every push to `main`.

**Architecture:** Three-stage Docker build: Node.js builds the Vite frontend into a single `rooms.html`, Go compiles the binary with that file embedded, a minimal Debian runtime image ships only the binary. A GitHub Actions workflow runs on push to `main`, logs in to `ghcr.io` with the built-in `GITHUB_TOKEN`, and pushes two tags (`latest` and `<short-sha>`).

**Tech Stack:** Docker multi-stage build, GitHub Actions (`docker/login-action`, `docker/metadata-action`, `docker/build-push-action`), `ghcr.io`

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `.dockerignore` | Create | Exclude `node_modules`, build artifacts, `.git` from build context |
| `Dockerfile.prod` | Create | Multi-stage production build |
| `.github/workflows/publish.yml` | Create | CI pipeline to build and push image to ghcr.io |

---

### Task 1: Create `.dockerignore`

**Files:**
- Create: `.dockerignore`

No tests for this task — correctness is verified when the Docker build succeeds in Task 2.

- [ ] **Step 1: Create `.dockerignore`**

```
# Git
.git
.gitignore

# Node dependencies and build output
node_modules
frontend/node_modules
frontend/dist-rooms

# Go build output
airquality

# Dev/test artifacts
tmp
```

Save this as `.dockerignore` at the repo root.

- [ ] **Step 2: Commit**

```bash
git add .dockerignore
git commit -m "Add .dockerignore for production build"
```

---

### Task 2: Create `Dockerfile.prod`

**Files:**
- Create: `Dockerfile.prod`

- [ ] **Step 1: Verify the `golang:1.25-bookworm` tag exists**

```bash
docker pull golang:1.25-bookworm
```

Expected: pull succeeds. If it fails, check `https://hub.docker.com/_/golang/tags` for the correct tag — `golang:1.25.8-bookworm` is also available.

- [ ] **Step 2: Create `Dockerfile.prod`**

```dockerfile
# Stage 1: Build the frontend (Vite → single-file rooms.html)
FROM node:22-bookworm-slim AS frontend
WORKDIR /app/frontend
COPY frontend/ .
RUN npm ci
RUN npm run build
# npm run build runs: vite build --config vite.rooms.config.js && cp dist-rooms/rooms.html ../rooms.html
# Result: /app/rooms.html

# Stage 2: Build the Go binary (embeds rooms.html)
FROM golang:1.25-bookworm AS builder
WORKDIR /app
COPY . .
# Overwrite any stale rooms.html from the build context with the freshly built one
COPY --from=frontend /app/rooms.html .
RUN go build -buildvcs=false -o airquality .

# Stage 3: Minimal runtime image
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN useradd -r -s /bin/false appuser
COPY --from=builder /app/airquality /airquality
EXPOSE 50233
USER appuser
ENTRYPOINT ["/airquality"]
```

- [ ] **Step 3: Build the image locally**

```bash
docker build -f Dockerfile.prod -t airquality:local .
```

Expected: build completes successfully, three stages visible in output. The image should be ~30-60 MB (check with `docker images airquality`).

If the build fails:
- Stage 1 failure: check Node version or `npm run build` output
- Stage 2 failure: check that `/app/rooms.html` is present (add `RUN ls -la /app/rooms.html` temporarily to debug)
- Stage 3 failure: unlikely — only copies a binary

- [ ] **Step 4: Run the image locally to verify startup**

```bash
docker run --rm -e DATABASE_URL="postgres://invalid" airquality:local
```

Expected: the app starts, connects to the (invalid) DB, and exits with a DB connection error — **not** a missing binary error or permission error. Something like:

```
Failed to connect to database: connect to database: ...
```

This confirms the binary runs, the non-root user is set correctly, and the entrypoint works.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile.prod
git commit -m "Add Dockerfile.prod for production builds"
```

---

### Task 3: Create GitHub Actions workflow

**Files:**
- Create: `.github/workflows/publish.yml`

- [ ] **Step 1: Create the `.github/workflows/` directory if it doesn't exist**

```bash
mkdir -p .github/workflows
```

- [ ] **Step 2: Create `.github/workflows/publish.yml`**

```yaml
name: Publish Docker image

on:
  push:
    branches:
      - main

permissions:
  contents: read
  packages: write

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=raw,value=latest
            type=sha,prefix=,format=short

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          file: Dockerfile.prod
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

Note: `github.repository` is automatically lowercased by `docker/metadata-action`, which is required by `ghcr.io`. Do not manually construct the image name string.

- [ ] **Step 3: Commit and push**

```bash
git add .github/workflows/publish.yml
git commit -m "Add GitHub Actions workflow to publish image to ghcr.io"
git push
```

- [ ] **Step 4: Verify the workflow runs**

Go to the repository on GitHub → Actions tab. The "Publish Docker image" workflow should appear and start running. Watch for:

1. The `Log in to GitHub Container Registry` step passes
2. The `Build and push` step completes
3. The image appears at `https://github.com/<owner>/<repo>/pkgs/container/<repo>`

If the push step fails with a 403:
- Confirm the `permissions: packages: write` block is present in the workflow YAML
- Check that the repository's Actions settings allow workflows to write packages (Settings → Actions → General → Workflow permissions → "Read and write permissions")

---

### Task 4: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add a "Docker" section to the README**

Add a section documenting how to build the production image locally and how the CI pipeline works:

```markdown
## Docker

A production image is built automatically on every push to `main` and published to GitHub Container Registry:

```
ghcr.io/<owner>/<repo>:latest
ghcr.io/<owner>/<repo>:<short-sha>
```

To build locally:

```bash
docker build -f Dockerfile.prod -t airquality:local .
docker run --rm \
  -e DATABASE_URL="postgres://user:pass@host/db" \
  -e SENSIBO_API_KEY="..." \
  -p 50234:50233 \
  airquality:local
```

The image runs as a non-root user and expects all configuration via environment variables (see [Environment Variables](#environment-variables)).
```

Replace `<owner>/<repo>` with the actual GitHub repository path.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Document production Docker image in README"
git push
```
