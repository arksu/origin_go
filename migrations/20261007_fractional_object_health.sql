-- Use the updated server with a fresh or explicitly reset development database.
-- NULL is reserved for special dropped items; definition-backed objects require
-- an explicitly saved HP value. There is no definition-based backfill.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE origin.object
    DROP CONSTRAINT IF EXISTS object_hp_check,
    ALTER COLUMN hp TYPE DOUBLE PRECISION USING hp::double precision,
    ADD CONSTRAINT object_hp_check
        CHECK (hp >= 0 AND hp < 'Infinity'::double precision);

COMMIT;
