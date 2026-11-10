import type { ZeroToHundredGuesserView, ZeroToHundredRevealEntryView } from '@core/models/game.models';

const noteFormat = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 });

/** A note of the 0 à 10 scale, French style: "8,1", or "8" for a whole note. */
export function formatNote(note: number): string {
  return noteFormat.format(note);
}

/** "1 bonne place", "7 bonnes places". */
export function goodPlacesLabel(count: number): string {
  return count > 1 ? `${count} bonnes places` : `${count} bonne place`;
}

/** One nominee on the round's podium. */
export interface PodiumNominee {
  id: string
  /** "Toi" for the current player. */
  name: string
  target: number
  /** 1 for the highest target; ties share a rank. */
  rank: number
  /** Seat of the nominee, which picks their color. */
  colorIndex: number
}

/** The nominees that have a target, from the 1st step down, ties in seat order. */
export function podiumNominees(reveal: readonly ZeroToHundredRevealEntryView[]): PodiumNominee[] {
  return reveal
    .flatMap((entry, seat) =>
      entry.targetPosition === undefined || entry.targetRank === undefined
        ? []
        : [{ id: entry.playerId, name: nomineeName(entry), target: entry.targetPosition, rank: entry.targetRank, colorIndex: seat }],
    )
    .sort((left, right) => left.rank - right.rank || left.colorIndex - right.colorIndex);
}

export function nomineeName(entry: Pick<ZeroToHundredRevealEntryView, 'isCurrentPlayer' | 'displayName'>): string {
  return entry.isCurrentPlayer ? 'Toi' : entry.displayName;
}

/** Below this gap between a nominee's own note and the group's, nothing is worth pointing out. */
export const BIG_GAP_MINIMUM = 3;

/**
 * "Ju s’est mis 3, le groupe lui met 8 😏" for the nominee whose own note is the furthest
 * from the group's average, the first in seat order on a tie; null under BIG_GAP_MINIMUM.
 */
export function bigGapLine(reveal: readonly ZeroToHundredRevealEntryView[]): string | null {
  let widest: { entry: ZeroToHundredRevealEntryView; self: number; group: number } | null = null;
  for (const entry of reveal) {
    if (entry.truePosition === undefined || entry.averagePosition === undefined) {
      continue;
    }
    const gap = Math.abs(entry.truePosition - entry.averagePosition);
    if (gap >= BIG_GAP_MINIMUM && (widest === null || gap > Math.abs(widest.self - widest.group))) {
      widest = { entry, self: entry.truePosition, group: entry.averagePosition };
    }
  }
  if (widest === null) {
    return null;
  }
  const self = formatNote(widest.self);
  const group = formatNote(widest.group);
  return widest.entry.isCurrentPlayer
    ? `Tu t’es mis ${self}, le groupe te met ${group} 😏`
    : `${widest.entry.displayName} s’est mis ${self}, le groupe lui met ${group} 😏`;
}

/** The current player's card first, then the others in the server's order (best score first). */
export function orderGuessers(guessers: readonly ZeroToHundredGuesserView[]): ZeroToHundredGuesserView[] {
  return [...guessers.filter((guesser) => guesser.isCurrentPlayer), ...guessers.filter((guesser) => !guesser.isCurrentPlayer)];
}
