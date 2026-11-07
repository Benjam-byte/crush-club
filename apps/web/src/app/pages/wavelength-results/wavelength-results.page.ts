import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { WavelengthDialComponent } from '@core/components/wavelength-dial/wavelength-dial.component';
import type { WavelengthDialNeedle } from '@core/components/wavelength-dial/wavelength-dial.component';
import { pointsLabel } from '@core/services/situation-points';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-wavelength-results-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, WaitingCardComponent, WavelengthDialComponent],
  templateUrl: './wavelength-results.page.html',
  styleUrl: './wavelength-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WavelengthResultsPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly wavelength = this.gameState.wavelengthGame;
  protected readonly isStartingNextRound = signal(false);
  protected readonly reveal = computed(() => this.wavelength()?.reveal ?? []);
  protected readonly needles = computed<readonly WavelengthDialNeedle[]>(() => {
    const currentPlayerId = this.gameState.state()?.currentPlayerId;
    return this.reveal().map((entry) => ({
      id: entry.playerId,
      position: entry.position,
      label: this.initials(entry.displayName),
      isCurrentPlayer: entry.playerId === currentPlayerId,
    }));
  });
  protected readonly pointsLabel = pointsLabel;

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
      await this.gameState.startNextWavelengthRound();
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
