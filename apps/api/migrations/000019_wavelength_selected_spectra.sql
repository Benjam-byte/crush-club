-- +goose Up
-- A Wavelength vote now selects up to three spectra, played one per round in
-- ranking order before the group votes again. round_number is the round that
-- plays the spectrum.
CREATE TABLE wavelength_selected_spectra (
  game_id uuid NOT NULL REFERENCES wavelength_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL CHECK (round_number >= 1),
  left_label text NOT NULL,
  right_label text NOT NULL,
  PRIMARY KEY (game_id, round_number)
);

-- +goose Down
DROP TABLE IF EXISTS wavelength_selected_spectra;
