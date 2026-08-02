import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
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
  protected readonly isLeaving = signal(false);

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected async onReplay(): Promise<void> {
    if (this.isLeaving()) {
      return;
    }
    this.isLeaving.set(true);
    const wasHost = this.gameState.isHost();
    try {
      await this.gameState.leaveGame();
      await this.router.navigate(['/join'], { queryParams: wasHost ? { mode: 'create' } : undefined });
    } finally {
      this.isLeaving.set(false);
    }
  }

  protected async onNewLobby(): Promise<void> {
    if (this.isLeaving()) {
      return;
    }
    this.isLeaving.set(true);
    try {
      await this.gameState.leaveGame();
      await this.router.navigate(['/']);
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
