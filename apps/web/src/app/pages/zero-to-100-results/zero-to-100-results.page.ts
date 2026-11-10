import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { InfoSheetComponent } from '@core/components/info-sheet/info-sheet.component';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { ZeroToTenPodiumComponent, podiumRevealDurationMs } from '@core/components/zero-to-10-podium/zero-to-10-podium.component';
import { nomineeColor, rankLabel } from '@core/components/zero-to-100-scale/zero-to-100-scale.component';
import { ZeroToHundredScoringRulesComponent } from '@core/components/zero-to-100-scoring-rules/zero-to-100-scoring-rules.component';
import { GameStateService } from '@core/services/game-state.service';
import { bigGapLine, nomineeName, orderGuessers, podiumNominees } from '@core/services/zero-to-10-results';

// Matches the gap between the carousel cards in the global .results-carousel.
const carouselGapPx = 12;

interface GuesserCard {
  id: string
  name: string
  isMine: boolean
  /** The current player did not answer in time: their card only says so. */
  isMissing: boolean
  scoreLabel: string
  perfectOrder: boolean
  rows: {
    nomineeId: string
    rankLabel: string
    name: string
    color: string
    note: number
    correct: boolean
    targetLabel: string
  }[]
}

@Component({
  selector: 'app-zero-to-100-results-page',
  imports: [
    InfoSheetComponent,
    IonButton,
    IonContent,
    IonIcon,
    PageHeaderComponent,
    WaitingCardComponent,
    ZeroToHundredScoringRulesComponent,
    ZeroToTenPodiumComponent,
  ],
  templateUrl: './zero-to-100-results.page.html',
  styleUrl: './zero-to-100-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ZeroToHundredResultsPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isStartingNextRound = signal(false);
  protected readonly themeLabel = computed(() => this.gameState.zeroToHundredGame()?.themeLabel ?? '');
  private readonly reveal = computed(() => this.gameState.zeroToHundredGame()?.reveal ?? []);
  protected readonly podium = computed(() => podiumNominees(this.reveal()));
  /** Nominees who did not rate themselves in time have no target, so no step on the podium. */
  protected readonly absentLine = computed(() => {
    const absent = this.reveal().filter((entry) => entry.targetPosition === undefined).map(nomineeName);
    return absent.length === 0 ? null : `Hors podium, pas noté·e à temps : ${absent.join(', ')}`;
  });
  protected readonly bigGap = computed(() => bigGapLine(this.reveal()));
  /** What follows the podium fades in once its 1st place has risen. */
  protected readonly afterPodiumDelay = computed(() => `${podiumRevealDurationMs(this.podium().map((nominee) => nominee.rank))}ms`);

  protected readonly cards = computed<GuesserCard[]>(() => {
    const reveal = this.reveal();
    const podiumSize = this.podium().length;
    const cards: GuesserCard[] = orderGuessers(this.gameState.zeroToHundredGame()?.guessers ?? []).map((guesser) => ({
      id: guesser.playerId,
      name: guesser.isCurrentPlayer ? 'Toi' : guesser.displayName,
      isMine: guesser.isCurrentPlayer,
      isMissing: false,
      scoreLabel: `${guesser.correctCount}/${podiumSize} bien placé${guesser.correctCount > 1 ? 's' : ''}`,
      perfectOrder: guesser.perfectOrder,
      rows: guesser.placements.map((placement) => {
        const seat = reveal.findIndex((entry) => entry.playerId === placement.nomineeId);
        const nominee = reveal[seat];
        return {
          nomineeId: placement.nomineeId,
          rankLabel: rankLabel(placement.rank),
          name: nominee === undefined ? '?' : nomineeName(nominee),
          color: nomineeColor(Math.max(0, seat)),
          note: placement.position,
          correct: placement.correct,
          targetLabel: nominee?.targetRank === undefined ? '' : `cible ${rankLabel(nominee.targetRank)}`,
        };
      }),
    }));
    if (!cards.some((card) => card.isMine)) {
      cards.unshift({ id: 'me', name: 'Toi', isMine: true, isMissing: true, scoreLabel: '', perfectOrder: false, rows: [] });
    }
    return cards;
  });

  /** The card centred in the carousel, mirrored by the dots under it. */
  protected readonly activeCardIndex = signal(0);
  protected readonly isLastRound = computed(() => {
    const zeroToHundred = this.gameState.zeroToHundredGame();
    return zeroToHundred?.roundNumber === zeroToHundred?.totalRounds;
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected onCarouselScroll(carousel: HTMLElement): void {
    const firstCard = carousel.firstElementChild;
    if (!firstCard) {
      return;
    }
    const cardStep = firstCard.getBoundingClientRect().width + carouselGapPx;
    this.activeCardIndex.set(Math.round(carousel.scrollLeft / cardStep));
  }

  protected scrollToCard(carousel: HTMLElement, index: number): void {
    carousel.children[index]?.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' });
  }

  protected async onStartNextRound(): Promise<void> {
    if (this.isStartingNextRound() || !this.gameState.isHost()) {
      return;
    }
    this.isStartingNextRound.set(true);
    try {
      await this.gameState.startNextZeroToHundredRound();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isStartingNextRound.set(false);
    }
  }
}
