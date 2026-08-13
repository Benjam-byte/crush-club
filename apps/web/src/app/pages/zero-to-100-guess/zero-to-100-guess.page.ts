import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon, IonRange } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { PhaseConfirmationComponent } from '@core/components/phase-confirmation/phase-confirmation.component';
import { PhaseTimerComponent } from '@core/components/phase-timer/phase-timer.component';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-zero-to-100-guess-page',
  imports: [IonButton, IonContent, IonIcon, IonRange, PageHeaderComponent, PhaseConfirmationComponent, PhaseTimerComponent],
  templateUrl: './zero-to-100-guess.page.html',
  styleUrl: './zero-to-100-guess.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ZeroToHundredGuessPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmitting = signal(false);
  protected readonly positionByPlayerId = signal<Record<string, number>>({});
  protected readonly remainingSeconds = signal(0);
  protected readonly nominees = computed(() => this.gameState.zeroToHundredGame()?.nominees ?? []);

  private countdownInterval: ReturnType<typeof setInterval> | null = null;
  private initializedForRound: number | null = null;

  constructor() {
    effect(() => {
      const nominees = this.nominees();
      const roundNumber = this.gameState.zeroToHundredGame()?.roundNumber ?? null;
      if (nominees.length === 0 || roundNumber === null || roundNumber === this.initializedForRound) {
        return;
      }
      this.initializedForRound = roundNumber;
      const initial: Record<string, number> = {};
      for (const nominee of nominees) {
        initial[nominee.playerId] = 50;
      }
      this.positionByPlayerId.set(initial);
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
      return;
    }
    this.refreshCountdown();
    this.countdownInterval = setInterval(() => this.refreshCountdown(), 250);
  }

  ngOnDestroy(): void {
    if (this.countdownInterval !== null) {
      clearInterval(this.countdownInterval);
    }
  }

  protected positionFor(playerId: string): number {
    return this.positionByPlayerId()[playerId] ?? 50;
  }

  protected onPositionChange(playerId: string, event: Event): void {
    const value = (event as CustomEvent<{ value: number }>).detail.value;
    this.positionByPlayerId.update((current) => ({ ...current, [playerId]: value }));
  }

  protected async onSubmit(): Promise<void> {
    const nominees = this.nominees();
    if (this.isSubmitting() || nominees.length === 0) {
      return;
    }
    const positions = this.positionByPlayerId();
    if (nominees.some((nominee) => positions[nominee.playerId] === undefined)) {
      return;
    }
    this.isSubmitting.set(true);
    try {
      await this.gameState.submitZeroToHundredGuesses(positions);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmitting.set(false);
    }
  }

  private refreshCountdown(): void {
    const deadline = this.gameState.zeroToHundredGame()?.submissionDeadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const remainingMs = new Date(deadline).getTime() - Date.now();
    this.remainingSeconds.set(Math.max(0, Math.ceil(remainingMs / 1000)));
  }
}
