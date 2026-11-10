import type {
  SecretRankingRevealEntryView,
  SecretRankingRoundSummaryView,
  SecretRankingSeatView,
} from '@core/models/game.models';

/**
 * Rules and timeline of « Le classement secret » shared by its pages. The API decides every outcome; these
 * helpers only phrase and pace it on each phone.
 */

export const secretRankingQuestionStart = 'Pour combien d’argent accepterais-tu';

/** Each phone's background on the table, by seat color index. Dark enough for white text. */
export const secretRankingSeatColors: readonly string[] = [
  '#e63e5b',
  '#8e62c7',
  '#16877a',
  '#3b6fd8',
  '#c2410c',
  '#be3a78',
  '#4f8f28',
  '#6b5bd6',
  '#0b7fa3',
  '#a8551f',
];

export function seatColor(colorIndex: number): string {
  const count = secretRankingSeatColors.length;
  return secretRankingSeatColors[((colorIndex % count) + count) % count];
}

/** Includes the separating space when the preposition is not elided. */
export function secretRankingQuestionPreposition(theme: string): string {
  return /^[aeiouyàâäéèêëîïôöùûü]/i.test(theme.trim()) ? 'd’' : 'de ';
}

/** "de manger un insecte ?" or "d’aller à Lille ?": the end of the question, elided before a vowel. */
export function secretRankingQuestionEnd(theme: string): string {
  return `${secretRankingQuestionPreposition(theme)}${theme.trim()}\u00a0?`;
}

export function secretRankingQuestion(theme: string): string {
  return `${secretRankingQuestionStart} ${secretRankingQuestionEnd(theme)}`;
}

/** A tap on a phone moves its number on: – → 1 → 2 → … → count → 1. */
export function nextCyclePosition(current: number | null | undefined, count: number): number {
  if (!current || current >= count) {
    return 1;
  }
  return current + 1;
}

/** The other phones showing the same number as this one. */
export function duplicateSeatNames(
  seats: readonly SecretRankingSeatView[],
  playerId: string,
  position: number | null | undefined,
): string[] {
  if (!position) {
    return [];
  }
  return seats.filter((seat) => seat.playerId !== playerId && seat.position === position).map((seat) => seat.displayName);
}

/** Whether the seats are numbered 1..N exactly once each, given this phone's own (possibly unsent) number. */
export function isTableNumbered(
  seats: readonly SecretRankingSeatView[],
  playerId: string,
  myPosition: number | null | undefined,
): boolean {
  const positions = seats.map((seat) => (seat.playerId === playerId ? myPosition ?? 0 : seat.position ?? 0));
  if (positions.length === 0) {
    return false;
  }
  const seen = new Set<number>();
  for (const position of positions) {
    if (position < 1 || position > positions.length || seen.has(position)) {
      return false;
    }
    seen.add(position);
  }
  return true;
}

// --- domino reveal --------------------------------------------------------

/** The "Silence…" before phone #1 flips, mirrored from the API's reveal lead. */
export const revealSilenceMs = 2000;
/** How long before its turn a phone's pulse speeds up. */
export const revealDrumrollMs = 600;
/** The verdict lights up this long after the last phone flipped. */
export const revealVerdictDelayMs = 900;

export interface RevealTimeline {
  revealAtMs: number
  stepMs: number
  count: number
  endsAtMs: number
}

export type RevealStage = 'silence' | 'drumroll' | 'flipped' | 'verdict' | 'over';

export function revealSlotAt(timeline: RevealTimeline, position: number): number {
  return timeline.revealAtMs + (position - 1) * timeline.stepMs;
}

export function revealVerdictAt(timeline: RevealTimeline): number {
  return revealSlotAt(timeline, Math.max(timeline.count, 1)) + revealVerdictDelayMs;
}

/** Where a phone stands in the reveal at serverNowMs. A phone with no position follows the whole line. */
export function revealStageAt(timeline: RevealTimeline, position: number | null, serverNowMs: number): RevealStage {
  if (serverNowMs >= timeline.endsAtMs) {
    return 'over';
  }
  if (serverNowMs >= revealVerdictAt(timeline)) {
    return 'verdict';
  }
  const slot = revealSlotAt(timeline, position ?? timeline.count);
  if (serverNowMs >= slot) {
    return 'flipped';
  }
  return serverNowMs >= slot - revealDrumrollMs ? 'drumroll' : 'silence';
}

/** How many phones of the line have flipped at serverNowMs. */
export function revealedCount(timeline: RevealTimeline, serverNowMs: number): number {
  if (serverNowMs < timeline.revealAtMs) {
    return 0;
  }
  return Math.min(timeline.count, Math.floor((serverNowMs - timeline.revealAtMs) / timeline.stepMs) + 1);
}

/** Whether the phone at this position shows less than the phone before it: the line "breaks" there. */
export function breaksLine(reveal: readonly SecretRankingRevealEntryView[], position: number): boolean {
  const current = reveal.find((entry) => entry.position === position);
  const previous = reveal.find((entry) => entry.position === position - 1);
  return current !== undefined && previous !== undefined && current.amount < previous.amount;
}

