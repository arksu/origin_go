BEGIN;

SET LOCAL lock_timeout = '5s';

-- Alter the partitioned parent so every region partition preserves uint32 quality.
ALTER TABLE origin.object
	DROP CONSTRAINT IF EXISTS object_quality_check,
	ALTER COLUMN quality TYPE BIGINT USING quality::bigint,
	ADD CONSTRAINT object_quality_check
		CHECK (quality BETWEEN 0 AND 4294967295);

COMMIT;
