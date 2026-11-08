import { describe, expect, it } from 'vitest';
import { wavelengthProposalHint, wavelengthRankingHint, wavelengthRoundsLabel } from './wavelength-vote-copy';

describe('Wavelength vote wording', () => {
  it('names the rounds a vote prepares', () => {
    expect(wavelengthRoundsLabel(1, 5, 3)).toBe('Manches 1 à 3 / 5');
    expect(wavelengthRoundsLabel(4, 5, 2)).toBe('Manches 4 à 5 / 5');
    expect(wavelengthRoundsLabel(4, 4, 1)).toBe('Manche 4 / 4');
  });

  it('explains which ranked spectra are played, and announces the next vote', () => {
    expect(wavelengthRankingHint(1, 5, 3)).toBe(
      'Les 3 premiers seront joués dans l’ordre, un par manche, sur les 3 prochaines manches. Ensuite, on revote pour les manches suivantes.',
    );
    expect(wavelengthRankingHint(1, 3, 3)).toBe(
      'Les 3 premiers seront joués dans l’ordre, un par manche, sur les 3 prochaines manches.',
    );
    expect(wavelengthRankingHint(4, 4, 1)).toBe('Seul le n°1 sera joué, à la prochaine manche.');
  });

  it('tells proposers how many spectra the vote keeps', () => {
    expect(wavelengthProposalHint(3)).toBe('Le vote retiendra les 3 spectres des 3 prochaines manches.');
    expect(wavelengthProposalHint(1)).toBe('Le vote retiendra le spectre de la prochaine manche.');
  });
});
