import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import type { LeaderboardEntry } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
import type { BadgeDefinition } from '../../core/models/game.models';

@Component({
  selector: 'app-round-results-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, ProfilePortraitComponent],
  templateUrl: './round-results.page.html',
  styleUrl: './round-results.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RoundResultsPage {
  private readonly router = inject(Router);

  protected readonly leaderboardEntryList: readonly LeaderboardEntry[] = [
    {
      id: 'lea',
      displayName: 'Léa',
      avatarIndex: 0,
      score: 87,
      exactCount: 4,
      badge: 'Soulmate',
    },
    {
      id: 'marco',
      displayName: 'Marco',
      avatarIndex: 1,
      score: 76,
      exactCount: 3,
    },
    {
      id: 'ines',
      displayName: 'Inès',
      avatarIndex: 2,
      score: 68,
      exactCount: 3,
    },
    {
      id: 'tom',
      displayName: 'Tom',
      avatarIndex: 3,
      score: 54,
      exactCount: 2,
    },
  ];
  protected readonly badgeList: readonly BadgeDefinition[] = [
    { icon: 'heart', label: 'Smooth Talker', owner: 'Marco', tone: 'brand' },
    { icon: 'heart', label: 'Risky Lover', owner: 'Léa', tone: 'purple' },
    { icon: 'close', label: 'Broken Heart', owner: 'Tom', tone: 'ink' },
    { icon: 'sparkles', label: 'Mind Reader', owner: 'Inès', tone: 'gold' },
  ];

  protected onShowFinalResults(): void {
    void this.router.navigate(['/game/demo/final']);
  }
}
