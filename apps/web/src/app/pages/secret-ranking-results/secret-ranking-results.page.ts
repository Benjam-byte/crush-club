import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { prefersReducedMotion } from '@core/services/device-feedback';
import { GameStateService } from '@core/services/game-state.service';
import { formatEuros } from '@core/services/money-format';
import {
  awardNames,
  groupTitle,
  initials,
  placeShift,
  rankRangeLabel,
  roundAwards,
  secretRankingQuestion,
  seatColor,
} from '@core/services/secret-ranking';

const scoreCountUpMs = 900;

/** Screen C: the phones are picked up again; the true order, the misplacements and the round's awards. */
@Component({
  selector: 'app-secret-ranking-results-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, WaitingCardComponent],
  templateUrl: './secret-ranking-results.page.html',
  styleUrl: './secret-ranking-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SecretRankingResultsPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly round = this.gameState.secretRankingGame;
  protected readonly isStartingNextRound = signal(false);
  protected readonly displayedScore = signal(0);
  protected readonly question = computed(() => secretRankingQuestion(this.round()?.themeLabel ?? ''));
  protected readonly title = computed(() => groupTitle(this.round()?.wellPlacedCount ?? 0, this.round()?.participantCount ?? 0));
  /** The line in its true order, smallest amount first. */
  protected readonly trueOrder = computed(() =>
    [...(this.round()?.reveal ?? [])].sort((first, second) => first.amount - second.amount || first.trueRankMin - second.trueRankMin),
  );
  protected readonly awards = computed(() => roundAwards(this.round()?.reveal ?? [], formatEuros));
  protected readonly formatEuros = formatEuros;
  protected readonly rankRangeLabel = rankRangeLabel;
  protected readonly placeShift = placeShift;
  protected readonly seatColor = seatColor;
  protected readonly initials = initials;
  protected readonly awardNames = awardNames;
  protected readonly currentPlayerId = computed(() => this.gameState.state()?.currentPlayerId ?? '');

  private countUpFrame: number | null = null;
  private countedRound: number | null = null;

  constructor() {
    effect(() => {
      const round = this.round();
      if (!round?.roundNumber || round.roundNumber === this.countedRound) {
        return;
      }
      this.countedRound = round.roundNumber;
      this.countUp(round.wellPlacedCount);
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  ngOnDestroy(): void {
    if (this.countUpFrame !== null) {
      cancelAnimationFrame(this.countUpFrame);
    }
  }

  protected async onStartNextRound(): Promise<void> {
    if (this.isStartingNextRound() || !this.gameState.isHost()) {
      return;
    }
    this.isStartingNextRound.set(true);
    try {
      await this.gameState.startNextSecretRankingRound();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isStartingNextRound.set(false);
    }
  }

  protected shiftLabel(shift: number): string {
    if (shift === 0) {
      return '✓';
    }
    return shift > 0 ? `→ ${shift}` : `← ${-shift}`;
  }

  private countUp(target: number): void {
    if (this.countUpFrame !== null) {
      cancelAnimationFrame(this.countUpFrame);
    }
    if (prefersReducedMotion() || target === 0) {
      this.displayedScore.set(target);
      return;
    }
    const startedAt = performance.now();
    const step = (now: number) => {
      const progress = Math.min(1, (now - startedAt) / scoreCountUpMs);
      this.displayedScore.set(Math.round(target * (1 - Math.pow(1 - progress, 3))));
      this.countUpFrame = progress < 1 ? requestAnimationFrame(step) : null;
    };
    this.displayedScore.set(0);
    this.countUpFrame = requestAnimationFrame(step);
  }
}
