/**
 * Wording of a Wavelength vote: it picks the spectra of the next rounds (up to three, one per round),
 * and the group votes again once they are played, until everyone has been the psychic.
 */

/** "Manches 1 à 3 / 5" for the rounds a vote prepares, "Manche 4 / 5" when it prepares only one. */
export function wavelengthRoundsLabel(roundNumber: number, totalRounds: number, spectraToPlay: number): string {
  if (spectraToPlay <= 1) {
    return `Manche ${roundNumber} / ${totalRounds}`;
  }
  return `Manches ${roundNumber} à ${roundNumber + spectraToPlay - 1} / ${totalRounds}`;
}

/** What the ranking decides, and whether another vote will follow. */
export function wavelengthRankingHint(roundNumber: number, totalRounds: number, spectraToPlay: number): string {
  const count = Math.max(spectraToPlay, 1);
  const decision = count === 1
    ? 'Seul le n°1 sera joué, à la prochaine manche.'
    : `Les ${count} premiers seront joués dans l’ordre, un par manche, sur les ${count} prochaines manches.`;
  const lastPreparedRound = roundNumber + count - 1;
  return lastPreparedRound < totalRounds ? `${decision} Ensuite, on revote pour les manches suivantes.` : decision;
}

/** Reminder on the proposal step of what the vote will pick. */
export function wavelengthProposalHint(spectraToPlay: number): string {
  return spectraToPlay > 1
    ? `Le vote retiendra les ${spectraToPlay} spectres des ${spectraToPlay} prochaines manches.`
    : 'Le vote retiendra le spectre de la prochaine manche.';
}
