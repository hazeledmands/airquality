-- More from Open-Meteo: cloud cover on current weather, outdoor air quality
-- (CAMS) and the hourly forecast. The app checks for these at startup and
-- keeps recording weather without them until this is applied.

ALTER TABLE weather ADD COLUMN IF NOT EXISTS cloud_cover double precision;  -- %

-- Outdoor air quality from the Open-Meteo Air Quality API, one row per hour.
-- NULL means the model has no value (pollen is modelled only for Europe).
CREATE TABLE IF NOT EXISTS outdoor_air_quality (
    latitude         double precision NOT NULL,
    longitude        double precision NOT NULL,
    timestamp        timestamptz      NOT NULL,
    pm2_5            double precision,  -- µg/m³
    pm10             double precision,  -- µg/m³
    ozone            double precision,  -- µg/m³
    nitrogen_dioxide double precision,  -- µg/m³
    sulphur_dioxide  double precision,  -- µg/m³
    carbon_monoxide  double precision,  -- µg/m³
    us_aqi           double precision,
    european_aqi     double precision,
    dust             double precision,  -- µg/m³
    alder_pollen     double precision,  -- grains/m³
    birch_pollen     double precision,
    grass_pollen     double precision,
    mugwort_pollen   double precision,
    olive_pollen     double precision,
    ragweed_pollen   double precision,
    -- The key is the dedup rule: live polling and backfill both upsert.
    PRIMARY KEY (latitude, longitude, timestamp)
);

CREATE INDEX IF NOT EXISTS outdoor_air_quality_timestamp_idx
    ON outdoor_air_quality (timestamp DESC);

-- The latest hourly forecast for each future hour. Each poll overwrites the
-- hours it covers, so a past hour keeps the last forecast made for it.
CREATE TABLE IF NOT EXISTS weather_forecast (
    latitude           double precision NOT NULL,
    longitude          double precision NOT NULL,
    valid_time         timestamptz      NOT NULL,
    issued_at          timestamptz      NOT NULL,
    temp_c             double precision,
    precip_probability double precision,  -- %
    precip_mm          double precision,
    cloud_cover        double precision,  -- %
    uv_index           double precision,
    wind_speed_kmh     double precision,
    weather_code       integer,
    PRIMARY KEY (latitude, longitude, valid_time)
);

CREATE INDEX IF NOT EXISTS weather_forecast_valid_time_idx
    ON weather_forecast (valid_time DESC);
