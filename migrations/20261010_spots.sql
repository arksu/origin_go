-- Initial spot generation is available for newly generated worlds only.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE origin.spot
(
	id                   BIGSERIAL PRIMARY KEY,

	region               INTEGER NOT NULL,
	layer                INTEGER NOT NULL CHECK (layer = 0),
	spot_type            VARCHAR(16) NOT NULL
		CHECK (spot_type IN ('www', 'clay', 'soil', 'sand', 'water')),

	district_x           INTEGER NOT NULL CHECK (district_x >= 0),
	district_y           INTEGER NOT NULL CHECK (district_y >= 0),

	-- Absolute world coordinates and world-unit radius.
	center_x             INTEGER NOT NULL,
	center_y             INTEGER NOT NULL,
	radius               INTEGER NOT NULL CHECK (radius > 0),

	peak_quality         SMALLINT NOT NULL
		CHECK (peak_quality BETWEEN 10 AND 32767),

	state                JSONB NOT NULL DEFAULT '{}'::JSONB
		CHECK (jsonb_typeof(state) = 'object'),
	last_runtime_seconds BIGINT NOT NULL
		CHECK (last_runtime_seconds >= 0),
	revision             BIGINT NOT NULL DEFAULT 1
		CHECK (revision > 0),

	UNIQUE (region, layer, district_x, district_y, spot_type)
);

COMMIT;