// --- scoring copy -----------------------------------------------------------

export function rankRangeLabel(entry: Pick<SecretRankingRevealEntryView, 'trueRankMin' | 'trueRankMax'>): string {
  return entry.trueRankMin === entry.trueRankMax ? `${entry.trueRankMin}` : `${entry.trueRankMin}–${entry.trueRankMax}`;
}

/** How many places to the right (positive) or the left (negative) the true rank lies. 0 when well placed. */
export function placeShift(entry: Pick<SecretRankingRevealEntryView, 'position' | 'trueRankMin' | 'trueRankMax'>): number {
  if (entry.position < entry.trueRankMin) {
    return entry.trueRankMin - entry.position;
  }
  if (entry.position > entry.trueRankMax) {
    return entry.trueRankMax - entry.position;
  }
  return 0;
}

export function groupTitle(wellPlacedCount: number, participantCount: number): string {
  if (participantCount <= 0) {
    return 'Manche annulée';
  }
  const ratio = wellPlacedCount / participantCount;
  if (ratio >= 1) {
    return 'Télépathes';
  }
  if (ratio >= 0.7) {
    return 'Vous vous connaissez par cœur';
  }
  if (ratio >= 0.4) {
    return 'Quelques surprises';
  }
  return 'De parfaits inconnus';
}

export interface SecretRankingAward {
  key: string
  emoji: string
  title: string
  names: readonly string[]
  detail: string
}

function joinNames(names: readonly string[]): string {
  if (names.length <= 1) {
    return names[0] ?? '';
  }
  return `${names.slice(0, -1).join(', ')} et ${names[names.length - 1]}`;
}

export function awardNames(award: SecretRankingAward): string {
  return joinNames(award.names);
}

/** The debrief's three awards: smallest bet, largest bet, most misjudged phone. */
export function roundAwards(
  reveal: readonly SecretRankingRevealEntryView[],
  formatAmount: (amount: number) => string,
): SecretRankingAward[] {
  if (reveal.length === 0) {
    return [];
  }
  const amounts = reveal.map((entry) => entry.amount);
  const smallest = Math.min(...amounts);
  const largest = Math.max(...amounts);
  const awards: SecretRankingAward[] = [
    {
      key: 'smallest',
      emoji: '🤝',
      title: 'Prix d’ami',
      names: reveal.filter((entry) => entry.amount === smallest).map((entry) => entry.displayName),
      detail: `Le plus petit montant : ${formatAmount(smallest)}`,
    },
  ];
  if (largest !== smallest) {
    awards.push({
      key: 'largest',
      emoji: '💎',
      title: 'Hors de prix',
      names: reveal.filter((entry) => entry.amount === largest).map((entry) => entry.displayName),
      detail: `Le plus gros montant : ${formatAmount(largest)}`,
    });
  }
  const shifts = reveal.map((entry) => Math.abs(placeShift(entry)));
  const largestShift = Math.max(...shifts);
  if (largestShift > 0) {
    awards.push({
      key: 'misjudged',
      emoji: '🙈',
      title: 'Le plus mal jugé',
      names: reveal.filter((_, index) => shifts[index] === largestShift).map((entry) => entry.displayName),
      detail: `À ${largestShift} place${largestShift > 1 ? 's' : ''} de sa vraie place`,
    });
  }
  return awards;
}

/** The final screen's awards: who was the cheapest, and the priciest, most often. */
export function gameAwards(rounds: readonly SecretRankingRoundSummaryView[]): SecretRankingAward[] {
  const awards: SecretRankingAward[] = [];
  const mostOften = (
    pick: (round: SecretRankingRoundSummaryView) => { ids: readonly string[], names: readonly string[] },
  ): { names: string[], count: number } => {
    const countById = new Map<string, number>();
    const nameById = new Map<string, string>();
    for (const round of rounds) {
      const { ids, names } = pick(round);
      ids.forEach((id, index) => {
        countById.set(id, (countById.get(id) ?? 0) + 1);
        nameById.set(id, names[index] ?? '');
      });
    }
    const count = Math.max(0, ...countById.values());
    const names = [...countById.entries()].filter(([, value]) => value === count).map(([id]) => nameById.get(id) ?? '');
    return { names, count };
  };
  const cheapest = mostOften((round) => ({ ids: round.smallestPlayerIds, names: round.smallestNames }));
  if (cheapest.count > 0) {
    awards.push({
      key: 'cheapest',
      emoji: '🛒',
      title: 'Le plus facile à acheter',
      names: cheapest.names,
      detail: `Le moins cher ${cheapest.count} fois`,
    });
  }
  const priciest = mostOften((round) => ({ ids: round.largestPlayerIds, names: round.largestNames }));
  if (priciest.count > 0) {
    awards.push({
      key: 'priciest',
      emoji: '🛡️',
      title: 'Le plus incorruptible',
      names: priciest.names,
      detail: `Le plus cher ${priciest.count} fois`,
    });
  }
  return awards;
}

export function initials(displayName: string): string {
  return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
}
