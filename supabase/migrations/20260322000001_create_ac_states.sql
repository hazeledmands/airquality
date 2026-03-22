-- AC state changes from Sensibo
CREATE TABLE IF NOT EXISTS ac_states (
    id                  bigserial PRIMARY KEY,
    timestamp           timestamptz NOT NULL,
    device_id           text        NOT NULL,
    room_name           text        NOT NULL,
    ac_on               boolean     NOT NULL,   -- "on" is reserved; use ac_on
    mode                text,
    target_temp         integer,
    temp_unit           text,
    fan_level           text,
    swing               text,
    horizontal_swing    text,
    changed_properties  text,                   -- semicolon-separated list
    reason              text,
    sensibo_id          text        NOT NULL UNIQUE  -- Sensibo's own ID, used for dedup
);

CREATE INDEX IF NOT EXISTS ac_states_timestamp_idx ON ac_states (timestamp DESC);
CREATE INDEX IF NOT EXISTS ac_states_device_idx    ON ac_states (device_id, timestamp DESC);
