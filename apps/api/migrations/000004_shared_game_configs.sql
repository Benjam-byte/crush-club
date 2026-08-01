-- +goose Up
ALTER TABLE game_configs
  ADD COLUMN is_public boolean NOT NULL DEFAULT false;

ALTER TABLE game_configs
  ADD CONSTRAINT game_configs_system_not_public CHECK (NOT is_system OR NOT is_public);

CREATE INDEX game_configs_public_idx
  ON game_configs(updated_at DESC)
  WHERE is_public;

-- +goose Down
DROP INDEX IF EXISTS game_configs_public_idx;
ALTER TABLE game_configs DROP CONSTRAINT IF EXISTS game_configs_system_not_public;
ALTER TABLE game_configs DROP COLUMN IF EXISTS is_public;
