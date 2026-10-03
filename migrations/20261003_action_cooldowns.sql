-- Apply before deploying the action cooldown server.
ALTER TABLE origin.character
    ADD COLUMN IF NOT EXISTS action_cooldowns JSONB NOT NULL DEFAULT '{}'::jsonb;
