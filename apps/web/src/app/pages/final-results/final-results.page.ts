import { FinalLeaderboardComponent } from '@core/components/final-leaderboard/final-leaderboard.component';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent } from '@ionic/angular/standalone';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';
import { SessionMenuService } from '../../core/services/session-menu.service';

@Component({
  selector: 'app-final-results-page',
  imports: [WaitingCardComponent, FinalLeaderboardComponent, IonButton, IonContent, PageHeaderComponent],
  templateUrl: './final-results.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FinalResultsPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  protected readonly sessionMenu = inject(SessionMenuService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  protected readonly leaderboard = computed(() => this.gameState.game()?.leaderboard ?? []);
  protected readonly finalEntries = computed(() => this.leaderboard().map(entry => ({
    id: entry.playerId, name: entry.displayName, score: entry.score,
    subtitle: `${entry.exactCount} exactes`, photo: this.playerPhotoUrl(entry.playerId),
  })));
  protected readonly isLeaving = signal(false);

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected async onReplay(): Promise<void> {
    if (this.isLeaving() || !this.gameState.isHost()) return;
    this.isLeaving.set(true);
    try {
      await this.gameState.replayClassic();
    } catch {
      // GameState displays the API error in the global toast.
    } finally {
      this.isLeaving.set(false);
    }
  }

  protected playerPhotoUrl(playerId: string): string | null {
    const player = this.gameState.playerList().find((candidate) => candidate.id === playerId);
    return this.gameState.photoUrl(player?.photoIds[0]);
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
