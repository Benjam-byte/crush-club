-- +goose Up
ALTER TABLE game_rounds
  DROP CONSTRAINT game_rounds_subject_player_id_fkey,
  ADD CONSTRAINT game_rounds_subject_player_id_fkey
    FOREIGN KEY (subject_player_id) REFERENCES players(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE game_rounds
  DROP CONSTRAINT game_rounds_subject_player_id_fkey,
  ADD CONSTRAINT game_rounds_subject_player_id_fkey
    FOREIGN KEY (subject_player_id) REFERENCES players(id) ON DELETE RESTRICT;
