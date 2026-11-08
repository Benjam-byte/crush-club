import { InfoSheetComponent } from '@core/components/info-sheet/info-sheet.component';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import type { SituationRoundResultView } from '@core/models/game.models';
import { GameStateService } from '@core/services/game-state.service';
import { placeLabel, pointsForPlace, pointsLabel } from '@core/services/situation-points';

@Component({
  selector: 'app-situation-results-page',
  imports: [InfoSheetComponent, IonButton, IonContent, IonIcon, PageHeaderComponent, WaitingCardComponent],
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
  protected readonly myResult = computed(() => this.results().find((result) => result.isMine) ?? null);
  /** "1re = 4 pts · 2e = 3 pts · …": the Borda weights for this round's finalist count. */
  protected readonly pointScale = computed(() => {
    const finalistCount = this.results().length;
    return Array.from({ length: finalistCount }, (_, index) => {
      return `${placeLabel(index + 1)} = ${pointsLabel(pointsForPlace(index, finalistCount))}`;
    }).join(' · ');
  });
  protected readonly placeLabel = placeLabel;

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected memberNames(result: SituationRoundResultView): string {
    return result.members.map((member) => member.displayName).join(', ');
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
