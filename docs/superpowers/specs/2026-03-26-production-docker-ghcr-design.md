# Production Docker Image & GitHub Container Registry Pipeline

**Date:** 2026-03-26
**Status:** Approved

## Overview

Add a production `Dockerfile.prod` and a GitHub Actions workflow that builds the air quality app into a lean container image and publishes it to GitHub Container Registry (`ghcr.io`) on every push to `main`.

## Context

The existing `Dockerfile` is a development environment image (~2 GB) containing Claude Code, Playwright, Node.js, and other dev tooling. It is not suitable for production deployment.

The app is a Go binary that:
- Polls Sensibo and PurpleAir sensors on a configurable interval
- Writes readings to an external PostgreSQL database
- Serves a web dashboard and Prometheus metrics on port 50233 (default)

The frontend (`rooms.html`) is built with Vite using `vite-plugin-singlefile`, which bundles all JS/CSS into a single HTML file that is then embedded in the Go binary via `//go:embed rooms.html`. The full build sequence is:

```
npm --prefix frontend install
npm --prefix frontend run build   # outputs dist-rooms/rooms.html → ../rooms.html
go build -buildvcs=false -o airquality .
```

## Files to Create

### `Dockerfile.prod`

Multi-stage build with three stages:

**Stage 1 — frontend** (`node:22-bookworm-slim`)
- Copy `frontend/` directory
- Run `npm ci` then `npm run build`
- Output: `rooms.html` (single-file bundle)

**Stage 2 — builder** (`golang:1.24-bookworm`)
- Copy Go source files and `go.mod`/`go.sum`
- Copy `rooms.html` from the frontend stage
- Run `go build -buildvcs=false -o airquality .`
- Output: `airquality` binary

**Stage 3 — runtime** (`debian:bookworm-slim`)
- Install `ca-certificates` (needed for HTTPS calls to Sensibo, PurpleAir, Open-Meteo APIs)
- Create a non-root user (`appuser`)
- Copy only the `airquality` binary from the builder stage
- `EXPOSE 50233`
- Run as non-root user
- `ENTRYPOINT ["/airquality"]`

Result: ~30 MB image.

### `.github/workflows/publish.yml`

Triggers on push to `main`.

Steps:
1. `actions/checkout@v4`
2. `docker/login-action` — log in to `ghcr.io` using the built-in `GITHUB_TOKEN` (no secrets to configure)
3. `docker/metadata-action` — generate tags:
   - `ghcr.io/<owner>/<repo>:latest`
   - `ghcr.io/<owner>/<repo>:<short-sha>`
4. `docker/build-push-action` — build `Dockerfile.prod` and push both tags

No additional repository secrets are required; `GITHUB_TOKEN` is provided automatically by GitHub Actions.

## What Is Not In Scope

- Kubernetes manifests (future work)
- Secrets management (`DATABASE_URL`, `SENSIBO_API_KEY`, etc.) — injected at runtime by the deployment environment
- Multi-architecture builds (linux/amd64 only for now)
- Caching of npm/Go build layers (can be added later if build times become a concern)

## Environment Variables (runtime)

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `SENSIBO_API_KEY` | At least one sensor | Sensibo cloud API key |
| `PURPLEAIR_HOSTS` | At least one sensor | Comma-separated local IPs |
| `WEATHER_LAT` | No | Latitude for Open-Meteo |
| `WEATHER_LON` | No | Longitude for Open-Meteo |

## Testing the Image Locally

```bash
docker build -f Dockerfile.prod -t airquality:local .
docker run --rm \
  -e DATABASE_URL="postgres://..." \
  -e SENSIBO_API_KEY="..." \
  -p 50234:50233 \
  airquality:local
```
