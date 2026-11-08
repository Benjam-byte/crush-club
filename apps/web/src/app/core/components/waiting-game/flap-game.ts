import { crushClubTheme } from '../../theme/crush-club-theme';
import { drawHeart, fillBackground, fillRoundedRect, waitingGameHeight, waitingGameWidth } from './waiting-game-engine';
import type { RandomSource, WaitingGameEngine } from './waiting-game-engine';

export interface FlapColumn {
  x: number;
  gapTop: number;
  gapHeight: number;
  isPassed: boolean;
}

export const flapHeroX = 80;
export const flapHeroSize = 22;
export const flapGroundY = 224;
export const flapColumnWidth = 36;
export const flapGapMargin = 20;
export const flapMinimumGapHeight = 66;
const gravity = 0.0012;
const flapVelocity = -0.34;
const maxFallSpeed = 0.5;
const initialScrollSpeed = 0.1;
const scrollSpeedupPerPoint = 0.003;
const maxScrollSpeed = 0.16;
const initialGapHeight = 92;
const gapShrinkPerPoint = 1.5;
const columnSpacing = 150;
const firstColumnX = waitingGameWidth + 20;
// Shrinks the hero hitbox a little so grazing a cloud does not feel unfair.
const collisionTolerance = 3;
const hoverAmplitude = 4;
const hoverPeriodMilliseconds = 200;

export class FlapGame implements WaitingGameEngine {
  score = 0;
  isOver = false;
  heroCenterY = flapGroundY / 2;
  columnList: FlapColumn[] = [];
  /** The hero hovers in place until the first tap, so the player is not caught off guard on start. */
  hasTakenOff = false;
  private verticalVelocity = 0;
  private hoverElapsedMilliseconds = 0;

  constructor(private readonly random: RandomSource) {}

  update(elapsedMilliseconds: number): void {
    if (this.isOver) return;
    if (!this.hasTakenOff) {
      this.hoverElapsedMilliseconds += elapsedMilliseconds;
      this.heroCenterY = flapGroundY / 2 + Math.sin(this.hoverElapsedMilliseconds / hoverPeriodMilliseconds) * hoverAmplitude;
      return;
    }
    this.verticalVelocity = Math.min(maxFallSpeed, this.verticalVelocity + gravity * elapsedMilliseconds);
    this.heroCenterY += this.verticalVelocity * elapsedMilliseconds;
    this.moveColumns(elapsedMilliseconds);
    const isOutOfSky = this.heroTop() < 0 || this.heroBottom() > flapGroundY;
    this.isOver = isOutOfSky || this.columnList.some((column) => this.isHittingColumn(column));
  }

  handleTap(): void {
    if (this.isOver) return;
    this.hasTakenOff = true;
    this.verticalVelocity = flapVelocity;
  }

  draw(context: CanvasRenderingContext2D): void {
    fillBackground(context, crushClubTheme.background);
    this.columnList.forEach((column) => drawCloudColumn(context, column));
    context.fillStyle = crushClubTheme.peach;
    context.fillRect(0, flapGroundY, waitingGameWidth, waitingGameHeight - flapGroundY);
    context.fillStyle = crushClubTheme.muted;
    context.fillRect(0, flapGroundY, waitingGameWidth, 2);
    this.drawHero(context);
    if (this.hasTakenOff) return;
    context.fillStyle = crushClubTheme.muted;
    context.font = '700 12px "Plus Jakarta Sans", sans-serif';
    context.textAlign = 'center';
    context.fillText('Touche pour t’envoler', waitingGameWidth / 2, flapGroundY / 2 + 36);
  }

  private moveColumns(elapsedMilliseconds: number): void {
    const scrollSpeed = Math.min(maxScrollSpeed, initialScrollSpeed + this.score * scrollSpeedupPerPoint);
    this.columnList.forEach((column) => (column.x -= scrollSpeed * elapsedMilliseconds));
    const passedColumnList = this.columnList.filter((column) => !column.isPassed && column.x + flapColumnWidth < flapHeroX - flapHeroSize / 2);
    passedColumnList.forEach((column) => (column.isPassed = true));
    this.score += passedColumnList.length;
    this.columnList = this.columnList.filter((column) => column.x + flapColumnWidth > 0);
    const lastColumn = this.columnList.at(-1);
    const nextColumnX = lastColumn ? lastColumn.x + columnSpacing : firstColumnX;
    if (nextColumnX <= waitingGameWidth + flapColumnWidth) this.columnList.push(this.createColumn(nextColumnX));
  }

  private createColumn(x: number): FlapColumn {
    const gapHeight = Math.max(flapMinimumGapHeight, initialGapHeight - this.score * gapShrinkPerPoint);
    const gapTop = flapGapMargin + this.random() * (flapGroundY - 2 * flapGapMargin - gapHeight);
    return { x, gapTop, gapHeight, isPassed: false };
  }

  private isHittingColumn(column: FlapColumn): boolean {
    const heroLeft = flapHeroX - flapHeroSize / 2 + collisionTolerance;
    const heroRight = flapHeroX + flapHeroSize / 2 - collisionTolerance;
    const isHorizontallyAligned = heroRight > column.x && heroLeft < column.x + flapColumnWidth;
    const isOutsideGap = this.heroTop() + collisionTolerance < column.gapTop || this.heroBottom() - collisionTolerance > column.gapTop + column.gapHeight;
    return isHorizontallyAligned && isOutsideGap;
  }

  private heroTop(): number {
    return this.heroCenterY - flapHeroSize / 2;
  }

  private heroBottom(): number {
    return this.heroCenterY + flapHeroSize / 2;
  }

  private drawHero(context: CanvasRenderingContext2D): void {
    // Wings point up while climbing and fold down while falling.
    const wingAngle = this.verticalVelocity < 0 ? -0.6 : 0.3;
    context.fillStyle = crushClubTheme.lover;
    for (const wingSide of [-1, 1]) {
      context.beginPath();
      context.ellipse(flapHeroX + wingSide * flapHeroSize * 0.45, this.heroCenterY - 2, 8, 4, wingSide * -wingAngle, 0, Math.PI * 2);
      context.fill();
    }
    drawHeart(context, flapHeroX, this.heroCenterY, flapHeroSize, crushClubTheme.brand);
  }
}

function drawCloudColumn(context: CanvasRenderingContext2D, column: FlapColumn): void {
  const gapBottom = column.gapTop + column.gapHeight;
  const cloudRadius = 10;
  fillRoundedRect(context, column.x, -cloudRadius, flapColumnWidth, column.gapTop + cloudRadius, cloudRadius, crushClubTheme.peach);
  fillRoundedRect(context, column.x, gapBottom, flapColumnWidth, flapGroundY - gapBottom + cloudRadius, cloudRadius, crushClubTheme.peach);
  context.fillStyle = crushClubTheme.coral;
  context.fillRect(column.x + 4, column.gapTop - 4, flapColumnWidth - 8, 4);
  context.fillRect(column.x + 4, gapBottom, flapColumnWidth - 8, 4);
}
