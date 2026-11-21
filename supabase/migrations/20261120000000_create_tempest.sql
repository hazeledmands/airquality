-- WeatherFlow Tempest observations: one row per device per report interval
-- (normally one minute), holding every field of the obs_st array as the
-- REST API returns it. NULL means the device reported null, never zero.
-- Units are the API's (metric); conversion happens at display time.
CREATE TABLE IF NOT EXISTS tempest_observations (
    device_id                 integer          NOT NULL,
    timestamp                 timestamptz      NOT NULL,
    wind_lull_ms              double precision,          -- [1] min 3 s sample
    wind_avg_ms               double precision,          -- [2]
    wind_gust_ms              double precision,          -- [3] max 3 s sample
    wind_direction_deg        double precision,          -- [4]
    wind_sample_interval_s    double precision,          -- [5]
    station_pressure_mb       double precision,          -- [6] not sea-level corrected
    air_temp_c                double precision,          -- [7]
    relative_humidity         double precision,          -- [8]
    illuminance_lux           double precision,          -- [9]
    uv_index                  double precision,          -- [10]
    solar_radiation_wm2       double precision,          -- [11]
    rain_mm                   double precision,          -- [12] over the interval
    precip_type               smallint,                  -- [13] 0 none, 1 rain, 2 hail, 3 both
    lightning_avg_distance_km double precision,          -- [14]
    lightning_strike_count    integer,                   -- [15]
    battery_v                 double precision,          -- [16]
    report_interval_min       integer,                   -- [17]
    local_day_rain_mm         double precision,          -- [18] since station-local midnight
    rain_final_mm             double precision,          -- [19] Rain Check corrected [12]
    local_day_rain_final_mm   double precision,          -- [20] Rain Check corrected [18]
    precip_analysis_type      smallint,                  -- [21]
    -- The primary key is the dedup rule: live polling, catch-up and backfill
    -- all upsert, so overlapping fetches never duplicate a row.
    PRIMARY KEY (device_id, timestamp)
);

CREATE INDEX IF NOT EXISTS tempest_observations_timestamp_idx
    ON tempest_observations (timestamp DESC);

-- Events the station pushes over the WebSocket API and the REST API does not
-- keep individually: lightning strikes (with distance and energy) and rain
-- start. Per-minute strike counts and average distance are also in
-- tempest_observations, so a WebSocket outage loses detail, not totals.
CREATE TABLE IF NOT EXISTS tempest_events (
    id          bigserial        PRIMARY KEY,
    device_id   integer          NOT NULL,
    timestamp   timestamptz      NOT NULL,
    type        text             NOT NULL,  -- 'lightning_strike' | 'rain_start'
    distance_km double precision,
    energy      double precision
);

-- Dedup: the same event replayed is one row, but two strikes in the same
-- second (different distance or energy) are two. The app counts only rows
-- this index lets in, so the database and the Prometheus counter agree.
CREATE UNIQUE INDEX IF NOT EXISTS tempest_events_dedup
    ON tempest_events (device_id, type, timestamp,
                       (COALESCE(distance_km, -1)), (COALESCE(energy, -1)));

CREATE INDEX IF NOT EXISTS tempest_events_timestamp_idx
    ON tempest_events (timestamp DESC);
