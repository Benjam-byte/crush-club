import { describe, expect, it } from 'vitest';
import {
  calculateSimilarityPercentage,
  createProfileComparisonItemList,
  createProfileSubmissionList,
} from './profile-submissions.data';
import { questionList } from './questionnaire.data';

const emptyDraft = {
  authorName: 'Léa',
  avatarIndex: 0,
  tagline: '',
  bioAnswerByCategoryId: {},
  answerByQuestionId: {},
};

describe('profile submissions', () => {
  it('calcule les similarités depuis les réponses structurées', () => {
    const submissionList = createProfileSubmissionList(emptyDraft, questionList);

    expect(submissionList.map((submission) => {
      return calculateSimilarityPercentage(submission, questionList);
    })).toEqual([79, 89, 79, 55]);
  });

  it('attribue 100 % à une proposition identique au profil officiel', () => {
    const [currentSubmission] = createProfileSubmissionList(
      {
        authorName: 'Léa',
        avatarIndex: 0,
        tagline: 'Une phrase complète',
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
          weekend: 'improvise',
          intimacy: 8,
        },
      },
      questionList,
    );

    expect(calculateSimilarityPercentage(currentSubmission, questionList)).toBe(100);
  });

  it('compare la phrase, les cinq éléments de bio et les cinq questions', () => {
    const [currentSubmission, selectedSubmission] = createProfileSubmissionList(emptyDraft, questionList);
    const comparisonItemList = createProfileComparisonItemList(
      currentSubmission,
      selectedSubmission,
      questionList,
    );

    expect(comparisonItemList).toHaveLength(11);
    expect(comparisonItemList[0]?.label).toBe('Phrase d’accroche');
    expect(comparisonItemList.find((item) => item.id === 'weekend')?.isSame).toBe(true);
  });

  it('limite les calculs et la comparaison au snapshot actif', () => {
    const activeQuestionList = questionList.filter((question) => question.id === 'weekend');
    const [currentSubmission, selectedSubmission] = createProfileSubmissionList(
      emptyDraft,
      activeQuestionList,
    );

    expect(createProfileComparisonItemList(
      currentSubmission,
      selectedSubmission,
      activeQuestionList,
    )).toHaveLength(7);
    expect(calculateSimilarityPercentage(selectedSubmission, activeQuestionList)).toBeGreaterThan(0);
  });
});
