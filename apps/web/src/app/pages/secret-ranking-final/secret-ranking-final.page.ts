import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent } from '@ionic/angular/standalone';
import { AwardListComponent } from '@core/components/award-list/award-list.component';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { GameStateService } from '@core/services/game-state.service';
import { gameAwards, groupTitle, secretRankingQuestion } from '@core/services/secret-ranking';

/** Screen D: the group's total over the game, the rounds, and the game's awards. */
@Component({
  selector: 'app-secret-ranking-final-page',
  imports: [AwardListComponent, IonButton, IonContent, PageHeaderComponent, WaitingCardComponent],
  templateUrl: './secret-ranking-final.page.html',
  styleUrl: './secret-ranking-final.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SecretRankingFinalPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  protected readonly rounds = computed(() => this.gameState.secretRankingGame()?.rounds ?? []);
  protected readonly playedRounds = computed(() => this.rounds().filter((round) => !round.cancelled));
  protected readonly wellPlacedTotal = computed(() =>
    this.playedRounds().reduce((total, round) => total + round.wellPlacedCount, 0),
  );
  protected readonly participantTotal = computed(() =>
    this.playedRounds().reduce((total, round) => total + round.participantCount, 0),
  );
  protected readonly percent = computed(() =>
    this.participantTotal() > 0 ? Math.round((this.wellPlacedTotal() / this.participantTotal()) * 100) : 0,
  );
  protected readonly title = computed(() => groupTitle(this.wellPlacedTotal(), this.participantTotal()));
  protected readonly awards = computed(() => gameAwards(this.rounds()));
  protected readonly isReplaying = signal(false);
  protected readonly secretRankingQuestion = secretRankingQuestion;

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected async onReplay(): Promise<void> {
    if (this.isReplaying() || !this.gameState.isHost()) {
      return;
    }
    this.isReplaying.set(true);
    try {
      await this.gameState.replaySecretRanking();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isReplaying.set(false);
    }
  }

  protected onLeave(): void {
    // The lobby exit guard asks for confirmation, then releases the player's place.
    void this.router.navigate(['/']);
  }
}
