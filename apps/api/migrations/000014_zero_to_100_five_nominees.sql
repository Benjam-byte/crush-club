-- +goose Up
ALTER TABLE zero_to_100_nominees
  DROP CONSTRAINT zero_to_100_nominees_seat_check,
  ADD CONSTRAINT zero_to_100_nominees_seat_check CHECK (seat BETWEEN 0 AND 4);

-- +goose Down
ALTER TABLE zero_to_100_nominees
  DROP CONSTRAINT zero_to_100_nominees_seat_check,
  ADD CONSTRAINT zero_to_100_nominees_seat_check CHECK (seat BETWEEN 0 AND 2);
