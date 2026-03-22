-- Outdoor weather observations from Open-Meteo
CREATE TABLE IF NOT EXISTS weather (
    id               bigserial PRIMARY KEY,
    timestamp        timestamptz     NOT NULL,
    latitude         double precision NOT NULL,
    longitude        double precision NOT NULL,
    temp_c           double precision,
    humidity         double precision,
    dewpoint_c       double precision,
    precipitation_mm double precision,
    weather_code     integer,          -- WMO weather interpretation code
    pressure_hpa     double precision,
    wind_speed_kmh   double precision,
    uv_index         double precision,
    sunrise          timestamptz,
    sunset           timestamptz
);

-- One observation per location per 15-minute interval (Open-Meteo updates every 15 min)
CREATE UNIQUE INDEX IF NOT EXISTS weather_dedup
    ON weather (latitude, longitude, minute_epoch(timestamp));

CREATE INDEX IF NOT EXISTS weather_timestamp_idx ON weather (timestamp DESC);
