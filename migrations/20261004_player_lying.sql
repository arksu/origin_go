-- Apply before deploying the timed KO server. KO deadlines remain runtime-only.
ALTER TABLE origin.character
    ADD COLUMN IF NOT EXISTS is_lying BOOLEAN NOT NULL DEFAULT false;
