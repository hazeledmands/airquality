# airquality

Collects and summarizes air quality sensor data from IoT devices.

Currently supports [Sensibo](https://sensibo.com/) Air/Sky devices, reporting temperature and humidity readings with 24-hour statistics.

## Setup

1. Get a Sensibo API key from https://home.sensibo.com/me/api
2. Set the environment variable:
   ```sh
   export SENSIBO_API_KEY="your-api-key"
   ```

## Usage

```sh
go run .
```

Example output:

```
Found 3 device(s)

=== Office (abc123) ===
  Latest reading (Mar 16 13:01):
    Temperature: 22.8°C
    Humidity:    55.4%

  Last 24h summary (1753 data points):
    Temperature: 22.4°C avg (20.2°C – 25.1°C)
    Humidity:    60.4% avg (48.7% – 66.9%)
```

## Building

```sh
go build -o airquality .
./airquality
```
