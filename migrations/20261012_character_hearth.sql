BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE origin.character_hearth
(
	character_id BIGINT PRIMARY KEY
		REFERENCES origin.character (id) ON DELETE RESTRICT,
	object_region INT NOT NULL,
	object_id BIGINT NOT NULL,
	CONSTRAINT character_hearth_object_unique UNIQUE (object_region, object_id),
	CONSTRAINT character_hearth_object_fk FOREIGN KEY (object_region, object_id)
		REFERENCES origin.object (region, id) ON DELETE RESTRICT
);

COMMIT;
