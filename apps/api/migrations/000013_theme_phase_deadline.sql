-- +goose Up
ALTER TABLE fast_bio_games ADD COLUMN theme_phase_deadline timestamptz;
ALTER TABLE zero_to_100_games ADD COLUMN theme_phase_deadline timestamptz;
ALTER TABLE situation_games ADD COLUMN theme_phase_deadline timestamptz;

-- +goose Down
ALTER TABLE situation_games DROP COLUMN theme_phase_deadline;
ALTER TABLE zero_to_100_games DROP COLUMN theme_phase_deadline;
ALTER TABLE fast_bio_games DROP COLUMN theme_phase_deadline;
