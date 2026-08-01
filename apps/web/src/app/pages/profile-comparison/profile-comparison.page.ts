import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { IonButton, IonContent } from '@ionic/angular/standalone';
import type { AnswerValue } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-profile-comparison-page',
  imports: [IonButton, IonContent, PageHeaderComponent, RouterLink],
  templateUrl: './profile-comparison.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProfileComparisonPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly selectedPlayerId = this.route.snapshot.paramMap.get('profileId');
  protected readonly selectedResult = computed(() => {
    return this.gameState.game()?.roundResults?.find((result) => result.playerId === this.selectedPlayerId);
  });
  protected readonly scoresLink = computed(() => {
    const game = this.gameState.game();
    return game ? `/game/${this.gameState.lobbyCode()}/round/${game.roundNumber}/scores` : '/';
  });
  protected readonly selectedPlayerPhotoUrl = computed(() => {
    const player = this.gameState.playerList().find((candidate) => candidate.id === this.selectedPlayerId);
    return this.gameState.photoUrl(player?.photoIds[0]);
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected formatValue(itemId: string, value: AnswerValue): string {
    if (typeof value === 'number') {
      const question = this.gameState.activeQuestionList().find((candidate) => candidate.id === itemId);
      return `${value}/${question?.maximum ?? 10}`;
    }
    if (typeof value === 'string') {
      const question = this.gameState.activeQuestionList().find((candidate) => candidate.id === itemId);
      const profileField = this.gameState.profileFieldList().find((candidate) => candidate.id === itemId);
      return question?.options?.find((option) => option.id === value)?.label ??
        profileField?.options.find((option) => option.id === value)?.label ?? value;
    }
    return value.join(', ');
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
