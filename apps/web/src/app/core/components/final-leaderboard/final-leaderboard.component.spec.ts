import { describe, expect, it } from 'vitest';
import { pointsLabel, rankEntries } from './final-leaderboard.component';

describe('pointsLabel', () => {
  it('shows scores in points by default', () => {
    expect(pointsLabel(12)).toBe('12 pts');
  });
});

describe('rankEntries', () => {
  it('gives tied scores the same rank and skips the next one', () => {
    const ranked = rankEntries([
      { id: 'a', name: 'Léa', score: 12 },
      { id: 'b', name: 'Tom', score: 12 },
      { id: 'c', name: 'Inès', score: 9 },
      { id: 'd', name: 'Noé', score: 4 },
    ]);
    expect(ranked.map((entry) => entry.rank)).toEqual([1, 1, 3, 4]);
  });

  it('ranks distinct scores in order', () => {
    const ranked = rankEntries([
      { id: 'a', name: 'Léa', score: 3 },
      { id: 'b', name: 'Tom', score: 2 },
    ]);
    expect(ranked.map((entry) => entry.rank)).toEqual([1, 2]);
  });
});
