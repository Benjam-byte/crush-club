import { DecimalPipe } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-zero-to-100-results-page',
  imports: [DecimalPipe, IonButton, IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './zero-to-100-results.page.html',
  styleUrl: './zero-to-100-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ZeroToHundredResultsPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isStartingNextRound = signal(false);
  protected readonly reveal = computed(() => this.gameState.zeroToHundredGame()?.reveal ?? []);
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

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
