export type WaitingGameKind = 'runner' | 'snake' | 'paddle' | 'flap';
export type WaitingGameDirection = 'up' | 'down' | 'left' | 'right';
export type RandomSource = () => number;

/** Engines work in this logical resolution; the component scales it to the real canvas size. */
export const waitingGameWidth = 320;
export const waitingGameHeight = 240;
export const swipeThresholdPixels = 24;

export interface WaitingGameEngine {
  readonly score: number;
  readonly isOver: boolean;
  update(elapsedMilliseconds: number): void;
  draw(context: CanvasRenderingContext2D): void;
  handleTap?(): void;
  handleDirection?(direction: WaitingGameDirection): void;
  /** Pointer position as a 0–1 ratio of the canvas, so engines never deal with CSS pixels. */
  handlePointerMove?(ratioX: number, ratioY: number): void;
}

/** Returns the dominant direction once the finger has travelled far enough, otherwise null. */
export function findSwipeDirection(deltaX: number, deltaY: number): WaitingGameDirection | null {
  if (Math.max(Math.abs(deltaX), Math.abs(deltaY)) < swipeThresholdPixels) return null;
  if (Math.abs(deltaX) > Math.abs(deltaY)) return deltaX > 0 ? 'right' : 'left';
  return deltaY > 0 ? 'down' : 'up';
}

export function drawHeart(context: CanvasRenderingContext2D, centerX: number, centerY: number, size: number, color: string): void {
  const left = centerX - size / 2;
  const top = centerY - size / 2;
  context.fillStyle = color;
  context.beginPath();
  context.moveTo(centerX, top + size * 0.3);
  context.bezierCurveTo(centerX, top, left, top, left, top + size * 0.3);
  context.bezierCurveTo(left, top + size * 0.6, centerX, top + size * 0.8, centerX, top + size);
  context.bezierCurveTo(centerX, top + size * 0.8, left + size, top + size * 0.6, left + size, top + size * 0.3);
  context.bezierCurveTo(left + size, top, centerX, top, centerX, top + size * 0.3);
  context.fill();
}

// Drawn by hand because CanvasRenderingContext2D.roundRect is missing on iOS 15.
export function fillRoundedRect(
  context: CanvasRenderingContext2D,
  left: number,
  top: number,
  width: number,
  height: number,
  radius: number,
  color: string,
): void {
  const cornerRadius = Math.min(radius, width / 2, height / 2);
  context.fillStyle = color;
  context.beginPath();
  context.moveTo(left + cornerRadius, top);
  context.arcTo(left + width, top, left + width, top + height, cornerRadius);
  context.arcTo(left + width, top + height, left, top + height, cornerRadius);
  context.arcTo(left, top + height, left, top, cornerRadius);
  context.arcTo(left, top, left + width, top, cornerRadius);
  context.closePath();
  context.fill();
}

export function fillBackground(context: CanvasRenderingContext2D, color: string): void {
  context.fillStyle = color;
  context.fillRect(0, 0, waitingGameWidth, waitingGameHeight);
}
