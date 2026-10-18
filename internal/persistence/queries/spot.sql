-- name: DeleteSpotsByRegion :exec
DELETE FROM spot WHERE region = $1;

-- name: InsertSpots :exec
-- All arrays have the same length; the caller bounds each batch to 1000 rows.
-- State and revision use their initial database defaults.
INSERT INTO spot (
	region, layer, spot_type, district_x, district_y,
	center_x, center_y, radius, peak_quality, last_runtime_seconds
)
SELECT sqlc.arg(region)::int, sqlc.arg(layer)::int,
	   unnest(sqlc.arg(spot_types)::text[]),
	   unnest(sqlc.arg(district_xs)::int[]), unnest(sqlc.arg(district_ys)::int[]),
	   unnest(sqlc.arg(center_xs)::int[]), unnest(sqlc.arg(center_ys)::int[]),
	   unnest(sqlc.arg(radii)::int[]), unnest(sqlc.arg(peak_qualities)::smallint[]),
	   sqlc.arg(last_runtime_seconds)::bigint;

-- name: LoadSpotsByRegion :many
SELECT * FROM spot WHERE region = $1 ORDER BY id;

-- name: SaveSpotState :execrows
-- A retry after an ambiguous result must not advance the revision twice.
UPDATE spot
SET state = sqlc.arg(state)::jsonb,
	last_runtime_seconds = sqlc.arg(last_runtime_seconds)::bigint,
	revision = sqlc.arg(expected_revision)::bigint + 1
WHERE id = sqlc.arg(id)::bigint AND region = sqlc.arg(region)::int
	AND (
		revision = sqlc.arg(expected_revision)::bigint
		OR (
			revision = sqlc.arg(expected_revision)::bigint + 1
			AND state = sqlc.arg(state)::jsonb
			AND last_runtime_seconds = sqlc.arg(last_runtime_seconds)::bigint
		)
	);
