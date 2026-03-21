# airquality

Collects and logs air quality sensor data from IoT devices to a CSV file.

Supports:
- [Sensibo](https://sensibo.com/) Air/Sky — temperature, humidity, dew point
- [PurpleAir](https://www.purpleair.com/) (local API) — PM1.0/2.5/10, AQI, temperature, humidity, dew point, pressure, VOC

## Setup

Configure one or both sensor types via environment variables:

```sh
# Sensibo (cloud API)
export SENSIBO_API_KEY="your-api-key"    # from https://home.sensibo.com/me/api

# PurpleAir (local sensors, comma-separated IPs)
export PURPLEAIR_HOSTS="192.168.1.218,192.168.1.219"
```

At least one must be set.

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

In continuous mode, a web dashboard is served at `http://localhost:50233/` showing live charts of temperature, humidity, dew point, PM2.5, and AC on/off state from the CSV logs. The page auto-refreshes every 60 seconds, includes a date-range filter, and the AC state chart is rendered as a stepped on/off timeline per room. A "By Room" view at `/rooms` groups all metrics per room instead of per metric type. A JSON API is available at `/api/data`.

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
