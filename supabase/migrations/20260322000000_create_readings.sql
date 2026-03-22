-- Immutable helper: truncates a timestamptz to the minute as a bigint epoch.
-- Needed because date_trunc on timestamptz is STABLE (not IMMUTABLE) in Postgres,
-- so it can't be used directly in a unique index expression.
CREATE OR REPLACE FUNCTION minute_epoch(ts timestamptz)
RETURNS bigint LANGUAGE sql IMMUTABLE PARALLEL SAFE AS
$$ SELECT floor(extract(epoch from ts) / 60)::bigint $$;

-- Sensor readings from all sources (Sensibo, PurpleAir, etc.)
CREATE TABLE IF NOT EXISTS readings (
    id          bigserial PRIMARY KEY,
    timestamp   timestamptz NOT NULL,
    source      text        NOT NULL,
    sensor_id   text        NOT NULL,
    location    text        NOT NULL,
    temp_c      double precision,
    humidity    double precision,
    dewpoint_c  double precision,
    pressure    double precision,  -- purpleair only
    pm1         double precision,  -- purpleair only
    pm25        double precision,  -- purpleair only
    pm10        double precision,  -- purpleair only
    aqi         double precision,  -- purpleair only
    voc         double precision   -- purpleair only
);

-- Dedup: one reading per source+sensor per minute (mirrors CSV logger behaviour)
CREATE UNIQUE INDEX IF NOT EXISTS readings_dedup
    ON readings (source, sensor_id, minute_epoch(timestamp));

-- Fast time-range queries for the dashboard
CREATE INDEX IF NOT EXISTS readings_timestamp_idx ON readings (timestamp DESC);
CREATE INDEX IF NOT EXISTS readings_sensor_idx    ON readings (source, sensor_id, timestamp DESC);
