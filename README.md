# airquality

Collects and logs air quality sensor data from IoT devices to a CSV file and (optionally) PostgreSQL.

Supports:
- [Sensibo](https://sensibo.com/) Air/Sky — temperature, humidity, dew point
- [PurpleAir](https://www.purpleair.com/) (local API) — PM1.0/2.5/10, AQI, temperature, humidity, dew point, pressure, VOC
- [Open-Meteo](https://open-meteo.com/) — outdoor temperature, humidity, UV index, wind, precipitation, sunrise/sunset (no API key required)
- [WeatherFlow Tempest](https://tempest.earth/) (cloud API) — every field the station reports each minute (temperature, humidity, pressure, wind lull/average/gust/direction, rain and Rain Check rain, precipitation type, UV, solar radiation, illuminance, lightning count and distance, battery), plus individual lightning strikes and rain-start events

## Setup

Configure one or both sensor types via environment variables:

```sh
# Sensibo (cloud API)
export SENSIBO_API_KEY="your-api-key"    # from https://home.sensibo.com/me/api

# PurpleAir (local sensors, comma-separated IPs)
export PURPLEAIR_HOSTS="192.168.1.218,192.168.1.219"
```

At least one sensor source (Sensibo, PurpleAir or Tempest) must be set.

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

### WeatherFlow Tempest (optional)

Set `TEMPEST_TOKEN` to a personal access token (Tempest web app → Settings → Data Authorizations). The app finds the first station the token can see that has a Tempest; set `TEMPEST_STATION_ID` (or `tempest.station_id` in the config file) to pick another. Requires `DATABASE_URL` and the `tempest_observations`/`tempest_events` tables from `supabase/migrations/20261120000000_create_tempest.sql`. If the token is set but the tables are missing, Tempest is disabled with a `TEMPEST DISABLED` log line and `airquality_tempest_enabled 0`, and everything else keeps running.

```sh
export TEMPEST_TOKEN="..."          # secret: environment only
export TEMPEST_STATION_ID="123456"  # optional
```

Data comes from the WeatherFlow cloud, not the hub's UDP broadcast on port 50222: broadcasts don't cross subnets, and the cloud keeps history, so gaps of up to 24 hours fill themselves in on the next poll and longer ones can be backfilled.

- **Observations.** The REST API is polled every minute (`tempest.poll_interval`), independent of `--interval`. Each poll asks for everything after the newest stored row, so an outage of up to 24 hours fills itself in on the next poll. Rows are keyed on (device, timestamp) and upserted, so overlapping fetches never duplicate. Once a day the last 72 hours are fetched again to pick up WeatherFlow's Rain Check corrections, which arrive during the following local day; a failed recheck waits a day rather than retrying every minute. A stored value is never overwritten with null. Implausible values (out-of-range or non-finite, e.g. a precipitation type of 40000) are stored as NULL with a log line, rows outside the requested window or in the future are dropped, and a row the database still rejects is skipped on its own, so one bad row never stalls ingestion.
- **Events.** A WebSocket connection (`wss://ws.weatherflow.com`) records each lightning strike (time, distance, energy) and rain start, which REST keeps only as per-minute counts. It reconnects with backoff, keeps only events from the selected devices with timestamps within 10 minutes of now, and caps delivery at 600 events a minute. Two strikes in the same second are two rows; a replayed event is stored and counted once. Turn it off with `tempest.events: false`. The 3-second `rapid_wind` stream is not stored: the per-minute lull, average and gust already capture it at about 1/20 the volume.
- **Backfill.** `airquality tempest-backfill` loads history, by default from the day the station was created:

  ```sh
  airquality tempest-backfill                          # everything since the station was created
  airquality tempest-backfill --from 2026-10-01 --to 2026-10-08
  ```

  It requests one day at a time (one-minute data is only served for short ranges; six days comes back in 30-minute buckets, which the client refuses), waits `--delay` (default 1s) between requests (longer if the API sends `Retry-After`), and upserts. Re-running it is safe and also picks up Rain Check corrections. Backfill writes to Postgres only. A running server serves the dashboard from an in-memory cache loaded at startup, so backfilled history appears there after the server restarts. Prometheus never gets it: its gauges carry only the latest live observation, and history lives in the database.
- **Testing against a stub.** `TEMPEST_API_URL` and `TEMPEST_WS_URL` override the WeatherFlow REST and WebSocket base URLs. The token goes to whatever host they name, so they must be `https://` / `wss://`, except to a loopback host (`localhost`, `127.0.0.1`, `::1`), where `http://` / `ws://` is allowed for a local stub. Any other scheme or host is a startup error. The effective host, never the full URL, is logged.

### Config file (optional)

Non-secret settings can live in a YAML file passed with `--config` (or `AIRQUALITY_CONFIG`). Environment variables take precedence over the file, so an env-only setup keeps working. Secrets (`SENSIBO_API_KEY`, `TEMPEST_TOKEN`, `DATABASE_URL`) are only ever read from the environment. Unknown keys are a startup error.

```yaml
purpleair:
  hosts: [192.168.1.218]       # PURPLEAIR_HOSTS overrides
weather:
  lat: 37.8313                 # WEATHER_LAT / WEATHER_LON override
  lon: -122.2853
tempest:
  station_id: 123456           # TEMPEST_STATION_ID overrides; default: first with a Tempest
  poll_interval: 1m            # REST poll cadence (minimum 30s)
  events: true                 # WebSocket lightning/rain-start listener
sensors:                       # keyed by source/sensor_id
  purpleair/a4:e5:7c:28:49:7:
    room: Downstairs           # show this sensor in the Downstairs card
    offsets:                   # calibration, added at display time
      temp_c: -4.4
      humidity: 4
```

**Rooms.** Sensors are grouped into dashboard cards by the location they report (the Sensibo room name, or the PurpleAir's `inside`/`outside`). Set `room` to put several devices in one card. This applies when data is read, so past readings regroup too.

**Offsets.** Devices in the same room often disagree. The PurpleAir's temperature/humidity sensor sits inside its warm case and typically reads about 8°F high and about 4% RH low. `offsets` are added to `temp_c` and `humidity` when serving the dashboard, and dew point is recomputed from the corrected values. Stored data and Prometheus gauges stay raw. Adjusted devices show as "(adj)" in the dashboard.

## Usage

```
Usage: airquality [options]
       airquality tempest-backfill [--from DATE] [--to DATE] [--station ID] [--delay 1s]

Options:
      --config path         YAML config file (default $AIRQUALITY_CONFIG)
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

### Annotation auth (optional)

Viewing is always public. Set `AUTH_USER_HEADER` to the request header a trusted reverse proxy fills with the signed-in user (e.g. `X-authentik-username` behind Authentik forward-auth) and adding or deleting annotations requires it; requests without it get `401`. Unset, anyone can edit — the local-development default.

With it set, the proxy must gate `/login` and `/api/me` and every non-GET request, and must be the only way to reach the app, or the header can be forged. `GET /login?next=<path>` exists only to be gated: it runs the sign-in flow and bounces back. `GET /api/me` reports `{"user", "canEdit"}`; signed-out viewers see a "Sign in to annotate" link and read-only annotations.

## Dashboard

In continuous mode, a web dashboard is served at `http://localhost:50233/` showing live charts. The default view groups all metrics (temperature, humidity, dew point, PM2.5, AC state) per room. A "By Metric" view at `/by-metric` shows one chart per metric type. Both views auto-refresh every 60 seconds. When a room has more than one device, every device's lines are drawn in the same per-metric colors. The tooltip names each device ("Temperature · PurpleAir (adj)"), the legend keeps one entry per metric (clicking it toggles all of that metric's lines), and the latest-values strip shows each device's reading ("70.1 / 70.8°F") followed by the device names in the same order. The date-range picker includes quick presets (1h, 8h, 1d, 3d, 1w) and prev/next buttons to pan through time. Hovering over any chart shows a synced vertical crosshair line across all charts at the same timestamp. A JSON API is available at `/api/data`.

When `WEATHER_LAT`/`WEATHER_LON` are configured, the rooms view also shows:
- **Outdoor weather card** — current conditions strip (temp, humidity, wind, precip) plus a dual-axis chart of outdoor temperature and UV index
- **Daylight shading** — non-daylight hours are shaded on all charts using the sunrise/sunset times from Open-Meteo
- **Outdoor temp overlay** — a toggle adds outdoor temperature as a dashed line on each indoor room chart for comparison; state persists in the URL (`?outdoorTemp=1`)

When `TEMPEST_TOKEN` is configured, a **Tempest** section follows. Its latest-values strip is built from the newest raw (not downsampled) observation in the range and says "as of" when that is more than 5 minutes old: temperature, humidity, dew point, wind with compass direction and gusts, rain today and current rate, UV, solar radiation, illuminance, recent strikes, sea-level pressure (inHg) with its 3-hour trend, battery. Below it, panels share the dashboard's time axis:
- **Outdoor**: temperature, dew point, humidity on the same °F and % axes as the room charts, so outdoor and indoor line up
- **Wind**: average (mph) inside a shaded lull–gust band
- **Wind direction**: a thin strip of dots on a compass axis (none in calm air)
- **Rain**: rate (in/h) as an area, the wettest single minute as a line so peaks survive downsampling, the total since local midnight and its Rain Check correction, hail, and rain-start markers
- **Sun**: UV index and solar radiation (W/m²); illuminance is in the legend, hidden by default
- **Lightning**: average strike distance per bucket, sized by strike count, plus each individual strike; collapses to a one-line note when there were none
- **Station**: sea-level pressure (inHg, from station pressure and the station's elevation) and battery voltage

The dashboard always requests downsampled data (about 500 points per range), and each field is aggregated by what it means: maximum gust, minimum lull, summed rain and strikes, wettest minute kept, daily total that survives the midnight reset, vector-mean direction. Legend toggles persist in the URL (`?tempestHidden=lux,gust`; absent means the default, illuminance hidden). When recording is on but the range has no observations, the section says so instead of disappearing. The "Outdoor temp" overlay uses the Tempest instead of Open-Meteo when both are present.

Port `50233` is the production default. When testing locally, use a different port such as `--port 50234`.

## Prometheus

Prometheus metrics are served at `http://localhost:50233/metrics` (port configurable with `--port`).

Exported metrics (all with `source`, `sensor_id`, `location`, `room` labels). `location` is what the device reports; `room` is the room from the config file (equal to `location` if not overridden). Values are raw, without calibration offsets:

| Metric | Description |
|--------|-------------|
| `airquality_temperature_celsius` | Temperature in °C |
| `airquality_humidity_percent` | Relative humidity % |
| `airquality_dewpoint_celsius` | Dew point in °C |
| `airquality_pressure_mbar` | Atmospheric pressure: station (uncorrected) pressure for PurpleAir; sea-level pressure for Open-Meteo and the Tempest |
| `airquality_pm1_ugm3` | PM1.0 µg/m³ |
| `airquality_pm25_ugm3` | PM2.5 µg/m³ |
| `airquality_pm10_ugm3` | PM10 µg/m³ |
| `airquality_aqi` | Air Quality Index |
| `airquality_voc_kohm` | VOC gas resistance |
| `airquality_uv_index` | UV index (Open-Meteo and Tempest) |

The Tempest sets `airquality_temperature_celsius`, `_humidity_percent`, `_dewpoint_celsius`, `_pressure_mbar` (sea-level pressure, comparable to Open-Meteo's) and `_uv_index` with `source="tempest"`, `sensor_id` = device ID, `location` = station name. Its other fields have their own gauges, with the same labels. A null reading removes its series instead of reporting zero. Per-interval values are gauges; use the `_total` counters to sum rain or strikes over a range:

| Metric | Description |
|--------|-------------|
| `airquality_tempest_enabled` | 1 when recording runs, 0 when configured but disabled (no label) |
| `airquality_tempest_station_pressure_mbar` | Station pressure, not sea-level corrected |
| `airquality_tempest_rain_millimeters_total` | Rain in observations recorded since start (each minute counted once) |
| `airquality_tempest_lightning_strikes_total` | Strikes in observations recorded since start |
| `airquality_tempest_wind_lull_meters_per_second` | Minimum 3 s wind sample in the interval |
| `airquality_tempest_wind_avg_meters_per_second` | Average wind speed |
| `airquality_tempest_wind_gust_meters_per_second` | Maximum 3 s wind sample |
| `airquality_tempest_wind_direction_degrees` | Wind direction |
| `airquality_tempest_wind_sample_interval_seconds` | Wind sample interval |
| `airquality_tempest_illuminance_lux` | Illuminance |
| `airquality_tempest_solar_radiation_watts_per_m2` | Solar radiation |
| `airquality_tempest_rain_millimeters` | Rain in the interval |
| `airquality_tempest_rain_local_day_millimeters` | Rain since station-local midnight |
| `airquality_tempest_rain_final_millimeters` | Rain Check corrected rain in the interval |
| `airquality_tempest_rain_local_day_final_millimeters` | Rain Check corrected rain since midnight |
| `airquality_tempest_precipitation_type` | 0 none, 1 rain, 2 hail, 3 both |
| `airquality_tempest_precip_analysis_type` | 0 none, 1 Rain Check on, 2 Rain Check off |
| `airquality_tempest_lightning_strikes` | Strikes in the interval |
| `airquality_tempest_lightning_avg_distance_km` | Average strike distance (absent without strikes) |
| `airquality_tempest_battery_volts` | Battery voltage |
| `airquality_tempest_report_interval_minutes` | Report interval |
| `airquality_tempest_observation_timestamp_seconds` | Time of the latest observation |
| `airquality_tempest_lightning_strike_events_total` | Strike events received over the WebSocket |
| `airquality_tempest_rain_start_events_total` | Rain-start events received |
| `airquality_tempest_last_strike_distance_km` / `_energy` / `_timestamp_seconds` | Latest strike |
| `airquality_tempest_last_rain_start_timestamp_seconds` | Latest rain start |

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

Those builds never deploy themselves. Releasing does: bump `VERSION`, commit it to `main`, then run the **Release** workflow (`gh workflow run release.yml -f version=X.Y.Z`). It verifies the build, publishes `airquality:X.Y.Z` (plus `X.Y` and the short SHA), and tags the commit `vX.Y.Z`. The homelab's argocd-image-updater promotes every release within `^1.0.0` to the cluster within a few minutes.

The version is the deployment contract. **MAJOR** means the deployment must change first — a new required env var, new infrastructure, a new expectation of the proxy in front, or a migration the previous build can't tolerate; the updater won't cross it, and the homelab chart change lands with a widened constraint. **MINOR** is a backward-compatible feature, **PATCH** a backward-compatible fix.

To build locally:

```bash
docker build -f Dockerfile.prod -t airquality:local .
docker run --rm \
  -e DATABASE_URL="postgres://user:pass@host/db" \
  -e SENSIBO_API_KEY="..." \
  -p 50234:50233 \
  airquality:local
```

The image runs as a non-root user and reads configuration from environment variables and an optional config file (see [Setup](#setup)).

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
