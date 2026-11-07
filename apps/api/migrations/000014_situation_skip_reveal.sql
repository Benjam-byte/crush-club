-- +goose Up
-- Situation rounds now go straight from the duels to the finalist ranking:
-- the host-driven reveal carousel (and its cursor) is gone.
UPDATE situation_rounds SET phase = 'ranking' WHERE phase = 'revealing';
ALTER TABLE situation_rounds
  DROP CONSTRAINT situation_rounds_phase_check,
  ADD CONSTRAINT situation_rounds_phase_check
    CHECK (phase IN ('proposing', 'dueling', 'ranking', 'results', 'completed')),
  DROP COLUMN review_index;

-- +goose Down
ALTER TABLE situation_rounds
  ADD COLUMN review_index integer NOT NULL DEFAULT 0,
  DROP CONSTRAINT situation_rounds_phase_check,
  ADD CONSTRAINT situation_rounds_phase_check
    CHECK (phase IN ('proposing', 'dueling', 'revealing', 'ranking', 'results', 'completed'));
