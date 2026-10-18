-- Apply before deploying ping-based character activity accounting.
BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE origin.character ADD COLUMN IF NOT EXISTS last_ping_at TIMESTAMPTZ;
COMMIT;
