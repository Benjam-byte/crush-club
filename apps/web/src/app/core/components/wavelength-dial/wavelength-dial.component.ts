import { ChangeDetectionStrategy, Component, computed, input, model, signal } from '@angular/core';
import {
  clampPosition,
  dialBands,
  dialCenterX,
  dialCenterY,
  dialHeight,
  dialRadius,
  dialWedgePath,
  dialWidth,
  pointOnDial,
  positionFromDialPoint,
  wavelengthScoringBands,
} from './wavelength-dial-geometry';

export interface WavelengthDialNeedle {
  id: string
  position: number
  /** Short text drawn on the needle tip, e.g. the player's initials. */
  label: string
  isCurrentPlayer?: boolean
}

const keyboardSteps: Readonly<Record<string, number>> = {
  ArrowLeft: -1,
  ArrowDown: -1,
  ArrowRight: 1,
  ArrowUp: 1,
  PageDown: -10,
  PageUp: 10,
};

/** When the shutter animation (250ms delay + 900ms, see the SCSS) is done. */
const shutterOpenedAtMs = 1150;
const needleRevealStepMs = 140;

let nextDialId = 0;

/**
 * Half-disc dial of a Wavelength round: the two poles, the secret target
 * bands, the player's own draggable needle and, at reveal, everyone's needles.
 */
@Component({
  selector: 'app-wavelength-dial',
  templateUrl: './wavelength-dial.component.html',
  styleUrl: './wavelength-dial.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WavelengthDialComponent {
  readonly leftLabel = input.required<string>();
  readonly rightLabel = input.required<string>();
  /** Secret target; while undefined a shutter covers the dial. */
  readonly target = input<number>();
  /** Opens the shutter over the target, then shows the needles one after another. */
  readonly animateReveal = input(false);
  readonly interactive = input(false);
  /** The player's own needle, hidden while null. */
  readonly value = model<number | null>(null);
  readonly needles = input<readonly WavelengthDialNeedle[]>([]);

  private readonly dialId = nextDialId++;
  private readonly isDragging = signal(false);

  protected readonly viewBox = `0 0 ${dialWidth} ${dialHeight}`;
  protected readonly clipId = `wavelength-dial-clip-${this.dialId}`;
  protected readonly shutterGradientId = `wavelength-dial-shutter-${this.dialId}`;
  protected readonly facePath = dialWedgePath(0, 100);
  protected readonly pivot = { x: dialCenterX, y: dialCenterY };
  protected readonly ticks = Array.from({ length: 11 }, (_, index) => {
    const outer = pointOnDial(index * 10, dialRadius);
    const inner = pointOnDial(index * 10, dialRadius - (index % 5 === 0 ? 7 : 4));
    return { x1: outer.x, y1: outer.y, x2: inner.x, y2: inner.y };
  });

  protected readonly showShutter = computed(() => this.target() === undefined || this.animateReveal());
  protected readonly bands = computed(() => {
    const target = this.target();
    if (target === undefined) {
      return [];
    }
    return dialBands(target).map((band) => ({ points: band.points, path: dialWedgePath(band.from, band.to) }));
  });
  /** One label per visible band segment: the bullseye once, the side bands on both sides. */
  protected readonly bandLabels = computed(() => {
    const target = this.target();
    if (target === undefined) {
      return [];
    }
    return wavelengthScoringBands.flatMap(({ radius, points }, index) => {
      if (index === 0) {
        return [{ points, ...pointOnDial(target, dialRadius - 12) }];
      }
      const offset = (wavelengthScoringBands[index - 1].radius + radius) / 2 + 0.5;
      const positions = [target - offset, target + offset];
      return positions
        .filter((position) => position >= 1 && position <= 99)
        .map((position) => ({ points, ...pointOnDial(position, dialRadius - 12) }));
    });
  });
  protected readonly needleLines = computed(() =>
    this.needles().map((needle, index) => ({
      ...needle,
      tip: pointOnDial(needle.position, dialRadius - 16),
      delayMs: this.animateReveal() ? shutterOpenedAtMs + index * needleRevealStepMs : 0,
    })),
  );
  protected readonly ownNeedle = computed(() => {
    const value = this.value();
    if (value === null) {
      return null;
    }
    return { tip: pointOnDial(value, dialRadius - 10), shaftEnd: pointOnDial(value, dialRadius - 17) };
  });
  protected readonly ariaLabel = computed(() => {
    const poles = `entre « ${this.leftLabel()} » et « ${this.rightLabel()} »`;
    return this.interactive() ? `Ton aiguille, ${poles}` : `Cadran ${poles}`;
  });

  protected onPointerDown(event: PointerEvent): void {
    if (!this.interactive()) {
      return;
    }
    event.preventDefault();
    const svg = event.currentTarget as SVGSVGElement;
    svg.setPointerCapture(event.pointerId);
    this.isDragging.set(true);
    this.moveNeedleTo(event, svg);
  }

  protected onPointerMove(event: PointerEvent): void {
    if (this.isDragging()) {
      this.moveNeedleTo(event, event.currentTarget as SVGSVGElement);
    }
  }

  protected onPointerUp(): void {
    this.isDragging.set(false);
  }

  protected onKeydown(event: KeyboardEvent): void {
    if (!this.interactive()) {
      return;
    }
    const current = this.value() ?? 50;
    let next: number | null = null;
    if (event.key === 'Home') {
      next = 0;
    } else if (event.key === 'End') {
      next = 100;
    } else if (keyboardSteps[event.key] !== undefined) {
      next = clampPosition(current + keyboardSteps[event.key]);
    }
    if (next !== null) {
      event.preventDefault();
      this.value.set(next);
    }
  }

  private moveNeedleTo(event: PointerEvent, svg: SVGSVGElement): void {
    const bounds = svg.getBoundingClientRect();
    const x = ((event.clientX - bounds.left) / bounds.width) * dialWidth;
    const y = ((event.clientY - bounds.top) / bounds.height) * dialHeight;
    this.value.set(positionFromDialPoint(x, y));
  }
}
