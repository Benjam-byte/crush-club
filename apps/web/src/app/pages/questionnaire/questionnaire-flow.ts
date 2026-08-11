import type { PlayerRole } from '@core/models/game.models';

export function questionnaireStepCount(
  role: PlayerRole,
  profileFieldCount: number,
  questionCount: number,
): number {
  const taglineStepCount = role === 'lover' ? 0 : 1;
  return 1 + taglineStepCount + profileFieldCount + questionCount;
}
