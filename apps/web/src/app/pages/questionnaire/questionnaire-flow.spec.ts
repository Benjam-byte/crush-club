import { describe, expect, it } from 'vitest';
import { questionnaireStepCount } from './questionnaire-flow';

describe('questionnaireStepCount', () => {
  it('uses only the photo, tagline and personal questions for a custom questionnaire', () => {
    expect(questionnaireStepCount('cupid', 0, 3)).toBe(5);
    expect(questionnaireStepCount('lover', 0, 3)).toBe(4);
  });

  it('keeps profile fields in the system questionnaire flow', () => {
    expect(questionnaireStepCount('cupid', 5, 5)).toBe(12);
    expect(questionnaireStepCount('lover', 5, 5)).toBe(11);
  });
});
