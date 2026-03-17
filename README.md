# airquality

Collects and summarizes air quality sensor data from IoT devices.

Supports:
- [Sensibo](https://sensibo.com/) Air/Sky — temperature, humidity, dew point (24h history)
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

```sh
go run .
```

Example output:

```
Sensibo: 3 device(s)

=== Office (abc123) ===
  Latest reading (Mar 16 14:45):
    Temperature: 24.5°C / 76.1°F
    Humidity:    49.2%
    Dew Point:   13.2°C / 55.7°F

  Last 24h summary (1822 data points):
    Temperature: 22.4°C / 72.4°F avg (20.2–25.1°C / 68.4–77.2°F)
    Humidity:    60.1% avg (48.7% – 66.9%)
    Dew Point:   14.3°C / 57.7°F avg (12.6–17.5°C / 54.6–63.5°F)

PurpleAir: 1 sensor(s)

=== PurpleAir a4:e5:7c:28:49:7 (outside) ===
  Environment:
    Temperature: 35.0°C / 95.0°F
    Humidity:    20.0%
    Dew Point:   8.7°C / 47.7°F
    Pressure:    1014.9 mbar
    VOC (gas):   87.9 kOhm

  Air Quality (avg of channels A & B):
    PM1.0:  1.5 µg/m³
    PM2.5:  2.5 µg/m³  (A: 2.3, B: 2.6)
    PM10:   2.9 µg/m³
    AQI:    10
```

## Continuous Logging

Record measurements to a CSV file at regular intervals:

```sh
go run . log                              # default: 1m interval, airquality.csv
go run . log -interval 30s -output data.csv   # custom interval and file
```

The CSV includes: timestamp, source, sensor_id, location, temp_c, humidity, dewpoint_c, pressure, pm1, pm25, pm10, aqi, voc. Fields not applicable to a sensor type are left empty.

Press Ctrl+C to stop logging.

## Building

```sh
go build -o airquality .
./airquality           # one-shot summary
./airquality log       # continuous logging
```
