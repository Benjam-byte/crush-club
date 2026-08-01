import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
import {
  createProfileComparisonItemList,
  createProfileSubmissionList,
  officialProfileSubmission,
} from '../../core/data/profile-submissions.data';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-profile-comparison-page',
  imports: [
    IonButton,
    IonContent,
    IonIcon,
    PageHeaderComponent,
    ProfilePortraitComponent,
    RouterLink,
  ],
  templateUrl: './profile-comparison.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProfileComparisonPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly route = inject(ActivatedRoute);
  protected readonly scoresLink = '/game/demo/reveal/1/scores';
  protected readonly officialSubmission = officialProfileSubmission;
  private readonly selectedProfileId = this.route.snapshot.paramMap.get('profileId');

  protected readonly profileSubmissionList = computed(() => {
    const currentPlayer = this.gameState.currentPlayer();
    return createProfileSubmissionList(
      {
        authorName: currentPlayer.displayName,
        avatarIndex: currentPlayer.avatarIndex,
        tagline: this.gameState.tagline(),
        bioAnswerByCategoryId: this.gameState.bioAnswerByCategoryId(),
        answerByQuestionId: this.gameState.answerByQuestionId(),
      },
      this.gameState.activeQuestionList(),
    );
  });
  protected readonly selectedSubmission = computed(() => {
    return this.profileSubmissionList().find(
      (submission) => submission.id === this.selectedProfileId,
    );
  });
  protected readonly comparisonItemList = computed(() => {
    const selectedSubmission = this.selectedSubmission();
    return selectedSubmission
      ? createProfileComparisonItemList(
          this.officialSubmission,
          selectedSubmission,
          this.gameState.activeQuestionList(),
        )
      : [];
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.loadQuestionnaire();
    } catch {
      // GameState exposes the load error.
    }
  }
}
