import { primaryPhotoQuestionId } from '../models/game.models';
import type { AnswerValue } from '../models/game.models';

export function restoredPrimaryPhotoId(
  storedPrimaryPhotoId: string | null | undefined,
  storedAnswerByQuestionId: Readonly<Record<string, AnswerValue>>,
  subjectPhotoIds: readonly string[],
): string | null {
  const legacyPhotoId = storedAnswerByQuestionId[primaryPhotoQuestionId];
  const candidate = storedPrimaryPhotoId ?? (typeof legacyPhotoId === 'string' ? legacyPhotoId : null);
  return candidate && subjectPhotoIds.includes(candidate) ? candidate : null;
}
