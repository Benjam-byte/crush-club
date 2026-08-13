import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

export function formatPhaseCountdown(totalSeconds: number): string {
  const normalizedSeconds = Math.max(0, Math.floor(totalSeconds));
  const minuteCount = Math.floor(normalizedSeconds / 60);
  const secondCount = normalizedSeconds % 60;
  return `${minuteCount.toString().padStart(2, '0')}:${secondCount.toString().padStart(2, '0')}`;
}

@Component({
  selector: 'app-phase-timer',
  templateUrl: './phase-timer.component.html',
  styleUrl: './phase-timer.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PhaseTimerComponent {
  readonly label = input('Temps restant');
  readonly remainingSeconds = input.required<number>();
  readonly warningAt = input(30);

  protected readonly countdownLabel = computed(() => formatPhaseCountdown(this.remainingSeconds()));
  protected readonly isWarning = computed(() => this.remainingSeconds() <= this.warningAt());
}
