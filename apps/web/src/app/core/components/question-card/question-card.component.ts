import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import type { AnswerValue, QuestionDefinition } from '@core/models/game.models';

@Component({
  selector: 'app-question-card',
  imports: [IonIcon],
  templateUrl: './question-card.component.html',
  styleUrl: './question-card.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class QuestionCardComponent {
  readonly question = input.required<QuestionDefinition>();
  readonly answer = input<AnswerValue>();
  readonly answerChange = output<AnswerValue>();

  protected readonly numericAnswer = computed(() => {
    const answer = this.answer();
    return typeof answer === 'number' ? answer : (this.question().minimum ?? 0);
  });

  protected onOptionSelect(optionId: string): void {
    this.answerChange.emit(optionId);
  }

  protected onRangeInput(event: Event): void {
    const rangeInput = event.target as HTMLInputElement;
    this.answerChange.emit(Number(rangeInput.value));
  }
}
