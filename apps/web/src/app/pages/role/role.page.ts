import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { BrandMarkComponent } from '../../core/components/brand-mark/brand-mark.component';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-role-page',
  imports: [BrandMarkComponent, IonButton, IonContent, IonIcon],
  templateUrl: './role.page.html',
  styleUrl: './role.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RolePage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  protected readonly subjectPhotoUrl = computed(() => {
    return this.gameState.photoUrl(this.gameState.subjectPlayer().photoIds[0]);
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected onStartQuestionnaire(): void {
    const game = this.gameState.game();
    if (game) {
      void this.router.navigate(['/game', this.gameState.lobbyCode(), 'round', game.roundNumber, 'profile']);
    }
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
