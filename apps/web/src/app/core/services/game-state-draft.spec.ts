import { describe, expect, it } from 'vitest';
import { filterBioAnswers, restoredPrimaryPhotoId } from './game-state-draft';

describe('game state draft', () => {
  it('retire les anciennes réponses de bio absentes du questionnaire courant', () => {
    expect(filterBioAnswers([], { quality: 'funny' })).toEqual({});
    expect(filterBioAnswers([{
      id: 'quality',
      label: 'Qualité',
      options: [{ id: 'funny', label: 'Drôle' }],
    }], { quality: 'funny', hidden: 'stale' })).toEqual({ quality: 'funny' });
  });

  it('restaure uniquement une photo encore disponible', () => {
    expect(restoredPrimaryPhotoId('photo-1', {}, ['photo-1'])).toBe('photo-1');
    expect(restoredPrimaryPhotoId('photo-old', {}, ['photo-1'])).toBeNull();
  });
});
