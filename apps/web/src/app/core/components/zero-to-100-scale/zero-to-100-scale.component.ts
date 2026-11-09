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

/** One nominee placed on a 0 à 100 scale. */
export interface ScalePoint {
  id: string
  label: string
  value: number
  /** Seat index of the nominee: it picks the dot color, so it must match across scales. */
  colorIndex: number
}

export interface ScaleValueChange {
  id: string
  value: number
}

/**
 * Nominee colors, picked by seat so a nominee keeps the same color on every scale.
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

/** Evenly spread starting positions, e.g. 10/30/50/70/90 for five nominees. */
export function spreadPositions(count: number): number[] {
  return Array.from({ length: count }, (_, index) => Math.round(((index + 0.5) * 100) / count));
}

export function clampScaleValue(value: number): number {
  return Math.min(100, Math.max(0, Math.round(value)));
}

/** Order of each value along the scale, 1 for the closest to 0; equal values share a rank. */
export function scaleRanks(values: readonly number[]): number[] {
  return values.map((value) => values.filter((other) => other < value).length + 1);
}

/** French ordinal shown after a name: 1er, 2e, 3e… */
export function rankLabel(rank: number): string {
  return rank === 1 ? '1er' : `${rank}e`;
}

/** Horizontal extent of a label along the track, in pixels. */
export interface LabelSpan {
  start: number
  end: number
}

/**
 * Where a label of labelWidth pixels lands on a track of trackWidth pixels. Labels
 * are anchored with `left: v%` and `translateX(-v%)`, so they never overflow the
 * track: their left edge sits at v% of (trackWidth - labelWidth).
 */
