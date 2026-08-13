import { describe, expect, it } from 'vitest';
import { formatPhaseCountdown } from './phase-timer.component';

describe('formatPhaseCountdown', () => {
  it('formats a phase duration consistently', () => {
    expect(formatPhaseCountdown(125)).toBe('02:05');
  });

  it('clamps an expired phase to zero', () => {
    expect(formatPhaseCountdown(-1)).toBe('00:00');
  });
});
