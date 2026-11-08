import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { pickWaitingGameDefinition, readBestScore, saveBestScore, waitingGameDefinitionList } from './waiting-game-catalog';
import { findSwipeDirection, swipeThresholdPixels } from './waiting-game-engine';

describe('pickWaitingGameDefinition', () => {
  it('never picks the previous game again', () => {
    for (const previousDefinition of waitingGameDefinitionList) {
      for (const randomValue of [0, 0.5, 0.999]) {
        expect(pickWaitingGameDefinition(() => randomValue, previousDefinition.kind).kind).not.toBe(previousDefinition.kind);
      }
    }
  });

  it('can pick any game on the first waiting screen', () => {
    const pickedKindList = [0, 0.25, 0.5, 0.75].map((randomValue) => pickWaitingGameDefinition(() => randomValue, null).kind);
    expect(new Set(pickedKindList).size).toBe(waitingGameDefinitionList.length);
  });
});

describe('waiting game best scores', () => {
  let storedValueMap: Map<string, string>;

  beforeEach(() => {
    storedValueMap = new Map();
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storedValueMap.get(key) ?? null,
      setItem: (key: string, value: string) => storedValueMap.set(key, value),
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts at zero and keeps the highest score of each game', () => {
    expect(readBestScore('snake')).toBe(0);
    saveBestScore('snake', 12);
    saveBestScore('snake', 5);
    saveBestScore('runner', 40);
    expect(readBestScore('snake')).toBe(12);
    expect(readBestScore('runner')).toBe(40);
  });

  it('ignores a stored record with an unexpected shape', () => {
    storedValueMap.set('crush-club-waiting-game-best-v1', '{"snake":"lots"}');
    expect(readBestScore('snake')).toBe(0);
  });

  it('falls back to zero when storage is blocked', () => {
    const throwBlockedStorage = (): never => {
      throw new Error('Storage blocked');
    };
    vi.stubGlobal('localStorage', { getItem: throwBlockedStorage, setItem: throwBlockedStorage });
    expect(readBestScore('paddle')).toBe(0);
    expect(() => saveBestScore('paddle', 3)).not.toThrow();
  });
});

describe('findSwipeDirection', () => {
  it('waits until the finger has travelled far enough', () => {
    expect(findSwipeDirection(swipeThresholdPixels - 1, 0)).toBeNull();
  });

  it('keeps the dominant axis', () => {
    expect(findSwipeDirection(30, 10)).toBe('right');
    expect(findSwipeDirection(-30, 10)).toBe('left');
    expect(findSwipeDirection(10, 30)).toBe('down');
    expect(findSwipeDirection(10, -30)).toBe('up');
  });
});
