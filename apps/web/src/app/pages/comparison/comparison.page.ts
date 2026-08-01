import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-comparison-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './comparison.page.html',
  styleUrl: './comparison.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ComparisonPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  protected readonly isAdvancing = signal(false);
  protected readonly roundResults = computed(() => this.gameState.game()?.roundResults ?? []);
  protected readonly leaderboard = computed(() => this.gameState.game()?.leaderboard ?? []);
  protected readonly isLastRound = computed(() => {
    const game = this.gameState.game();
    return game !== null && !game.nextSubjectPlayerId;
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected onProfileSelect(playerId: string): void {
    const game = this.gameState.game();
    if (game && this.roundResult(playerId)) {
      void this.router.navigate(['/game', this.gameState.lobbyCode(), 'round', game.roundNumber, 'scores', playerId]);
    }
  }

  protected roundResult(playerId: string) {
    return this.roundResults().find((result) => result.playerId === playerId);
  }

  protected async onNextRound(): Promise<void> {
    this.isAdvancing.set(true);
    try {
      await this.gameState.returnToLobby();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isAdvancing.set(false);
    }
  }

  protected onQuitGame(): void {
    this.gameState.resetGame();
    void this.router.navigate(['/']);
  }

  protected playerPhotoUrl(playerId: string): string | null {
    const player = this.gameState.playerList().find((candidate) => candidate.id === playerId);
    return this.gameState.photoUrl(player?.photoIds[0]);
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
