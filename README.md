# airquality

Collects and logs air quality sensor data from IoT devices to a CSV file and (optionally) PostgreSQL.

Supports:
- [Sensibo](https://sensibo.com/) Air/Sky — temperature, humidity, dew point
- [PurpleAir](https://www.purpleair.com/) (local API) — PM1.0/2.5/10, AQI, temperature, humidity, dew point, pressure, VOC
- [WeatherFlow Tempest](https://weatherflow.com/tempest/) — temperature, humidity, dew point, pressure, wind, rain, UV, solar radiation, lightning
- [Open-Meteo](https://open-meteo.com/) — outdoor temperature, humidity, UV index, wind, precipitation, sunrise/sunset (no API key required)

## Setup

Configure one or more sensor types via environment variables:

```sh
# Sensibo (cloud API)
export SENSIBO_API_KEY="your-api-key"    # from https://home.sensibo.com/me/api

# PurpleAir (local sensors, comma-separated IPs)
export PURPLEAIR_HOSTS="192.168.1.218,192.168.1.219"

# WeatherFlow Tempest (cloud API)
export TEMPEST_TOKEN="your-token"        # from https://tempestwx.com/settings
export TEMPEST_STATION_ID="123456"       # your station ID
```

At least one sensor must be set.

### PostgreSQL (optional)

Set `DATABASE_URL` to a Postgres connection string to mirror all readings to a database. Writes are idempotent — duplicate entries are silently skipped — so multiple instances can run concurrently against the same database.

```sh
export DATABASE_URL="postgresql://user:pass@host:5432/dbname"
```

Schema migrations live in `supabase/migrations/`. Apply them with `psql` or the Supabase CLI before first use.

### Outdoor weather (optional)

Set `WEATHER_LAT` and `WEATHER_LON` to fetch outdoor conditions from [Open-Meteo](https://open-meteo.com/) on each capture cycle. Requires `DATABASE_URL` — weather data is stored in the `weather` table and surfaced in the dashboard.

```sh
export WEATHER_LAT="37.8313"
export WEATHER_LON="-122.2853"
```

## Usage

```
Usage: airquality [options]

Options:
  -i, --interval duration   capture interval (default 1m)
  -o, --output string       output CSV file path (default "airquality.csv")
      --ac-output string    AC state changes CSV file path (default "acstates.csv")
  -p, --port int            prometheus metrics port (default 50233)
  -v, --verbose             print detailed sensor data on each capture
      --once                capture once and exit
  -h, --help                show this help
```

### Continuous logging (default)

```sh
airquality                            # log every 1m to airquality.csv
airquality -i 30s -o data.csv         # custom interval and file
airquality --verbose                  # print all sensor data each cycle
airquality -p 9090                    # prometheus on a different port
```

Press Ctrl+C to stop.

### One-shot reading

```sh
airquality --once                     # capture once and exit
airquality --once --verbose           # detailed one-shot output
```

The CSV includes: timestamp, source, sensor_id, location, temp_c, humidity, dewpoint_c, pressure, pm1, pm25, pm10, aqi, voc. Fields not applicable to a sensor type are left empty.

## AC State Tracking

When a Sensibo API key is configured, AC state changes (on/off, mode, target temperature, fan level, swing) are tracked in a separate CSV file (`acstates.csv` by default, configurable with `--ac-output`).

Each run fetches up to 20 recent state changes per device from the Sensibo API. The command is idempotent — duplicate entries are never written, so it's safe to run repeatedly. Each state change is identified by a unique Sensibo ID.

CSV columns: timestamp, device_id, room, on, mode, target_temp, temp_unit, fan_level, swing, horizontal_swing, changed_properties, reason, sensibo_id.

## Dashboard

In continuous mode, a web dashboard is served at `http://localhost:50233/` showing live charts. The default view groups all metrics (temperature, humidity, dew point, PM2.5, AC state) per room. A "By Metric" view at `/by-metric` shows one chart per metric type. Both views auto-refresh every 60 seconds. The date-range picker includes quick presets (1h, 8h, 1d, 3d, 1w) and prev/next buttons to pan through time. Hovering over any chart shows a synced vertical crosshair line across all charts at the same timestamp. A JSON API is available at `/api/data`.

When `WEATHER_LAT`/`WEATHER_LON` are configured, the rooms view also shows:
- **Outdoor weather card** — current conditions strip (temp, humidity, wind, precip) plus a dual-axis chart of outdoor temperature and UV index
- **Daylight shading** — non-daylight hours are shaded on all charts using the sunrise/sunset times from Open-Meteo
- **Outdoor temp overlay** — a toggle adds outdoor temperature as a dashed line on each indoor room chart for comparison; state persists in the URL (`?outdoorTemp=1`)

Port `50233` is the production default. When testing locally, use a different port such as `--port 50234`.

## Prometheus

Prometheus metrics are served at `http://localhost:50233/metrics` (port configurable with `--port`).

Exported metrics (all with `source`, `sensor_id`, `location` labels):

| Metric | Description |
|--------|-------------|
| `airquality_temperature_celsius` | Temperature in °C |
| `airquality_humidity_percent` | Relative humidity % |
| `airquality_dewpoint_celsius` | Dew point in °C |
| `airquality_pressure_mbar` | Atmospheric pressure |
| `airquality_pm1_ugm3` | PM1.0 µg/m³ |
| `airquality_pm25_ugm3` | PM2.5 µg/m³ |
| `airquality_pm10_ugm3` | PM10 µg/m³ |
| `airquality_aqi` | Air Quality Index |
| `airquality_voc_kohm` | VOC gas resistance |

## Dev Container

A Docker-based development environment is available with Claude Code, Go, Node.js, and network isolation (iptables firewall allowing only the project's API domains).

```sh
bin/dev-container up -d --build          # build and start
bin/dev-container exec dev claude        # Claude Code inside container
bin/dev-container exec dev go test ./... # run Go tests
bin/dev-container exec dev bash          # interactive shell
bin/dev-container down                   # stop
```

The wrapper script (`bin/dev-container`) handles UID/GID mapping so files created inside the container are owned by your host user. Source code is bind-mounted at `/app` — edits inside the container appear on the host and vice versa.

Environment variables (`SENSIBO_API_KEY`, `PURPLEAIR_HOSTS`, etc.) are passed through from the host. The app port is mapped at `50234` to avoid conflicting with a production instance on `50233`.

## Docker

A production image is built automatically on every push to `main` and published to GitHub Container Registry:

```
ghcr.io/hazeledmands/airquality:latest
ghcr.io/hazeledmands/airquality:<short-sha>
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

The image runs as a non-root user and expects all configuration via environment variables (see [Setup](#setup)).

## Building

The dashboard frontend lives in `frontend/` as ES modules bundled by Vite into single-file HTML pages (`dashboard.html` and `rooms.html`) that Go embeds at compile time.

```sh
make build   # install deps (if needed), build frontend, compile Go binary
make run     # build and start the server
make test    # run all tests (frontend + Go)
make clean   # remove build artifacts
```

Or manually:

```sh
npm --prefix frontend install       # install frontend dependencies (first time only)
npm --prefix frontend run build     # build frontend (produces dashboard.html + rooms.html)
go build -o airquality .            # compile Go binary
```

## Frontend tests

Unit tests for the dashboard JavaScript use Vitest:

```sh
make test                            # all tests (frontend + Go)
npm --prefix frontend test           # frontend only
npm --prefix frontend run test:watch # frontend watch mode
```
