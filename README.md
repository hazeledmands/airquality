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

## Prometheus

In continuous mode, a Prometheus metrics endpoint is served at `http://localhost:50233/metrics` (configurable with `--port`).

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

## Building

```sh
go build -o airquality .
```
