import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import type { ElementRef } from '@angular/core';

/** Top of the 0 à 10 scale; positions are whole numbers from 0 to this value. */
export const SCALE_MAX = 10;

/** One nominee placed on the 0 à 10 scale. */
export interface ScalePoint {
  id: string
  label: string
  value: number
  /** Seat index of the nominee: it picks the dot color, so it must match across screens. */
  colorIndex: number
}

export interface ScaleValueChange {
  id: string
  value: number
}

/**
 * Nominee colors, picked by seat so a nominee keeps the same color on every screen.
 * Validated as a set (every pair, normal and color-blind vision) against the track
 * surface. Names are always displayed next to the dots, so color is never the only cue.
 */
export const NOMINEE_COLORS = ['#2a78d6', '#e63e5b', '#eda100', '#4a3aa7', '#008300'] as const;

/** Same hues darkened where needed so small text stays readable (≥ 4.8:1 on the page background). */
export const NOMINEE_TEXT_COLORS = ['#276ec5', '#c8364f', '#956500', '#4a3aa7', '#008000'] as const;

export function nomineeColor(colorIndex: number): string {
  return NOMINEE_COLORS[colorIndex % NOMINEE_COLORS.length];
}

export function nomineeTextColor(colorIndex: number): string {
  return NOMINEE_TEXT_COLORS[colorIndex % NOMINEE_TEXT_COLORS.length];
}

/** Where a scale value sits along the track, in percent. */
export function scalePercent(value: number): number {
  return (value * 100) / SCALE_MAX;
}

/** Evenly spread starting positions, e.g. 1/3/5/7/9 for five nominees. */
export function spreadPositions(count: number): number[] {
  return Array.from({ length: count }, (_, index) => Math.round(((index + 0.5) * SCALE_MAX) / count));
}

/** Snaps a value to the nearest notch of the scale. */
export function clampScaleValue(value: number): number {
  return Math.min(SCALE_MAX, Math.max(0, Math.round(value)));
}

/** Podium rank of each value, 1 for the highest; equal values share a rank. */
export function scaleRanks(values: readonly number[]): number[] {
  return values.map((value) => values.filter((other) => other > value).length + 1);
}

/** French ordinal shown after a name: 1er, 2e, 3e… */
export function rankLabel(rank: number): string {
  return rank === 1 ? '1er' : `${rank}e`;
}

/** Horizontal gap between the dots of nominees placed on the same notch. */
export const TIE_SPREAD_PX = 12;

/**
 * Horizontal shift of each dot, in pixels, so nominees on the same notch fan out
 * around it instead of hiding each other. Untied values stay at 0.
 */
export function tieOffsets(values: readonly number[], spread = TIE_SPREAD_PX): number[] {
  return values.map((value, index) => {
    const tiedCount = values.filter((other) => other === value).length;
    const tieIndex = values.slice(0, index).filter((other) => other === value).length;
    return (tieIndex - (tiedCount - 1) / 2) * spread;
  });
}

/** Horizontal extent of a label along the track, in pixels. */
export interface LabelSpan {
  start: number
  end: number
}

/**
 * Where a label of labelWidth pixels lands on a track of trackWidth pixels. Labels
 * are anchored with `left: p%` and `translateX(-p%)`, so they never overflow the
 * track: their left edge sits at p% of (trackWidth - labelWidth).
 */
export function labelSpan(value: number, labelWidth: number, trackWidth: number): LabelSpan {
  const start = (value / SCALE_MAX) * Math.max(0, trackWidth - labelWidth);
  return { start, end: start + labelWidth };
}

/**
 * Stacks labels on lanes so overlapping labels never share one; lane 0 sits next to
 * the track and starts with the reserved spans. Returns each label's lane, in input order.
 */
export function assignLabelLanes(spans: readonly LabelSpan[], reserved: readonly LabelSpan[] = [], spacing = 4): number[] {
  const lanes: LabelSpan[][] = [[...reserved]];
  const laneByIndex = spans.map(() => 0);
  const byStart = spans.map((span, index) => ({ span, index })).sort((left, right) => left.span.start - right.span.start);
  for (const { span, index } of byStart) {
    const isFree = (occupied: LabelSpan[]) =>
      occupied.every((other) => span.start >= other.end + spacing || other.start >= span.end + spacing);
    let lane = lanes.findIndex(isFree);
    if (lane === -1) {
      lane = lanes.push([]) - 1;
    }
    lanes[lane].push(span);
    laneByIndex[index] = lane;
  }
  return laneByIndex;
}

