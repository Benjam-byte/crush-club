import { describe, expect, it } from 'vitest';
import type { GameConfig } from '@core/models/game.models';
import { duplicateConfigQuestionInputs } from './game-config-duplication';

describe('duplicateConfigQuestionInputs', () => {
  it('duplique un modèle système avec des questions personnalisées complètes', () => {
    const config = {
      id: 'classic',
      name: 'Classique',
      kind: 'system',
      isPublic: true,
      isOwner: false,
      version: 1,
      questionIds: ['romance'],
      questions: [{
        id: 'romance',
        kind: 'system',
        label: 'Romantisme',
        type: 'integer_range',
        maximumScore: 10,
        loverEligible: true,
        minimum: 0,
        maximum: 10,
      }],
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    } satisfies GameConfig;

    expect(duplicateConfigQuestionInputs(config)).toEqual([{
      label: 'Romantisme',
      type: 'integer_range',
      options: undefined,
      minimum: 0,
      maximum: 10,
    }]);
  });
});
