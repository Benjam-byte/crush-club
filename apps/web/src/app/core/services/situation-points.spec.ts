import { describe, expect, it } from 'vitest';
import { placeLabel, pointsForPlace, pointsLabel } from './situation-points';

describe('situation points', () => {
  it('gives N points to the first of N finalists and 1 to the last', () => {
    expect(pointsForPlace(0, 4)).toBe(4);
    expect(pointsForPlace(3, 4)).toBe(1);
  });

  it('labels places and points in French', () => {
    expect(placeLabel(1)).toBe('1re');
    expect(placeLabel(2)).toBe('2e');
    expect(pointsLabel(1)).toBe('1 pt');
    expect(pointsLabel(3)).toBe('3 pts');
  });
});
