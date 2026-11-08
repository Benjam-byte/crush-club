-- +goose Up
-- Players who join a game already in progress wait for the host's approval
-- ('pending'); everyone else is 'admitted' straight away. exclusion_reason
-- records why a player left the lobby, so their client can be told.
ALTER TABLE players
  ADD COLUMN admission_status text NOT NULL DEFAULT 'admitted'
    CHECK (admission_status IN ('admitted', 'pending')),
  ADD COLUMN exclusion_reason text
    CHECK (exclusion_reason IN ('left', 'kicked', 'refused'));

-- is_active is the single definition of "takes part in the game": admitted
-- and still in the lobby. Game logic filters on it; lobby membership
-- (authentication, seats) still only looks at excluded_at.
ALTER TABLE players
  ADD COLUMN is_active boolean
    GENERATED ALWAYS AS (excluded_at IS NULL AND admission_status = 'admitted') STORED;

-- +goose Down
ALTER TABLE players DROP COLUMN IF EXISTS is_active;
ALTER TABLE players
  DROP COLUMN IF EXISTS exclusion_reason,
  DROP COLUMN IF EXISTS admission_status;
