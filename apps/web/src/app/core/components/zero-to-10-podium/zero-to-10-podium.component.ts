import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
// Relative imports: Vitest does not resolve the @core alias.
import { nomineeColor, nomineeTextColor, rankLabel } from '../zero-to-100-scale/zero-to-100-scale.component';
import { formatNote } from '../../services/zero-to-10-results';
import type { PodiumNominee } from '../../services/zero-to-10-results';

/** Pause between two steps of the podium reveal, last place first. */
export const PODIUM_REVEAL_STEP_MS = 550;
/** Length of one step's rise animation. */
export const PODIUM_RISE_MS = 420;

/** Step heights by rank, in pixels: the 1st place stands the tallest. */
const STEP_HEIGHTS = [46, 38, 31, 26, 22];

/**
 * Visual slot of each place, from the 1st down: the 1st in the middle, then
 * alternating left and right (2-1-3, 4-2-1-3-5…).
 */
export function podiumSlots(count: number): number[] {
  const center = Math.floor(count / 2);
  const slots = count > 0 ? [center] : [];
  let left = center - 1;
  let right = center + 1;
  for (let index = 1; index < count; index++) {
    slots.push(index % 2 === 1 && left >= 0 ? left-- : right < count ? right++ : left--);
  }
  return slots;
}

/** Reveal order of each rank, 0 for the lowest step: tied nominees rise together. */
export function revealSteps(ranks: readonly number[]): number[] {
  const distinct = [...new Set(ranks)].sort((left, right) => right - left);
  return ranks.map((rank) => distinct.indexOf(rank));
}

/** How long the whole podium takes to show, so what follows can wait for the 1st place. */
export function podiumRevealDurationMs(ranks: readonly number[]): number {
  const stepCount = new Set(ranks).size;
  return stepCount === 0 ? 0 : (stepCount - 1) * PODIUM_REVEAL_STEP_MS + PODIUM_RISE_MS;
}

function initials(name: string): string {
  return name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
}

/**
 * The round's podium: each nominee that has a target stands on a step in their seat
 * color, the 1st in the middle. Steps rise one after the other, from the last place up.
 */
@Component({
  selector: 'app-zero-to-10-podium',
  templateUrl: './zero-to-10-podium.component.html',
  styleUrl: './zero-to-10-podium.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ZeroToTenPodiumComponent {
  /** Sorted from the 1st place down. */
  readonly nominees = input.required<readonly PodiumNominee[]>();

  protected readonly columns = computed(() => {
    const nominees = this.nominees();
    const slots = podiumSlots(nominees.length);
    const steps = revealSteps(nominees.map((nominee) => nominee.rank));
    return nominees.map((nominee, index) => ({
      ...nominee,
      initials: initials(nominee.name),
      rankLabel: rankLabel(nominee.rank),
      noteLabel: formatNote(nominee.target),
      color: nomineeColor(nominee.colorIndex),
      textColor: nomineeTextColor(nominee.colorIndex),
      stepHeight: STEP_HEIGHTS[Math.min(nominee.rank, STEP_HEIGHTS.length) - 1],
      slot: slots[index],
      revealDelay: `${steps[index] * PODIUM_REVEAL_STEP_MS}ms`,
    }));
  });

  protected readonly summary = computed(
    () => `Podium du groupe : ${this.columns().map((column) => `${column.rankLabel} ${column.name}, ${column.noteLabel}`).join(' ; ')}`,
  );
}
