import type { GameConfig, GameConfigQuestionInput } from '@core/models/game.models';

export function duplicateConfigQuestionInputs(config: GameConfig): readonly GameConfigQuestionInput[] {
  return config.questions.map<GameConfigQuestionInput>((question) => ({
    label: question.label,
    type: question.type as GameConfigQuestionInput['type'],
    options: question.options?.map((option) => option.label),
    minimum: question.minimum,
    maximum: question.maximum,
  }));
}
