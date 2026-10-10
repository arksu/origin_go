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
