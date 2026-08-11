import { primaryPhotoQuestionId } from '../models/game.models';
import type { AnswerValue, ProfileFieldDefinition } from '../models/game.models';

export function filterBioAnswers(
  profileFields: readonly ProfileFieldDefinition[],
  answers: Readonly<Record<string, string>>,
): Record<string, string> {
  const activeFieldIds = new Set(profileFields.map((field) => field.id));
  return Object.fromEntries(
    Object.entries(answers).filter(([fieldId]) => activeFieldIds.has(fieldId)),
  );
}

export function restoredPrimaryPhotoId(
  storedPrimaryPhotoId: string | null | undefined,
  storedAnswerByQuestionId: Readonly<Record<string, AnswerValue>>,
  subjectPhotoIds: readonly string[],
): string | null {
  const legacyPhotoId = storedAnswerByQuestionId[primaryPhotoQuestionId];
  const candidate = storedPrimaryPhotoId ?? (typeof legacyPhotoId === 'string' ? legacyPhotoId : null);
  return candidate && subjectPhotoIds.includes(candidate) ? candidate : null;
}
