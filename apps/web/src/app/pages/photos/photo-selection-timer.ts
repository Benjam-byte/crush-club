export const photoSelectionDurationMs = 2 * 60 * 1000;

export function calculateRemainingSelectionSeconds(deadline: number, now = Date.now()): number {
  return Math.max(0, Math.ceil((deadline - now) / 1000));
}

export class PhotoSelectionTimer {
  private deadline = 0;
  private interval: ReturnType<typeof setInterval> | null = null;
  private active = false;
  private onTick: (remainingSeconds: number) => void = () => undefined;
  private onExpire: () => void = () => undefined;

  start(
    onTick: (remainingSeconds: number) => void,
    onExpire: () => void,
  ): void {
    this.stop();
    this.active = true;
    this.deadline = Date.now() + photoSelectionDurationMs;
    this.onTick = onTick;
    this.onExpire = onExpire;
    this.refresh();

    if (this.active) {
      this.interval = setInterval(() => this.refresh(), 250);
    }
  }

  refresh(): void {
    if (!this.active) {
      return;
    }

    const remainingSeconds = calculateRemainingSelectionSeconds(this.deadline);
    this.onTick(remainingSeconds);

    if (remainingSeconds === 0) {
      const onExpire = this.onExpire;
      this.stop();
      onExpire();
    }
  }

  stop(): void {
    this.active = false;

    if (this.interval !== null) {
      clearInterval(this.interval);
      this.interval = null;
    }
  }
}
