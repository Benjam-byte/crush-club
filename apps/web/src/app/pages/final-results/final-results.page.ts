import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { BrandMarkComponent } from '../../core/components/brand-mark/brand-mark.component';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-final-results-page',
  imports: [BrandMarkComponent, IonButton, IonContent, IonIcon],
  templateUrl: './final-results.page.html',
  styleUrl: './final-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FinalResultsPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  protected readonly leaderboard = computed(() => this.gameState.game()?.leaderboard ?? []);
  protected readonly winner = computed(() => this.leaderboard()[0]);

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected onReplay(): void {
    const wasHost = this.gameState.isHost();
    this.gameState.resetGame();
    void this.router.navigate(['/join'], { queryParams: wasHost ? { mode: 'create' } : undefined });
  }

  protected onNewLobby(): void {
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
