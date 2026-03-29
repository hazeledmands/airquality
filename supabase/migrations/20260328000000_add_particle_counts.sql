-- Add particle count bin columns (average of channels A and B, per deciliter).
ALTER TABLE readings ADD COLUMN IF NOT EXISTS p03  double precision;  -- ≥0.3 µm
ALTER TABLE readings ADD COLUMN IF NOT EXISTS p05  double precision;  -- ≥0.5 µm
ALTER TABLE readings ADD COLUMN IF NOT EXISTS p10  double precision;  -- ≥1.0 µm
ALTER TABLE readings ADD COLUMN IF NOT EXISTS p25  double precision;  -- ≥2.5 µm
ALTER TABLE readings ADD COLUMN IF NOT EXISTS p50  double precision;  -- ≥5.0 µm
ALTER TABLE readings ADD COLUMN IF NOT EXISTS p100 double precision;  -- ≥10.0 µm
