import { describe, expect, it } from 'vitest';
import type { ZeroToHundredGuesserView, ZeroToHundredRevealEntryView } from '@core/models/game.models';
import { bigGapLine, formatNote, goodPlacesLabel, orderGuessers, podiumNominees } from './zero-to-10-results';

const entry = (overrides: Partial<ZeroToHundredRevealEntryView> & { playerId: string }): ZeroToHundredRevealEntryView => ({
  displayName: overrides.playerId.toUpperCase(),
  isCurrentPlayer: false,
  ...overrides,
});

describe('formatNote', () => {
  it('writes notes the French way, without a useless decimal', () => {
    expect(formatNote(8.1)).toBe('8,1');
    expect(formatNote(8)).toBe('8');
    expect(formatNote(0)).toBe('0');
  });
});

describe('goodPlacesLabel', () => {
  it('agrees with the count', () => {
    expect(goodPlacesLabel(0)).toBe('0 bonne place');
    expect(goodPlacesLabel(1)).toBe('1 bonne place');
    expect(goodPlacesLabel(7)).toBe('7 bonnes places');
  });
});

describe('podiumNominees', () => {
  it('lists the nominees that have a target from the 1st step down, keeping their seat color', () => {
    const reveal = [
      entry({ playerId: 'a', targetPosition: 3.4, targetRank: 3 }),
      entry({ playerId: 'b', isCurrentPlayer: true, targetPosition: 8.1, targetRank: 1 }),
      entry({ playerId: 'c' }),
      entry({ playerId: 'd', targetPosition: 5, targetRank: 2 }),
    ];
    expect(podiumNominees(reveal)).toEqual([
      { id: 'b', name: 'Toi', target: 8.1, rank: 1, colorIndex: 1 },
      { id: 'd', name: 'D', target: 5, rank: 2, colorIndex: 3 },
      { id: 'a', name: 'A', target: 3.4, rank: 3, colorIndex: 0 },
    ]);
  });

  it('keeps tied nominees in seat order', () => {
    const reveal = [
      entry({ playerId: 'a', targetPosition: 5, targetRank: 1 }),
      entry({ playerId: 'b', targetPosition: 5, targetRank: 1 }),
    ];
    expect(podiumNominees(reveal).map((nominee) => nominee.id)).toEqual(['a', 'b']);
  });
});

describe('bigGapLine', () => {
  it('points out the nominee whose own note is the furthest from the group', () => {
    const reveal = [
      entry({ playerId: 'a', displayName: 'Ju', truePosition: 3, averagePosition: 8 }),
      entry({ playerId: 'b', truePosition: 9, averagePosition: 5.5 }),
    ];
    expect(bigGapLine(reveal)).toBe('Ju s’est mis 3, le groupe lui met 8 😏');
  });

  it('speaks to the current player', () => {
    const reveal = [entry({ playerId: 'a', isCurrentPlayer: true, truePosition: 9, averagePosition: 4.5 })];
    expect(bigGapLine(reveal)).toBe('Tu t’es mis 9, le groupe te met 4,5 😏');
  });

  it('stays quiet under the minimum gap or without both notes', () => {
    const reveal = [
      entry({ playerId: 'a', truePosition: 5, averagePosition: 7.5 }),
      entry({ playerId: 'b', truePosition: 1 }),
      entry({ playerId: 'c', averagePosition: 9 }),
    ];
    expect(bigGapLine(reveal)).toBeNull();
  });
});

describe('orderGuessers', () => {
  const guesser = (playerId: string, isCurrentPlayer = false): ZeroToHundredGuesserView => ({
    playerId,
    displayName: playerId,
    isCurrentPlayer,
    placements: [],
    correctCount: 0,
    perfectOrder: false,
  });

  it('puts the current player first and keeps the others in order', () => {
    const ordered = orderGuessers([guesser('a'), guesser('b'), guesser('me', true), guesser('c')]);
    expect(ordered.map((entry) => entry.playerId)).toEqual(['me', 'a', 'b', 'c']);
  });
});
