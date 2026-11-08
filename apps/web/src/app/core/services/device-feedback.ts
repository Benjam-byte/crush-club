/**
 * Haptics and screen wake lock for the games played with the phones lying on the table. Both are best
 * effort: iOS ignores vibrations, and a browser may refuse the wake lock.
 */

export function vibrate(pattern: number | readonly number[]): void {
  try {
    if (typeof navigator !== 'undefined' && typeof navigator.vibrate === 'function') {
      navigator.vibrate(typeof pattern === 'number' ? pattern : [...pattern]);
    }
  } catch {
    // Some browsers throw outside a user gesture: the vibration is only a bonus.
  }
}

export function prefersReducedMotion(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** Keeps the screen on while held. The lock is lost when the page is hidden, so it is taken again on return. */
export class ScreenWakeLock {
  readonly isSupported = typeof navigator !== 'undefined' && 'wakeLock' in navigator;
  private sentinel: WakeLockSentinel | null = null;
  private isWanted = false;
  private readonly onVisibilityChange = (): void => {
    if (this.isWanted && document.visibilityState === 'visible') {
      void this.acquire();
    }
  };

  async request(): Promise<void> {
    if (this.isWanted) {
      return;
    }
    this.isWanted = true;
    document.addEventListener('visibilitychange', this.onVisibilityChange);
    await this.acquire();
  }

  async release(): Promise<void> {
    this.isWanted = false;
    document.removeEventListener('visibilitychange', this.onVisibilityChange);
    const sentinel = this.sentinel;
    this.sentinel = null;
    try {
      await sentinel?.release();
    } catch {
      // Already released by the browser.
    }
  }

  private async acquire(): Promise<void> {
    if (!this.isSupported || this.sentinel) {
      return;
    }
    try {
      const sentinel = await navigator.wakeLock.request('screen');
      if (!this.isWanted) {
        await sentinel.release();
        return;
      }
      this.sentinel = sentinel;
      sentinel.addEventListener('release', () => {
        if (this.sentinel === sentinel) {
          this.sentinel = null;
        }
      });
    } catch {
      // Refused (battery saver, hidden page): the table screen shows a tip instead.
    }
  }
}
