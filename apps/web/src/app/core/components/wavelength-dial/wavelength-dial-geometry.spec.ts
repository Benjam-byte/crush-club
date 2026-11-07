import { describe, expect, it } from 'vitest';
import { dialBands, dialCenterX, dialCenterY, dialRadius, pointOnDial, positionFromDialPoint } from './wavelength-dial-geometry';

describe('wavelength dial geometry', () => {
  it('puts the poles on each side of the pivot and the middle on top', () => {
    expect(pointOnDial(0, dialRadius).x).toBeCloseTo(dialCenterX - dialRadius);
    expect(pointOnDial(100, dialRadius).x).toBeCloseTo(dialCenterX + dialRadius);
    expect(pointOnDial(50, dialRadius).y).toBeCloseTo(dialCenterY - dialRadius);
  });

  it('reads back the position a point on the dial was drawn at', () => {
    for (const position of [0, 17, 50, 83, 100]) {
      const point = pointOnDial(position, 60);
      expect(positionFromDialPoint(point.x, point.y)).toBe(position);
    }
  });

  it('snaps points below the pivot to the nearest pole', () => {
    expect(positionFromDialPoint(dialCenterX - 30, dialCenterY + 5)).toBe(0);
    expect(positionFromDialPoint(dialCenterX + 30, dialCenterY + 5)).toBe(100);
  });

  it('draws the 2-3-4 bands outermost first, clipped to the dial', () => {
    expect(dialBands(50)).toEqual([
      { from: 34.5, to: 65.5, points: 2 },
      { from: 40.5, to: 59.5, points: 3 },
      { from: 46.5, to: 53.5, points: 4 },
    ]);
    expect(dialBands(5)[0]).toEqual({ from: 0, to: 20.5, points: 2 });
  });
});
