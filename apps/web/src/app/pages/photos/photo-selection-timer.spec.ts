import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  calculateRemainingSelectionSeconds,
  photoSelectionDurationMs,
  PhotoSelectionTimer,
} from './photo-selection-timer';

describe('PhotoSelectionTimer', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-08-01T10:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('calcule le temps restant depuis une échéance réelle', () => {
    const deadline = Date.now() + photoSelectionDurationMs;

    expect(calculateRemainingSelectionSeconds(deadline)).toBe(120);
    vi.setSystemTime(new Date(Date.now() + 90_500));
    expect(calculateRemainingSelectionSeconds(deadline)).toBe(30);
    vi.setSystemTime(new Date(Date.now() + 30_000));
    expect(calculateRemainingSelectionSeconds(deadline)).toBe(0);
  });

  it('expire une seule fois après deux minutes', () => {
    const onTick = vi.fn();
    const onExpire = vi.fn();
    const timer = new PhotoSelectionTimer();
    timer.start(onTick, onExpire);

    vi.advanceTimersByTime(photoSelectionDurationMs);
    vi.advanceTimersByTime(photoSelectionDurationMs);

    expect(onTick).toHaveBeenCalledWith(0);
    expect(onExpire).toHaveBeenCalledTimes(1);
  });

  it('détecte immédiatement un retour tardif de la galerie', () => {
    const onTick = vi.fn();
    const onExpire = vi.fn();
    const timer = new PhotoSelectionTimer();
    timer.start(onTick, onExpire);
    vi.setSystemTime(new Date(Date.now() + photoSelectionDurationMs + 1));

    timer.refresh();

    expect(onExpire).toHaveBeenCalledTimes(1);
    expect(onTick).toHaveBeenLastCalledWith(0);
  });

  it('peut être arrêté après une sélection réussie', () => {
    const onExpire = vi.fn();
    const timer = new PhotoSelectionTimer();
    timer.start(() => undefined, onExpire);

    timer.stop();
    vi.advanceTimersByTime(photoSelectionDurationMs * 2);

    expect(onExpire).not.toHaveBeenCalled();
  });
});
