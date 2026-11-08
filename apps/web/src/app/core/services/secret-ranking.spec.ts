import { describe, expect, it } from 'vitest';
import type {
  SecretRankingRevealEntryView,
  SecretRankingRoundSummaryView,
  SecretRankingSeatView,
} from '@core/models/game.models';
import {
  breaksLine,
  duplicateSeatNames,
  gameAwards,
  groupTitle,
  isTableNumbered,
  nextCyclePosition,
  placeShift,
  rankRangeLabel,
  revealedCount,
  revealStageAt,
  roundAwards,
  secretRankingQuestion,
  seatColor,
  secretRankingSeatColors,
} from './secret-ranking';
import type { RevealTimeline } from './secret-ranking';

function seat(playerId: string, position?: number): SecretRankingSeatView {
  return { playerId, displayName: playerId.toUpperCase(), colorIndex: 0, position };
}

function entry(
  displayName: string,
  position: number,
  amount: number,
  trueRankMin: number,
  trueRankMax = trueRankMin,
): SecretRankingRevealEntryView {
  return {
    playerId: displayName.toLowerCase(),
    displayName,
    colorIndex: 0,
    position,
    amount,
    wellPlaced: position >= trueRankMin && position <= trueRankMax,
    trueRankMin,
    trueRankMax,
  };
}

describe('secretRankingQuestion', () => {
  it('completes the fixed start of the question', () => {
    expect(secretRankingQuestion('manger un insecte')).toBe('Pour combien d’argent accepterais-tu de manger un insecte\u00a0?');
  });

  it('elides "de" before a vowel', () => {
    expect(secretRankingQuestion('aller à Lille')).toBe('Pour combien d’argent accepterais-tu d’aller à Lille\u00a0?');
    expect(secretRankingQuestion('écrire à ton ex')).toContain('d’écrire');
  });
});

describe('table numbering', () => {
  it('cycles a phone number from blank through the line and back to 1', () => {
    expect(nextCyclePosition(undefined, 3)).toBe(1);
    expect(nextCyclePosition(1, 3)).toBe(2);
    expect(nextCyclePosition(3, 3)).toBe(1);
  });

  it('names the other phones that share this number', () => {
    const seats = [seat('ana', 2), seat('ben', 2), seat('cleo', 1)];
    expect(duplicateSeatNames(seats, 'ana', 2)).toEqual(['BEN']);
    expect(duplicateSeatNames(seats, 'cleo', 1)).toEqual([]);
    expect(duplicateSeatNames(seats, 'ana', undefined)).toEqual([]);
  });

  it('accepts the table only once numbered 1..N without duplicates, with this phone’s pending number', () => {
    const seats = [seat('ana', 1), seat('ben', 1), seat('cleo', 3)];
    expect(isTableNumbered(seats, 'ana', 1)).toBe(false);
    expect(isTableNumbered(seats, 'ben', 2)).toBe(true);
    expect(isTableNumbered([seat('ana', 1), seat('ben')], 'ana', 1)).toBe(false);
    expect(isTableNumbered([], 'ana', 1)).toBe(false);
  });

  it('wraps seat colors around the palette', () => {
    expect(seatColor(secretRankingSeatColors.length + 2)).toBe(secretRankingSeatColors[2]);
  });
});

describe('domino reveal timeline', () => {
  const timeline: RevealTimeline = { revealAtMs: 10_000, stepMs: 1400, count: 3, endsAtMs: 10_000 + 2 * 1400 + 2500 };

  it('walks a phone through silence, drumroll, flip, verdict and the end', () => {
    expect(revealStageAt(timeline, 2, 9_000)).toBe('silence');
    expect(revealStageAt(timeline, 2, 10_900)).toBe('drumroll');
    expect(revealStageAt(timeline, 2, 11_400)).toBe('flipped');
    expect(revealStageAt(timeline, 2, 10_000 + 2 * 1400 + 900)).toBe('verdict');
    expect(revealStageAt(timeline, 2, timeline.endsAtMs)).toBe('over');
  });

  it('counts the phones already flipped', () => {
    expect(revealedCount(timeline, 9_999)).toBe(0);
    expect(revealedCount(timeline, 10_000)).toBe(1);
    expect(revealedCount(timeline, 11_400)).toBe(2);
    expect(revealedCount(timeline, 60_000)).toBe(3);
  });

  it('spots where the line breaks', () => {
    const reveal = [entry('Ana', 1, 500, 2), entry('Ben', 2, 50, 1), entry('Cleo', 3, 5000, 3)];
    expect(breaksLine(reveal, 1)).toBe(false);
    expect(breaksLine(reveal, 2)).toBe(true);
    expect(breaksLine(reveal, 3)).toBe(false);
  });
});

describe('scoring copy', () => {
  it('labels a tied rank range', () => {
    expect(rankRangeLabel(entry('Ana', 1, 0, 2, 3))).toBe('2–3');
    expect(rankRangeLabel(entry('Ana', 1, 0, 2))).toBe('2');
  });

  it('measures how far a phone is from its true place', () => {
    expect(placeShift(entry('Ana', 1, 0, 3))).toBe(2);
    expect(placeShift(entry('Ana', 4, 0, 2, 3))).toBe(-1);
    expect(placeShift(entry('Ana', 2, 0, 2, 3))).toBe(0);
  });

  it('titles the group score', () => {
    expect(groupTitle(4, 4)).toBe('Télépathes');
    expect(groupTitle(7, 10)).toBe('Vous vous connaissez par cœur');
    expect(groupTitle(2, 5)).toBe('Quelques surprises');
    expect(groupTitle(1, 5)).toBe('De parfaits inconnus');
    expect(groupTitle(0, 0)).toBe('Manche annulée');
  });

  it('hands out the round awards, ties included', () => {
    const reveal = [entry('Ana', 1, 500, 2), entry('Ben', 2, 50, 1), entry('Cleo', 3, 5000, 4), entry('Dan', 4, 50, 1, 2)];
    const awards = roundAwards(reveal, (amount) => `${amount} €`);
    expect(awards.map((award) => award.key)).toEqual(['smallest', 'largest', 'misjudged']);
    expect(awards[0].names).toEqual(['Ben', 'Dan']);
    expect(awards[1].names).toEqual(['Cleo']);
    expect(awards[2]).toMatchObject({ names: ['Dan'], detail: 'À 2 places de sa vraie place' });
  });

  it('skips the misjudged award on a perfect line', () => {
    const reveal = [entry('Ana', 1, 10, 1), entry('Ben', 2, 20, 2)];
    expect(roundAwards(reveal, String).map((award) => award.key)).toEqual(['smallest', 'largest']);
  });

  it('finds who was the cheapest and the priciest most often', () => {
    const round = (smallest: string[], largest: string[]): SecretRankingRoundSummaryView => ({
      roundNumber: 1,
      themeLabel: 'x',
      wellPlacedCount: 0,
      participantCount: 3,
      cancelled: false,
      smallestPlayerIds: smallest,
      smallestNames: smallest.map((id) => id.toUpperCase()),
      largestPlayerIds: largest,
      largestNames: largest.map((id) => id.toUpperCase()),
    });
    const awards = gameAwards([round(['ana'], ['ben']), round(['ana', 'cleo'], ['cleo']), round(['cleo'], ['ben'])]);
    expect(awards[0]).toMatchObject({ key: 'cheapest', names: ['ANA', 'CLEO'], detail: 'Le moins cher 2 fois' });
    expect(awards[1]).toMatchObject({ key: 'priciest', names: ['BEN'], detail: 'Le plus cher 2 fois' });
    expect(gameAwards([])).toEqual([]);
  });
});
