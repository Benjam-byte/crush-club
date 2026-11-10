import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import type { SituationRoundResultView } from '@core/models/game.models';
import { GameStateService } from '@core/services/game-state.service';
import { placeLabel } from '@core/services/situation-points';

// Matches the gap between the carousel cards in the stylesheet.
const carouselGapPx = 12;

@Component({
  selector: 'app-situation-results-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, WaitingCardComponent],
  templateUrl: './situation-results.page.html',
  styleUrl: './situation-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SituationResultsPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isStartingNextRound = signal(false);
  protected readonly isLastRound = computed(() => {
    const situation = this.gameState.situationGame();
    return situation?.roundNumber === situation?.totalRounds;
  });
  protected readonly results = computed(() => this.gameState.situationGame()?.roundResults ?? []);
  protected readonly placeLabel = placeLabel;
  /** The team card centred in the carousel, mirrored by the dots under it. */
  protected readonly activeCardIndex = signal(0);

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected tierFor(rank: number): string {
    return ['gold', 'silver', 'bronze'][rank - 1] ?? 'plain';
  }

  /** The server gives tied teams the same rank: they get an "ex æquo" badge. */
  protected isTied(result: SituationRoundResultView): boolean {
    return this.results().some((other) => other !== result && other.rank === result.rank);
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
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
      await this.gameState.startNextSituationRound();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isStartingNextRound.set(false);
    }
  }
}
