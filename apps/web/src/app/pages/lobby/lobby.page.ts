import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { Router } from '@angular/router';
import {
  IonButton,
  IonContent,
  IonIcon,
  IonSelect,
  IonSelectOption,
  IonToast,
} from '@ionic/angular/standalone';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
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
    ProfilePortraitComponent,
  ],
  templateUrl: './lobby.page.html',
  styleUrl: './lobby.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class LobbyPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  protected readonly gameConfigs = inject(GameConfigService);
  private readonly router = inject(Router);

  protected readonly isToastOpen = signal(false);
  protected readonly isConfigUpdating = signal(false);
  protected readonly readyPlayerCount = computed(() => {
    return this.gameState.playerList().filter((player) => player.readyStatus === 'ready').length;
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby();
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
      await this.router.navigate(['/game/demo/role']);
    } catch {
      // GameState exposes the API error.
    }
  }
}
