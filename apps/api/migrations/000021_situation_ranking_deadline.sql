-- +goose Up
-- The finalist ranking of a Situation round now has a deadline too: when it
-- expires, the round is scored with the rankings already sent.
ALTER TABLE situation_rounds ADD COLUMN ranking_deadline timestamptz;

-- +goose Down
ALTER TABLE situation_rounds DROP COLUMN IF EXISTS ranking_deadline;
