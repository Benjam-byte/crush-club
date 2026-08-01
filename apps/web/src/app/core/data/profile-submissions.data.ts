import type { AnswerValue, QuestionDefinition } from '@core/models/game.models';
import { bioCategoryList } from './questionnaire.data';

export const bestTaglineBonusPointCount = 10;

export interface ProfileSubmission {
  id: string
  authorName: string
  avatarIndex: number
  tagline: string
  bioTagList: readonly string[]
  bioAnswerByCategoryId: Readonly<Record<string, string>>
  answerByQuestionId: Readonly<Record<string, AnswerValue>>
}

export interface CurrentProfileDraft {
  authorName: string
  avatarIndex: number
  tagline: string
  bioAnswerByCategoryId: Readonly<Record<string, string>>
  answerByQuestionId: Readonly<Record<string, AnswerValue>>
}

export interface ProfileComparisonItem {
  id: string
  label: string
  currentValue: string
  selectedValue: string
  isSame: boolean
}

const officialBioAnswerByCategoryId: Readonly<Record<string, string>> = {
  quality: 'funny',
  flaw: 'stubborn',
  passion: 'music',
  lifestyle: 'adventurous',
  intention: 'complicity',
};

const officialAnswerByQuestionId: Readonly<Record<string, AnswerValue>> = {
  romance: 7,
  'love-language': 'time',
  'first-date': 'picnic',
  weekend: 'improvise',
  intimacy: 8,
};

export const officialProfileSubmission: ProfileSubmission = createSubmission({
  id: 'profile-official',
  authorName: 'Camille',
  avatarIndex: 0,
  tagline: 'J’aime les gens vrais, les plans spontanés et le jeudi à en avoir mal aux joues.',
  bioAnswerByCategoryId: officialBioAnswerByCategoryId,
  answerByQuestionId: officialAnswerByQuestionId,
});

const fallbackCurrentSubmission: ProfileSubmission = createSubmission({
  id: 'player-current',
  authorName: 'Léa',
  avatarIndex: 0,
  tagline: 'Sensible, solaire, sélective… mais ouverte aux surprises.',
  bioAnswerByCategoryId: {
    quality: 'funny',
    flaw: 'stubborn',
    passion: 'music',
    lifestyle: 'adventurous',
    intention: 'see',
  },
  answerByQuestionId: {
    romance: 7,
    'love-language': 'time',
    'first-date': 'picnic',
    weekend: 'planned',
    intimacy: 9,
  },
});

const otherSubmissionList: readonly ProfileSubmission[] = [
  createSubmission({
    id: 'player-marco',
    authorName: 'Marco',
    avatarIndex: 1,
    tagline: 'Aventurière dans l’âme, dresseuse de playlists.',
    bioAnswerByCategoryId: {
      quality: 'funny',
      flaw: 'stubborn',
      passion: 'music',
      lifestyle: 'adventurous',
      intention: 'complicity',
    },
    answerByQuestionId: {
      romance: 7,
      'love-language': 'time',
      'first-date': 'picnic',
      weekend: 'planned',
      intimacy: 9,
    },
  }),
  createSubmission({
    id: 'player-ines',
    authorName: 'Inès',
    avatarIndex: 2,
    tagline: 'Je ne sais pas où je vais, mais j’ai une bonne playlist.',
    bioAnswerByCategoryId: {
      quality: 'funny',
      flaw: 'impatient',
      passion: 'music',
      lifestyle: 'adventurous',
      intention: 'serious',
    },
    answerByQuestionId: {
      romance: 8,
      'love-language': 'time',
      'first-date': 'picnic',
      weekend: 'improvise',
      intimacy: 8,
    },
  }),
  createSubmission({
    id: 'player-tom',
    authorName: 'Tom',
    avatarIndex: 3,
    tagline: 'Petit-déj ou bonne vibe ? Moi, je prends les deux.',
    bioAnswerByCategoryId: {
      quality: 'attentive',
      flaw: 'stubborn',
      passion: 'cooking',
      lifestyle: 'adventurous',
      intention: 'complicity',
    },
    answerByQuestionId: {
      romance: 4,
      'love-language': 'acts',
      'first-date': 'concert',
      weekend: 'improvise',
      intimacy: 6,
    },
  }),
];

