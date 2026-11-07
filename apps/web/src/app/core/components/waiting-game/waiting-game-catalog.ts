import { FlapGame } from './flap-game';
import { PaddleGame } from './paddle-game';
import { RunnerGame } from './runner-game';
import { SnakeGame } from './snake-game';
import type { RandomSource, WaitingGameEngine, WaitingGameKind } from './waiting-game-engine';

export interface WaitingGameDefinition {
  readonly kind: WaitingGameKind;
  readonly title: string;
  readonly hint: string;
  readonly createEngine: (random: RandomSource) => WaitingGameEngine;
}

type BestScoreRecord = Partial<Record<WaitingGameKind, number>>;

const bestScoreStorageKey = 'crush-club-waiting-game-best-v1';

export const waitingGameDefinitionList: readonly WaitingGameDefinition[] = [
  {
    kind: 'runner',
    title: 'Course du cœur',
    hint: 'Touche l’écran pour sauter par-dessus les roses et les cœurs brisés.',
    createEngine: (random) => new RunnerGame(random),
  },
  {
    kind: 'snake',
    title: 'Serpentin',
    hint: 'Glisse le doigt pour tourner et croque un maximum de cœurs.',
    createEngine: (random) => new SnakeGame(random),
  },
  {
    kind: 'paddle',
    title: 'Ping Love',
    hint: 'Fais glisser ta raquette et renvoie le cœur. Trois vies.',
    createEngine: (random) => new PaddleGame(random),
  },
  {
    kind: 'flap',
    title: 'Envol de Cupidon',
    hint: 'Touche l’écran pour battre des ailes entre les nuages.',
    createEngine: (random) => new FlapGame(random),
  },
];

/** Never repeats the previous game, so two waiting screens in a row feel different. */
export function pickWaitingGameDefinition(random: RandomSource, previousKind: WaitingGameKind | null): WaitingGameDefinition {
  const candidateList = waitingGameDefinitionList.filter((definition) => definition.kind !== previousKind);
  return candidateList[Math.floor(random() * candidateList.length)];
}

export function readBestScore(kind: WaitingGameKind): number {
  return readBestScoreRecord()[kind] ?? 0;
}

export function saveBestScore(kind: WaitingGameKind, score: number): void {
  const bestScoreRecord = readBestScoreRecord();
  if (score <= (bestScoreRecord[kind] ?? 0)) return;
  try {
    localStorage.setItem(bestScoreStorageKey, JSON.stringify({ ...bestScoreRecord, [kind]: score }));
  } catch {
    // Storage blocked (private browsing): the record only lasts for this screen.
  }
}

function readBestScoreRecord(): BestScoreRecord {
  let storedValue: string | null = null;
  try {
    storedValue = localStorage.getItem(bestScoreStorageKey);
  } catch {
    // Storage blocked (private browsing): start without a record.
  }
  if (!storedValue) return {};
  try {
    const parsedValue: unknown = JSON.parse(storedValue);
    return isBestScoreRecord(parsedValue) ? parsedValue : {};
  } catch (error: unknown) {
    console.error('Unable to read the waiting game best scores', error);
    return {};
  }
}

function isBestScoreRecord(parsedValue: unknown): parsedValue is BestScoreRecord {
  if (typeof parsedValue !== 'object' || parsedValue === null) return false;
  return Object.values(parsedValue).every((score) => typeof score === 'number');
}