export function labelSpan(value: number, labelWidth: number, trackWidth: number): LabelSpan {
  const start = (value / 100) * Math.max(0, trackWidth - labelWidth);
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

// Keep in sync with the .scale__name, .scale__value and .scale__end styles.
const NAME_FONT = '800 11px "Plus Jakarta Sans", system-ui, sans-serif';
const RANK_FONT = '700 10px "Plus Jakarta Sans", system-ui, sans-serif';
const NAME_MAX_WIDTH = 120;
const NAME_CHROME_WIDTH = 30; // padding, border, swatch and the gaps around the name and rank
const NAME_HEIGHT = 20;
const NAME_LANE_HEIGHT = 24;
const VALUE_FONT = '800 12px "Plus Jakarta Sans", system-ui, sans-serif';
const END_FONT = '700 10px "Plus Jakarta Sans", system-ui, sans-serif';
const VALUE_LANE_HEIGHT = 16;
// Two value lanes cover the usual case of a value next to another one or to 0 / 100.
const RESERVED_VALUE_LANES = 2;
const STEM_GAP = 6;
const TRACK_ROW_HEIGHT = 32;
const DEFAULT_TRACK_WIDTH = 300;

const KEY_MOVES: Partial<Record<string, (value: number) => number>> = {
  ArrowLeft: (value) => value - 1,
  ArrowDown: (value) => value - 1,
  ArrowRight: (value) => value + 1,
  ArrowUp: (value) => value + 1,
  PageDown: (value) => value - 10,
  PageUp: (value) => value + 10,
  Home: () => 0,
  End: () => 100,
};

/**
 * A single 0 à 100 track holding one colored dot per nominee, with the name above
 * and the value below each dot, so the order of the nominees reads at a glance.
 * Read-only by default; `editable` lets the player drag the dots.
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
  /** Places each value beside its name so two result scales fit on a small screen. */
  readonly compact = input(false);
  /** Shows a single point, by id; null shows them all. */
  readonly focusId = input<string | null>(null);
  /**
   * Keeps room for this many names stacked on their own lane (plus two value lanes),
   * so scales keep one height while names re-stack or a single point is focused.
   */
  readonly reservedNames = input(0);
  readonly valueChange = output<ScaleValueChange>();

  protected readonly activeId = signal<string | null>(null);
  private readonly trackWidth = signal(DEFAULT_TRACK_WIDTH);
  private readonly fontsLoaded = signal(false);
  private readonly rail = viewChild.required<ElementRef<HTMLElement>>('rail');
  private readonly measuringContext = document.createElement('canvas').getContext('2d');

  private drag: { pointerId: number; pointId: string; grabOffset: number } | null = null;
  private pointerDownOnPoint = false;

  /** The drawn points: all of them, or only the focused one, still ranked among all. */
  private readonly shownPoints = computed(() => {
    const focusId = this.focusId();
    // Ranked from the displayed (rounded) values, so ties match the numbers on screen.
    const rankLabels = scaleRanks(this.points().map((point) => Math.round(point.value))).map(rankLabel);
    return this.points()
      .map((point, index) => ({ ...point, rankLabel: rankLabels[index] }))
      .filter((point) => focusId === null || point.id === focusId);
  });

  protected readonly summary = computed(() => {
    const positions = this.shownPoints().map((point) => `${point.label} ${Math.round(point.value)} (${point.rankLabel})`);
    return `${this.ariaLabel()} : ${positions.join(', ')}`;
  });

  protected readonly layout = computed(() => {
    const points = this.shownPoints();
    const trackWidth = this.trackWidth();
    // Label widths change once the web font replaces the fallback one.
    this.fontsLoaded();
    const nameLanes = assignLabelLanes(
      points.map((point) => {
        const valueWidth = this.compact() ? this.textWidth(` · ${Math.round(point.value)}`, NAME_FONT) : 0;
        const textWidth = this.textWidth(point.label, NAME_FONT) + this.textWidth(point.rankLabel, RANK_FONT) + valueWidth;
        return labelSpan(point.value, Math.min(this.compact() ? 168 : NAME_MAX_WIDTH, textWidth + NAME_CHROME_WIDTH), trackWidth);
      }),
    );
    // The 0 and 100 end labels sit on the first value lane, so values move out of their way.
    const endSpans = [labelSpan(0, this.textWidth('0', END_FONT), trackWidth), labelSpan(100, this.textWidth('100', END_FONT), trackWidth)];
    const valueLanesOf = (scalePoints: readonly ScalePoint[]) => assignLabelLanes(
      scalePoints.map((point) => labelSpan(point.value, this.textWidth(`${Math.round(point.value)}`, VALUE_FONT), trackWidth)),
      endSpans,
    );
    const valueLanes = valueLanesOf(points);
    // Sized for every point, hidden ones included, so focusing one keeps the scale's height.
    const allValueLanes = this.focusId() === null ? valueLanes : valueLanesOf(this.points());
    const reservedNames = this.reservedNames();
    const namesHeight = Math.max(1, reservedNames, ...nameLanes.map((lane) => lane + 1)) * NAME_LANE_HEIGHT;
    const valueLaneCount = Math.max(reservedNames > 0 ? RESERVED_VALUE_LANES : 1, ...allValueLanes.map((lane) => lane + 1));
    const valuesTop = namesHeight + STEM_GAP + TRACK_ROW_HEIGHT;
    return {
      height: valuesTop + (this.compact() ? 1 : valueLaneCount) * VALUE_LANE_HEIGHT,
      trackCenter: namesHeight + STEM_GAP + TRACK_ROW_HEIGHT / 2,
      valuesTop,
      points: points.map((point, index) => {
        const nameTop = namesHeight - (nameLanes[index] + 1) * NAME_LANE_HEIGHT;
        return {
          ...point,
          displayValue: Math.round(point.value),
          color: nomineeColor(point.colorIndex),
          textColor: nomineeTextColor(point.colorIndex),
          labelShift: `translateX(-${point.value}%)`,
          nameTop,
          stemTop: nameTop + NAME_HEIGHT,
          valueTop: valuesTop + valueLanes[index] * VALUE_LANE_HEIGHT,
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
    const pointId = nearestPointId(this.shownPoints(), value, this.activeId());
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

  /** Scale value under clientX, left unclamped so a drag keeps its grab offset. */
  private pointerValue(clientX: number): number {
    const rect = this.rail().nativeElement.getBoundingClientRect();
    return rect.width > 0 ? ((clientX - rect.left) / rect.width) * 100 : 0;
  }

  private textWidth(text: string, font: string): number {
    if (this.measuringContext === null) {
      return text.length * 8;
    }
    this.measuringContext.font = font;
    return Math.ceil(this.measuringContext.measureText(text).width);
  }
}
