import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { AwaitingPlayersComponent } from '@core/components/awaiting-players/awaiting-players.component';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { PhaseConfirmationComponent } from '@core/components/phase-confirmation/phase-confirmation.component';
import { PhaseTimerComponent } from '@core/components/phase-timer/phase-timer.component';
import { WavelengthDialComponent } from '@core/components/wavelength-dial/wavelength-dial.component';
import { GameStateService } from '@core/services/game-state.service';

/** Clue and guessing phases of a Wavelength round, on one page so the dial stays put between them. */
@Component({
  selector: 'app-wavelength-round-page',
  imports: [
    AwaitingPlayersComponent,
    IonButton,
    IonContent,
    IonIcon,
    PageHeaderComponent,
    PhaseConfirmationComponent,
    PhaseTimerComponent,
    WavelengthDialComponent,
  ],
  templateUrl: './wavelength-round.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WavelengthRoundPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly wavelength = this.gameState.wavelengthGame;
  protected readonly isConfirmingClue = signal(false);
  protected readonly isSubmitting = signal(false);
  protected readonly position = signal(50);
  protected readonly remainingSeconds = signal(0);

  protected readonly isGuessing = computed(() => this.wavelength()?.roundPhase === 'guessing');
  protected readonly canPlaceNeedle = computed(() => {
    const wavelength = this.wavelength();
    return this.isGuessing() && !wavelength?.isPsychic && !wavelength?.submitted;
  });
  /** The guesser's needle, once they may place it; the psychic never has one. */
  protected readonly needleValue = computed(() => {
    const wavelength = this.wavelength();
    if (!this.isGuessing() || wavelength?.isPsychic) {
      return null;
    }
    return wavelength?.submitted ? wavelength.myPosition ?? this.position() : this.position();
  });

  private countdownInterval: ReturnType<typeof setInterval> | null = null;
  private initializedForRound: number | null = null;

  constructor() {
    effect(() => {
      const roundNumber = this.wavelength()?.roundNumber ?? null;
      if (roundNumber === null || roundNumber === this.initializedForRound) {
        return;
      }
      this.initializedForRound = roundNumber;
      this.position.set(this.wavelength()?.myPosition ?? 50);
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

  protected async onConfirmClue(): Promise<void> {
    if (this.isConfirmingClue()) {
      return;
    }
    this.isConfirmingClue.set(true);
    try {
      await this.gameState.confirmWavelengthClue();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isConfirmingClue.set(false);
    }
  }

  protected async onSubmit(): Promise<void> {
    if (this.isSubmitting() || !this.canPlaceNeedle()) {
      return;
    }
    this.isSubmitting.set(true);
    try {
      await this.gameState.submitWavelengthGuess(this.position());
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmitting.set(false);
    }
  }

  private refreshCountdown(): void {
    const deadline = this.wavelength()?.phaseDeadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const remainingMs = new Date(deadline).getTime() - Date.now();
    this.remainingSeconds.set(Math.max(0, Math.ceil(remainingMs / 1000)));
  }
}
