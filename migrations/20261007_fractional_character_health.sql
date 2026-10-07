-- Stop the old server after its final save before applying this migration.
-- Integer health is exactly representable as double precision. Re-running this
-- migration preserves existing fractional values; do not cast back to INT after
-- fractional saves without checking that every value is integral and in range.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE origin.character
    DROP CONSTRAINT IF EXISTS character_shp_check,
    DROP CONSTRAINT IF EXISTS character_hhp_check,
    ALTER COLUMN shp TYPE DOUBLE PRECISION USING shp::double precision,
    ALTER COLUMN hhp TYPE DOUBLE PRECISION USING hhp::double precision,
    ADD CONSTRAINT character_shp_check
        CHECK (shp >= 0 AND shp < 'Infinity'::double precision),
    ADD CONSTRAINT character_hhp_check
        CHECK (hhp >= 0 AND hhp < 'Infinity'::double precision);

COMMIT;
