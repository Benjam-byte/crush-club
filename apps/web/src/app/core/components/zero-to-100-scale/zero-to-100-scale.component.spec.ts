import { describe, expect, it } from 'vitest';
import {
  assignLabelLanes,
  clampScaleValue,
  labelSpan,
  nearestPointId,
  nomineeColor,
  NOMINEE_COLORS,
  NOMINEE_TEXT_COLORS,
  rankLabel,
  scaleRanks,
  spreadPositions,
} from './zero-to-100-scale.component';
import type { ScalePoint } from './zero-to-100-scale.component';

describe('spreadPositions', () => {
  it('spreads five nominees evenly along the scale', () => {
    expect(spreadPositions(5)).toEqual([10, 30, 50, 70, 90]);
  });

  it('spreads three nominees evenly along the scale', () => {
    expect(spreadPositions(3)).toEqual([17, 50, 83]);
  });
});

describe('clampScaleValue', () => {
  it('rounds and keeps values between 0 and 100', () => {
    expect(clampScaleValue(-4)).toBe(0);
    expect(clampScaleValue(42.6)).toBe(43);
    expect(clampScaleValue(130)).toBe(100);
  });
});

describe('scaleRanks', () => {
  it('ranks values from the closest to 0, in input order', () => {
    expect(scaleRanks([70, 10, 90, 30])).toEqual([3, 1, 4, 2]);
  });

  it('gives tied values the same rank', () => {
    expect(scaleRanks([30, 10, 30, 80])).toEqual([2, 1, 2, 4]);
  });
});

describe('rankLabel', () => {
  it('writes French ordinals', () => {
    expect([1, 2, 5].map(rankLabel)).toEqual(['1er', '2e', '5e']);
  });
});

describe('labelSpan', () => {
  it('keeps labels inside the track at both ends', () => {
    expect(labelSpan(0, 60, 300)).toEqual({ start: 0, end: 60 });
    expect(labelSpan(100, 60, 300)).toEqual({ start: 240, end: 300 });
  });

  it('slides the label along the track with its value', () => {
    expect(labelSpan(50, 60, 300)).toEqual({ start: 120, end: 180 });
  });
});

describe('assignLabelLanes', () => {
  const span = (start: number, end: number) => ({ start, end });

  it('keeps labels that do not overlap on the first lane', () => {
    expect(assignLabelLanes([span(0, 40), span(50, 90), span(100, 140)])).toEqual([0, 0, 0]);
  });

  it('stacks overlapping labels and reuses a lane once there is room again', () => {
    expect(assignLabelLanes([span(0, 60), span(30, 90), span(62, 120), span(100, 160)])).toEqual([0, 1, 2, 0]);
  });

  it('returns lanes in input order whatever the positions order', () => {
    expect(assignLabelLanes([span(200, 240), span(10, 60), span(0, 40)])).toEqual([0, 1, 0]);
  });

  it('moves labels off the reserved spans of the first lane', () => {
    expect(assignLabelLanes([span(270, 290), span(100, 120)], [span(0, 10), span(280, 300)])).toEqual([1, 0]);
  });
});

describe('nearestPointId', () => {
  const points: ScalePoint[] = [
    { id: 'a', label: 'A', value: 20, colorIndex: 0 },
    { id: 'b', label: 'B', value: 50, colorIndex: 1 },
    { id: 'c', label: 'C', value: 50, colorIndex: 2 },
  ];

  it('picks the closest point', () => {
    expect(nearestPointId(points, 25, null)).toBe('a');
  });

  it('breaks ties in favor of the preferred point', () => {
    expect(nearestPointId(points, 55, 'b')).toBe('b');
  });

  it('otherwise breaks ties in favor of the point drawn on top', () => {
    expect(nearestPointId(points, 55, null)).toBe('c');
  });
});

describe('nomineeColor', () => {
  it('gives every seat of a full round its own color', () => {
    const colors = spreadPositions(NOMINEE_COLORS.length).map((_, seat) => nomineeColor(seat));
    expect(new Set(colors).size).toBe(NOMINEE_COLORS.length);
  });
});

describe('NOMINEE_TEXT_COLORS', () => {
  const luminance = (hex: string) => {
    const [red, green, blue] = (hex.match(/\w\w/g) ?? []).map((pair) => {
      const channel = parseInt(pair, 16) / 255;
      return channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
  };

  it('pairs every dot color with a text color', () => {
    expect(NOMINEE_TEXT_COLORS).toHaveLength(NOMINEE_COLORS.length);
  });

  it('keeps the values readable on the page background', () => {
    const background = luminance('#fff8f7');
    for (const color of NOMINEE_TEXT_COLORS) {
      expect((background + 0.05) / (luminance(color) + 0.05)).toBeGreaterThanOrEqual(4.5);
    }
  });
});
