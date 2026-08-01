import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import {
  IonButton,
  IonContent,
  IonIcon,
  IonSelect,
  IonSelectOption,
  IonToast,
} from '@ionic/angular/standalone';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';
import { GameConfigService } from '../../core/services/game-config.service';

@Component({
  selector: 'app-lobby-page',
  imports: [
    IonButton,
    IonContent,
    IonIcon,
    IonSelect,
    IonSelectOption,
    IonToast,
    PageHeaderComponent,
  ],
  templateUrl: './lobby.page.html',
  styleUrl: './lobby.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class LobbyPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  protected readonly gameConfigs = inject(GameConfigService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isToastOpen = signal(false);
  protected readonly isConfigUpdating = signal(false);
  protected readonly isStartingNextRound = signal(false);
  protected readonly leaderboard = computed(() => this.gameState.game()?.leaderboard ?? []);
  protected readonly nextRoundNumber = computed(() => (this.gameState.game()?.roundNumber ?? 0) + 1);
  protected readonly readyPlayerCount = computed(() => {
    return this.gameState.playerList().filter((player) => player.readyStatus === 'ready').length;
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
      if (this.gameState.isHost()) {
        await this.gameConfigs.initialize();
      }
    } catch {
      // Services expose their error messages in the template.
    }
  }
  protected async onCopyLink(): Promise<void> {
    try {
      const invitationPath = this.router.serializeUrl(
        this.router.createUrlTree(['/join'], {
          queryParams: { code: this.gameState.lobbyCode() },
        }),
      );
      const invitationUrl = new URL(invitationPath, window.location.origin).toString();
      await navigator.clipboard.writeText(invitationUrl);
      this.isToastOpen.set(true);
    } catch (error: unknown) {
      console.error('Unable to copy the lobby link', error);
    }
  }

  protected onToastDismiss(): void {
    this.isToastOpen.set(false);
  }

  protected onPreparePhotos(): void {
    void this.router.navigate(['/lobby', this.gameState.lobbyCode(), 'photos']);
  }

  protected async onConfigChange(event: Event): Promise<void> {
    const gameConfigId = (event as CustomEvent<{ value: string }>).detail.value;
    if (!gameConfigId || gameConfigId === this.gameState.gameConfigId()) {
      return;
    }
    this.isConfigUpdating.set(true);
    try {
      await this.gameState.changeLobbyConfig(gameConfigId);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isConfigUpdating.set(false);
    }
  }

  protected async onStartGame(): Promise<void> {
    if (!this.gameState.canStartGame()) {
      return;
    }

    try {
      await this.gameState.startGame();
      const game = this.gameState.game();
      if (game) {
        await this.router.navigate(['/game', this.gameState.lobbyCode(), 'round', game.roundNumber, 'role']);
      }
    } catch {
      // GameState exposes the API error.
    }
  }

  protected async onStartNextRound(): Promise<void> {
    if (!this.gameState.canStartNextRound()) {
      return;
    }
    this.isStartingNextRound.set(true);
    try {
      await this.gameState.startNextRound();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isStartingNextRound.set(false);
    }
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }

  protected async onExcludePlayer(playerId: string): Promise<void> {
    try {
      await this.gameState.excludePlayer(playerId);
    } catch {
      // GameState exposes the API error.
    }
  }
}
