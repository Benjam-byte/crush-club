import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import type { LeaderboardEntry } from '@core/models/game.models';
import { BrandMarkComponent } from '../../core/components/brand-mark/brand-mark.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-final-results-page',
  imports: [BrandMarkComponent, IonButton, IonContent, IonIcon, ProfilePortraitComponent],
  templateUrl: './final-results.page.html',
  styleUrl: './final-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FinalResultsPage {
  private readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  protected readonly leaderboardEntryList: readonly LeaderboardEntry[] = [
    { id: 'lea', displayName: 'Léa', avatarIndex: 0, score: 245, exactCount: 12 },
    { id: 'marco', displayName: 'Marco', avatarIndex: 1, score: 195, exactCount: 9 },
    { id: 'ines', displayName: 'Inès', avatarIndex: 2, score: 165, exactCount: 8 },
    { id: 'tom', displayName: 'Tom', avatarIndex: 3, score: 125, exactCount: 6 },
  ];

  protected onReplay(): void {
    this.gameState.resetGame();
    void this.router.navigate(['/join'], {
      queryParams: {
        mode: 'create',
      },
    });
  }

  protected onNewLobby(): void {
    this.gameState.resetGame();
    void this.router.navigate(['/']);
  }
}
