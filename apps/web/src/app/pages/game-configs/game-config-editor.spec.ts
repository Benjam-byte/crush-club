import { describe, expect, it } from 'vitest';
import {
  createBlankEditorQuestion,
  isEditorQuestionValid,
  moveEditorQuestion,
  toGameConfigQuestionInput,
  type EditorQuestion,
} from './game-config-editor';

describe('game config editor', () => {
  it('valide une liste avec au moins deux options renseignées', () => {
    const blankQuestion = createBlankEditorQuestion('question-1');
    expect(isEditorQuestionValid(blankQuestion)).toBe(false);

    const validQuestion: EditorQuestion = {
      ...blankQuestion,
      label: 'Son activité préférée ?',
      options: ['Sport', 'Cinéma', 'Cuisine'],
    };
    expect(isEditorQuestionValid(validQuestion)).toBe(true);
    expect(toGameConfigQuestionInput(validQuestion)).toEqual({
      id: undefined,
      label: 'Son activité préférée ?',
      type: 'single_choice',
      options: ['Sport', 'Cinéma', 'Cuisine'],
      minimum: undefined,
      maximum: undefined,
    });
  });

  it('impose un minimum inférieur au maximum pour Number', () => {
    const rangeQuestion: EditorQuestion = {
      ...createBlankEditorQuestion('question-1'),
      label: 'Niveau de patience',
      type: 'integer_range',
      options: [],
      minimum: 10,
      maximum: 5,
    };
    expect(isEditorQuestionValid(rangeQuestion)).toBe(false);
    expect(isEditorQuestionValid({ ...rangeQuestion, minimum: 0, maximum: 10 })).toBe(true);
  });

  it('envoie une question catalogue comme référence', () => {
    expect(toGameConfigQuestionInput({
      key: 'system-romance',
      source: 'system',
      questionId: 'romance',
      label: 'Romantisme',
      type: 'integer_range',
      options: [],
      minimum: 0,
      maximum: 10,
    })).toEqual({ questionId: 'romance' });
  });

  it('réordonne les cartes et protège les limites', () => {
    const first = { ...createBlankEditorQuestion('first'), label: 'Première', options: ['A', 'B'] };
    const second = { ...createBlankEditorQuestion('second'), label: 'Deuxième', options: ['A', 'B'] };
    const questions = [first, second];
    expect(moveEditorQuestion(questions, 'second', -1)).toEqual([second, first]);
    expect(moveEditorQuestion(questions, 'first', -1)).toBe(questions);
  });
});
