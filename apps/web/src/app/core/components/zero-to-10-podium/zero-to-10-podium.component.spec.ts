import { describe, expect, it } from 'vitest';
import { PODIUM_REVEAL_STEP_MS, PODIUM_RISE_MS, podiumRevealDurationMs, podiumSlots, revealSteps } from './zero-to-10-podium.component';

describe('podiumSlots', () => {
  it('puts the 1st in the middle and alternates left and right', () => {
    expect(podiumSlots(5)).toEqual([2, 1, 3, 0, 4]);
    expect(podiumSlots(3)).toEqual([1, 0, 2]);
  });

  it('keeps every slot inside the podium for even counts', () => {
    expect(podiumSlots(4)).toEqual([2, 1, 3, 0]);
    expect(podiumSlots(2)).toEqual([1, 0]);
  });

  it('handles a lone or empty podium', () => {
    expect(podiumSlots(1)).toEqual([0]);
    expect(podiumSlots(0)).toEqual([]);
  });
});

describe('revealSteps', () => {
  it('reveals the lowest step first', () => {
    expect(revealSteps([1, 2, 3])).toEqual([2, 1, 0]);
  });

  it('raises tied nominees together', () => {
    expect(revealSteps([1, 2, 2, 4])).toEqual([2, 1, 1, 0]);
  });
});

describe('podiumRevealDurationMs', () => {
  it('waits for every distinct step to rise', () => {
    expect(podiumRevealDurationMs([1, 2, 2, 4])).toBe(2 * PODIUM_REVEAL_STEP_MS + PODIUM_RISE_MS);
  });

  it('takes no time without a podium', () => {
    expect(podiumRevealDurationMs([])).toBe(0);
  });
});
