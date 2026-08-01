-- +goose Up
ALTER TABLE games
  DROP CONSTRAINT games_phase_check,
  ADD CONSTRAINT games_phase_check
    CHECK (phase IN ('collecting_submissions', 'reveal_and_vote', 'round_results', 'between_rounds', 'completed'));

-- +goose Down
UPDATE games SET phase = 'round_results' WHERE phase = 'between_rounds';
ALTER TABLE games
  DROP CONSTRAINT games_phase_check,
  ADD CONSTRAINT games_phase_check
    CHECK (phase IN ('collecting_submissions', 'reveal_and_vote', 'round_results', 'completed'));
