/** SVG frame of the dial: a half disc pivoting on the bottom center. */
export const dialWidth = 200;
export const dialHeight = 112;
export const dialCenterX = 100;
export const dialCenterY = 104;
export const dialRadius = 92;

/**
 * Scoring bands, by distance between a needle and the target.
 * Mirrors `wavelengthPoints` in wavelength_handlers.go.
 */
export const wavelengthScoringBands = [
  { radius: 3, points: 4 },
  { radius: 9, points: 3 },
  { radius: 15, points: 2 },
] as const;

export interface DialBand {
  from: number
  to: number
  points: number
}

export interface DialPoint {
  x: number
  y: number
}

/**
 * Bands drawn around the target, outermost first so the inner ones paint over
 * it. Each band spans half a step past its last scoring position, so its
 * edges sit between two integer positions.
 */
export function dialBands(target: number): DialBand[] {
  return [...wavelengthScoringBands].reverse().map(({ radius, points }) => ({
    from: Math.max(0, target - radius - 0.5),
    to: Math.min(100, target + radius + 0.5),
    points,
  }));
}

/** Position 0 sits on the left pole, 100 on the right one. */
export function pointOnDial(position: number, radius: number): DialPoint {
  const angle = Math.PI * (1 - position / 100);
  return {
    x: dialCenterX + radius * Math.cos(angle),
    y: dialCenterY - radius * Math.sin(angle),
  };
}

/** Pie wedge from the pivot, covering positions `from` to `to`. */
export function dialWedgePath(from: number, to: number, radius = dialRadius): string {
  const start = pointOnDial(from, radius);
  const end = pointOnDial(to, radius);
  return `M ${dialCenterX} ${dialCenterY} L ${format(start.x)} ${format(start.y)} ` +
    `A ${radius} ${radius} 0 0 1 ${format(end.x)} ${format(end.y)} Z`;
}

/**
 * Dial position (0-100) pointed at from an SVG point. Points below the pivot
 * line snap to the nearest pole.
 */
export function positionFromDialPoint(x: number, y: number): number {
  const dx = x - dialCenterX;
  const dy = dialCenterY - y;
  const angle = dy < 0 ? (dx < 0 ? Math.PI : 0) : Math.atan2(dy, dx);
  return clampPosition(Math.round(100 * (1 - angle / Math.PI)));
}

export function clampPosition(position: number): number {
  return Math.min(100, Math.max(0, position));
}

function format(value: number): string {
  return value.toFixed(2);
}