export function createProfileSubmissionList(
  currentDraft: CurrentProfileDraft,
  questionList: readonly QuestionDefinition[],
): readonly ProfileSubmission[] {
  const hasCompleteDraft =
    currentDraft.tagline.trim().length > 0 &&
    bioCategoryList.every(
      (category) => currentDraft.bioAnswerByCategoryId[category.id] !== undefined,
    ) &&
    questionList.every(
      (question) => currentDraft.answerByQuestionId[question.id] !== undefined,
    );
  const currentSubmission = hasCompleteDraft
    ? createSubmission({
        id: 'player-current',
        authorName: currentDraft.authorName,
        avatarIndex: currentDraft.avatarIndex,
        tagline: currentDraft.tagline,
        bioAnswerByCategoryId: currentDraft.bioAnswerByCategoryId,
        answerByQuestionId: currentDraft.answerByQuestionId,
      })
    : {
        ...fallbackCurrentSubmission,
        authorName: currentDraft.authorName,
        avatarIndex: currentDraft.avatarIndex,
      };

  return [currentSubmission, ...otherSubmissionList];
}

export function calculateSimilarityPercentage(
  submission: ProfileSubmission,
  questionList: readonly QuestionDefinition[],
): number {
  const bioSimilarityTotal = bioCategoryList.reduce((similarityTotal, category) => {
    return similarityTotal +
      (submission.bioAnswerByCategoryId[category.id] ===
      officialBioAnswerByCategoryId[category.id]
        ? 1
        : 0);
  }, 0);
  const questionSimilarityTotal = questionList.reduce((similarityTotal, question) => {
    const answer = submission.answerByQuestionId[question.id];
    const officialAnswer = officialAnswerByQuestionId[question.id];

    if (question.type === 'integer_range' && typeof answer === 'number' && typeof officialAnswer === 'number') {
      const minimum = question.minimum ?? 0;
      const maximum = question.maximum ?? 10;
      const rangeSize = Math.max(1, maximum - minimum);
      return similarityTotal + Math.max(0, 1 - Math.abs(answer - officialAnswer) / rangeSize);
    }

    return similarityTotal + (answer === officialAnswer ? 1 : 0);
  }, 0);
  const answerCount = bioCategoryList.length + questionList.length;

  return Math.round(((bioSimilarityTotal + questionSimilarityTotal) / answerCount) * 100);
}

export function createProfileComparisonItemList(
  currentSubmission: ProfileSubmission,
  selectedSubmission: ProfileSubmission,
  questionList: readonly QuestionDefinition[],
): readonly ProfileComparisonItem[] {
  const taglineItem: ProfileComparisonItem = {
    id: 'tagline',
    label: 'Phrase d’accroche',
    currentValue: currentSubmission.tagline,
    selectedValue: selectedSubmission.tagline,
    isSame: currentSubmission.tagline === selectedSubmission.tagline,
  };
  const bioItemList = bioCategoryList.map<ProfileComparisonItem>((category) => {
    const currentAnswer = currentSubmission.bioAnswerByCategoryId[category.id];
    const selectedAnswer = selectedSubmission.bioAnswerByCategoryId[category.id];
    return {
      id: category.id,
      label: category.label,
      currentValue: formatBioAnswer(category.id, currentAnswer),
      selectedValue: formatBioAnswer(category.id, selectedAnswer),
      isSame: currentAnswer === selectedAnswer,
    };
  });
  const questionItemList = questionList.map<ProfileComparisonItem>((question) => {
    const currentAnswer = currentSubmission.answerByQuestionId[question.id];
    const selectedAnswer = selectedSubmission.answerByQuestionId[question.id];
    return {
      id: question.id,
      label: question.label,
      currentValue: formatQuestionAnswer(questionList, question.id, currentAnswer),
      selectedValue: formatQuestionAnswer(questionList, question.id, selectedAnswer),
      isSame: currentAnswer === selectedAnswer,
    };
  });

  return [taglineItem, ...bioItemList, ...questionItemList];
}

function createSubmission(
  submission: Omit<ProfileSubmission, 'bioTagList'>,
): ProfileSubmission {
  return {
    ...submission,
    bioTagList: bioCategoryList.map((category) => {
      return formatBioAnswer(category.id, submission.bioAnswerByCategoryId[category.id]);
    }),
  };
}

function formatBioAnswer(categoryId: string, answerId: string | undefined): string {
  const category = bioCategoryList.find((candidate) => candidate.id === categoryId);
  return category?.optionList.find((option) => option.id === answerId)?.label ?? 'Sans réponse';
}

function formatQuestionAnswer(
  questionList: readonly QuestionDefinition[],
  questionId: string,
  answer: AnswerValue | undefined,
): string {
  const question = questionList.find((candidate) => candidate.id === questionId);
  if (!question) {
    return 'Sans réponse';
  }

  if (typeof answer === 'number') {
    return `${answer}/${question.maximum ?? 10}`;
  }

  if (typeof answer === 'string') {
    return question.options?.find((option) => option.id === answer)?.label ?? answer;
  }

  return 'Sans réponse';
}
