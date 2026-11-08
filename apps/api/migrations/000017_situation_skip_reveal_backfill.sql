-- +goose Up
-- Version 14 was used twice (situation_skip_reveal and zero_to_100_five_nominees).
-- A database that recorded version 14 through the 0 à 100 file never ran the
-- Situation change, so it is replayed here; every statement is idempotent.
UPDATE situation_rounds SET phase = 'ranking' WHERE phase = 'revealing';
ALTER TABLE situation_rounds
  DROP CONSTRAINT IF EXISTS situation_rounds_phase_check,
  ADD CONSTRAINT situation_rounds_phase_check
    CHECK (phase IN ('proposing', 'dueling', 'ranking', 'results', 'completed')),
  DROP COLUMN IF EXISTS review_index;

-- +goose Down
-- Nothing to undo: version 14 owns this change.
SELECT 1;
