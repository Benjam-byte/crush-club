import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { MoneyInputComponent } from '@core/components/money-input/money-input.component';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { PhaseConfirmationComponent } from '@core/components/phase-confirmation/phase-confirmation.component';
import { PhaseTimerComponent } from '@core/components/phase-timer/phase-timer.component';
import { SecretRankingRulesComponent } from '@core/components/secret-ranking-rules/secret-ranking-rules.component';
import { vibrate } from '@core/services/device-feedback';
import { GameStateService } from '@core/services/game-state.service';
import { formatEuros } from '@core/services/money-format';
import { secretRankingQuestionEnd, secretRankingQuestionStart } from '@core/services/secret-ranking';

/** Screen A: the round's question, and the player's secret bet. */
@Component({
  selector: 'app-secret-ranking-secret-page',
  imports: [
    IonButton,
    IonContent,
    IonIcon,
    MoneyInputComponent,
    PageHeaderComponent,
    PhaseConfirmationComponent,
    PhaseTimerComponent,
    SecretRankingRulesComponent,
  ],
  templateUrl: './secret-ranking-secret.page.html',
  styleUrl: './secret-ranking-secret.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SecretRankingSecretPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly round = this.gameState.secretRankingGame;
  protected readonly questionStart = secretRankingQuestionStart;
  protected readonly formatEuros = formatEuros;
  protected readonly questionEnd = computed(() => secretRankingQuestionEnd(this.round()?.themeLabel ?? ''));
  protected readonly amount = signal<number | null>(null);
  protected readonly isSubmitting = signal(false);
  /** True right after this phone sealed its bet: the card flip only plays then, not after a reload. */
  protected readonly justSealed = signal(false);
  protected readonly remainingSeconds = signal(0);

  private countdownInterval: ReturnType<typeof setInterval> | null = null;
  private initializedForRound: number | null = null;

  constructor() {
    // Ionic can reuse this page from one round to the next: start each round from a blank amount.
    effect(() => {
      const roundNumber = this.round()?.roundNumber ?? null;
      if (roundNumber === null || roundNumber === this.initializedForRound) {
        return;
      }
      this.initializedForRound = roundNumber;
      this.amount.set(null);
      this.justSealed.set(false);
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

  protected async onSeal(): Promise<void> {
    const amount = this.amount();
    if (this.isSubmitting() || amount === null) {
      return;
    }
    this.isSubmitting.set(true);
    try {
      await this.gameState.submitSecretRankingAmount(amount);
      this.justSealed.set(true);
      vibrate(30);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmitting.set(false);
    }
  }

  private refreshCountdown(): void {
    const deadline = this.round()?.secretDeadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const serverNow = Date.now() + this.gameState.serverClockOffsetMs();
    this.remainingSeconds.set(Math.max(0, Math.ceil((Date.parse(deadline) - serverNow) / 1000)));
  }
}