/** The point closest to value. Ties go to preferredId, then to the point drawn on top (the last one). */
export function nearestPointId(points: readonly ScalePoint[], value: number, preferredId: string | null): string | null {
  let nearestId: string | null = null;
  let nearestDistance = Number.POSITIVE_INFINITY;
  for (const point of points) {
    const distance = Math.abs(point.value - value);
    if (distance < nearestDistance || (distance === nearestDistance && nearestId !== preferredId)) {
      nearestId = point.id;
      nearestDistance = distance;
    }
  }
  return nearestId;
}

// Keep in sync with the .scale__name styles.
const NAME_FONT = '800 11px "Plus Jakarta Sans", system-ui, sans-serif';
const RANK_FONT = '700 10px "Plus Jakarta Sans", system-ui, sans-serif';
const NAME_MAX_WIDTH = 120;
const NAME_CHROME_WIDTH = 30; // padding, border, swatch and the gaps around the name and rank
const NAME_HEIGHT = 20;
const NAME_LANE_HEIGHT = 24;
const RULER_HEIGHT = 18;
const STEM_GAP = 6;
const TRACK_ROW_HEIGHT = 32;
const DEFAULT_TRACK_WIDTH = 300;

const KEY_MOVES: Partial<Record<string, (value: number) => number>> = {
  ArrowLeft: (value) => value - 1,
  ArrowDown: (value) => value - 1,
  ArrowRight: (value) => value + 1,
  ArrowUp: (value) => value + 1,
  PageDown: (value) => value - 2,
  PageUp: (value) => value + 2,
  Home: () => 0,
  End: () => SCALE_MAX,
};

/**
 * A single 0 à 10 track holding one colored dot per nominee, with the name and podium
 * rank above each dot and a 0…10 ruler below, so the order reads at a glance.
 * Read-only by default; `editable` lets the player drag the dots, which snap to the notches.
 */
