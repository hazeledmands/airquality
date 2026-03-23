CREATE TABLE annotations (
    id         bigserial PRIMARY KEY,
    timestamp  timestamptz NOT NULL,
    note       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX annotations_timestamp_idx ON annotations (timestamp DESC);
