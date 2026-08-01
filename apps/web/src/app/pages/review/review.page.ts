import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import type { OnInit } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import {
  IonButton,
  IonContent,
  IonIcon,
} from '@ionic/angular/standalone';
import type { AnswerValue, QuestionDefinition } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';

interface ReviewItem {
  id: string
  label: string
  answerLabel: string
  maximumScore: number
}

@Component({
  selector: 'app-review-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, RouterLink],
  templateUrl: './review.page.html',
  styleUrl: './review.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ReviewPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  protected readonly reviewItemList = computed<readonly ReviewItem[]>(() => {
    return this.gameState.activeQuestionList().map((question) => {
      return {
        id: question.id,
        label: question.label,
        answerLabel: this.formatAnswer(question, this.gameState.answerByQuestionId()[question.id]),
        maximumScore: question.maximumScore,
      };
    });
  });
  protected readonly selectedLoverItem = computed(() => {
    return this.reviewItemList().find(
      (reviewItem) => reviewItem.id === this.gameState.loverQuestionId(),
    );
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.loadQuestionnaire();
    } catch {
      // GameState exposes the load error.
    }
  }
  protected onLoverSelect(questionId: string): void {
    this.gameState.selectLoverQuestion(questionId);
  }

  protected onLockProfile(): void {
    this.gameState.lockProfile();
    void this.router.navigate(['/game/demo/reveal/1']);
  }

  private formatAnswer(question: QuestionDefinition, answer: AnswerValue | undefined): string {
    if (typeof answer === 'number') {
      return `${answer}/${question.maximum ?? 10}`;
    }

    if (typeof answer === 'string') {
      return question.options?.find((option) => option.id === answer)?.label ?? answer;
    }

    if (Array.isArray(answer)) {
      return answer.join(', ');
    }

    return 'Sans réponse';
  }
}
