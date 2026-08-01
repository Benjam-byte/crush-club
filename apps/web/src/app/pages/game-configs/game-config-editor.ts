import type {
  GameConfigQuestionInput,
  QuestionDefinition,
} from '@core/models/game.models';

export type EditorQuestionType = 'integer_range' | 'single_choice' | 'binary_choice'

export interface EditorQuestion {
  key: string
  source: 'system' | 'custom'
  id?: string
  questionId?: string
  label: string
  type: EditorQuestionType
  options: readonly string[]
  minimum: number
  maximum: number
}

export function createBlankEditorQuestion(key: string): EditorQuestion {
  return {
    key,
    source: 'custom',
    label: '',
    type: 'single_choice',
    options: ['', ''],
    minimum: 0,
    maximum: 10,
  };
}

export function createEditorQuestion(
  definition: QuestionDefinition,
  key = definition.id,
): EditorQuestion {
  const type = isEditorQuestionType(definition.type) ? definition.type : 'single_choice';
  const isPersonal = definition.kind === 'personal';
  return {
    key,
    source: isPersonal ? 'custom' : 'system',
    id: isPersonal ? definition.id : undefined,
    questionId: isPersonal ? undefined : definition.id,
    label: definition.label,
    type,
    options: definition.options?.map((option) => option.label) ?? [],
    minimum: definition.minimum ?? 0,
    maximum: definition.maximum ?? 10,
  };
}

export function moveEditorQuestion(
  questions: readonly EditorQuestion[],
  key: string,
  direction: -1 | 1,
): readonly EditorQuestion[] {
  const currentIndex = questions.findIndex((question) => question.key === key);
  const nextIndex = currentIndex + direction;
  if (currentIndex < 0 || nextIndex < 0 || nextIndex >= questions.length) {
    return questions;
  }
  const nextQuestions = [...questions];
  [nextQuestions[currentIndex], nextQuestions[nextIndex]] = [
    nextQuestions[nextIndex],
    nextQuestions[currentIndex],
  ];
  return nextQuestions;
}

export function isEditorQuestionValid(question: EditorQuestion): boolean {
  if (question.source === 'system') {
    return Boolean(question.questionId);
  }
  if (question.label.trim().length === 0) {
    return false;
  }
  switch (question.type) {
    case 'integer_range':
      return Number.isFinite(question.minimum)
        && Number.isFinite(question.maximum)
        && question.minimum < question.maximum;
    case 'single_choice':
      return question.options.length >= 2
        && question.options.length <= 20
        && question.options.every((option) => option.trim().length > 0);
    case 'binary_choice':
      return true;
  }
}

export function toGameConfigQuestionInput(
  question: EditorQuestion,
): GameConfigQuestionInput {
  if (question.source === 'system') {
    return { questionId: question.questionId };
  }
  return {
    id: question.id,
    label: question.label.trim(),
    type: question.type,
    options: question.type === 'single_choice'
      ? question.options.map((option) => option.trim())
      : undefined,
    minimum: question.type === 'integer_range' ? question.minimum : undefined,
    maximum: question.type === 'integer_range' ? question.maximum : undefined,
  };
}

function isEditorQuestionType(type: string): type is EditorQuestionType {
  return type === 'integer_range' || type === 'single_choice' || type === 'binary_choice';
}