@Component({
  selector: 'app-zero-to-100-scale',
  templateUrl: './zero-to-100-scale.component.html',
  styleUrl: './zero-to-100-scale.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ZeroToHundredScaleComponent {
  readonly points = input.required<readonly ScalePoint[]>();
  readonly ariaLabel = input.required<string>();
  readonly editable = input(false);
  /** Keeps room for this many names stacked on their own lane, so the scale keeps one height while names re-stack. */
  readonly reservedNames = input(0);
  readonly valueChange = output<ScaleValueChange>();

  protected readonly activeId = signal<string | null>(null);
  protected readonly ruler = Array.from({ length: SCALE_MAX + 1 }, (_, value) => ({ value, percent: scalePercent(value) }));
  protected readonly scaleMax = SCALE_MAX;
  private readonly trackWidth = signal(DEFAULT_TRACK_WIDTH);
  private readonly fontsLoaded = signal(false);
  private readonly rail = viewChild.required<ElementRef<HTMLElement>>('rail');
  private readonly measuringContext = document.createElement('canvas').getContext('2d');

  private drag: { pointerId: number; pointId: string; grabOffset: number } | null = null;
  private pointerDownOnPoint = false;

  /** The points with their podium rank and tie offset, ranked from the displayed (snapped) values. */
  private readonly rankedPoints = computed(() => {
    const values = this.points().map((point) => clampScaleValue(point.value));
    const rankLabels = scaleRanks(values).map(rankLabel);
    const offsets = tieOffsets(values);
    return this.points().map((point, index) => ({
      ...point,
      displayValue: values[index],
      rankLabel: rankLabels[index],
      dotOffset: offsets[index],
    }));
  });

  /** Notches holding at least one dot, highlighted on the ruler. */
  protected readonly occupiedValues = computed(() => new Set(this.rankedPoints().map((point) => point.displayValue)));

  protected readonly summary = computed(() => {
    const positions = this.rankedPoints().map((point) => `${point.label} ${point.displayValue} (${point.rankLabel})`);
    return `${this.ariaLabel()} : ${positions.join(', ')}`;
  });

  protected readonly layout = computed(() => {
    const points = this.rankedPoints();
    const trackWidth = this.trackWidth();
    // Label widths change once the web font replaces the fallback one.
    this.fontsLoaded();
    const nameLanes = assignLabelLanes(
      points.map((point) => {
        const textWidth = this.textWidth(point.label, NAME_FONT) + this.textWidth(point.rankLabel, RANK_FONT);
        return labelSpan(point.displayValue, Math.min(NAME_MAX_WIDTH, textWidth + NAME_CHROME_WIDTH), trackWidth);
      }),
    );
    const namesHeight = Math.max(1, this.reservedNames(), ...nameLanes.map((lane) => lane + 1)) * NAME_LANE_HEIGHT;
    const trackCenter = namesHeight + STEM_GAP + TRACK_ROW_HEIGHT / 2;
    const rulerTop = namesHeight + STEM_GAP + TRACK_ROW_HEIGHT;
    return {
      height: rulerTop + RULER_HEIGHT,
      trackCenter,
      rulerTop,
      points: points.map((point, index) => {
        const percent = scalePercent(point.displayValue);
        const nameTop = namesHeight - (nameLanes[index] + 1) * NAME_LANE_HEIGHT;
        return {
          ...point,
          percent,
          color: nomineeColor(point.colorIndex),
          labelShift: `translateX(-${percent}%)`,
          nameTop,
          stemTop: nameTop + NAME_HEIGHT,
        };
      }),
    };
  });

  constructor() {
    const destroyRef = inject(DestroyRef);
    afterNextRender(() => {
      const rail = this.rail().nativeElement;
      const measure = () => {
        if (rail.clientWidth > 0) {
          this.trackWidth.set(rail.clientWidth);
        }
      };
      measure();
      const observer = new ResizeObserver(measure);
      observer.observe(rail);
      destroyRef.onDestroy(() => observer.disconnect());
      void document.fonts.ready.then(() => this.fontsLoaded.set(true));
    });
  }

  protected onPointerDown(event: PointerEvent): void {
    const point = this.pointAt(event.target);
    this.pointerDownOnPoint = point !== undefined;
    if (!this.editable() || point === undefined || !event.isPrimary || event.button !== 0) {
      return;
    }
    // Keeps focus on the dot (the captured rail would otherwise take the mousedown) and blocks text selection.
    event.preventDefault();
    const rail = this.rail().nativeElement;
    this.drag = {
      pointerId: event.pointerId,
      pointId: point.id,
      grabOffset: this.pointerValue(event.clientX) - point.value,
    };
    this.activeId.set(point.id);
    rail.setPointerCapture(event.pointerId);
    if (event.pointerType === 'mouse') {
      // Lets the arrow keys fine-tune the grabbed dot; touch has no keyboard to follow up.
      rail.querySelector<HTMLElement>(`.scale__dot[data-point-id="${CSS.escape(point.id)}"]`)?.focus({ preventScroll: true });
    }
  }

  protected onPointerMove(event: PointerEvent): void {
    if (this.drag?.pointerId !== event.pointerId) {
      return;
    }
    this.emitValue(this.drag.pointId, this.pointerValue(event.clientX) - this.drag.grabOffset);
  }

  protected onPointerEnd(event: PointerEvent): void {
    if (this.drag?.pointerId === event.pointerId) {
      this.drag = null;
    }
  }

  /** A tap on the bare track brings the nearest dot there; taps on a dot are drags. */
  protected onTrackClick(event: MouseEvent): void {
    if (!this.editable() || this.pointerDownOnPoint) {
      return;
    }
    const value = clampScaleValue(this.pointerValue(event.clientX));
    const pointId = nearestPointId(this.points(), value, this.activeId());
    if (pointId !== null) {
      this.activeId.set(pointId);
      this.emitValue(pointId, value);
    }
  }

  protected onKeydown(event: KeyboardEvent, point: ScalePoint): void {
    const move = KEY_MOVES[event.key];
    if (!this.editable() || move === undefined) {
      return;
    }
    event.preventDefault();
    this.activeId.set(point.id);
    this.emitValue(point.id, move(point.value));
  }

  private emitValue(pointId: string, rawValue: number): void {
    const value = clampScaleValue(rawValue);
    const point = this.points().find((candidate) => candidate.id === pointId);
    if (point !== undefined && point.value !== value) {
      this.valueChange.emit({ id: pointId, value });
    }
  }

  private pointAt(target: EventTarget | null): ScalePoint | undefined {
    const pointId = target instanceof Element ? target.closest<HTMLElement>('[data-point-id]')?.dataset['pointId'] : undefined;
    return pointId === undefined ? undefined : this.points().find((point) => point.id === pointId);
  }

  /** Scale value under clientX, left unrounded and unclamped so a drag keeps its grab offset. */
  private pointerValue(clientX: number): number {
    const rect = this.rail().nativeElement.getBoundingClientRect();
    return rect.width > 0 ? ((clientX - rect.left) / rect.width) * SCALE_MAX : 0;
  }

  private textWidth(text: string, font: string): number {
    if (this.measuringContext === null) {
      return text.length * 8;
    }
    this.measuringContext.font = font;
    return Math.ceil(this.measuringContext.measureText(text).width);
  }
}
