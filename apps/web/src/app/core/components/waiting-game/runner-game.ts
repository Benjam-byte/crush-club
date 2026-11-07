import { crushClubTheme } from '../../theme/crush-club-theme';
import { drawHeart, fillBackground, waitingGameHeight, waitingGameWidth } from './waiting-game-engine';
import type { RandomSource, WaitingGameEngine } from './waiting-game-engine';

export interface RunnerObstacle {
  kind: 'rose' | 'brokenHeart';
  x: number;
  width: number;
  height: number;
}

export const runnerGroundY = 204;
export const runnerHeroX = 40;
export const runnerHeroSize = 22;
const gravity = 0.0018;
const jumpVelocity = -0.58;
const jumpAirMilliseconds = (2 * -jumpVelocity) / gravity;
const initialSpeed = 0.18;
const maxSpeed = 0.42;
// Reaches max speed after about one minute of play.
const acceleration = 0.000004;
const pixelsPerPoint = 25;
const firstObstacleDistance = 200;
const landingRoom = 60;
const obstacleGapVariation = 140;
// Shrinks hitboxes a little so grazing an obstacle does not feel unfair.
const collisionTolerance = 3;
const groundDashSpacing = 24;

const obstacleSizeMap: Record<RunnerObstacle['kind'], Pick<RunnerObstacle, 'width' | 'height'>> = {
  rose: { width: 14, height: 34 },
  brokenHeart: { width: 22, height: 20 },
};

export class RunnerGame implements WaitingGameEngine {
  score = 0;
  isOver = false;
  heroBottom = runnerGroundY;
  obstacleList: RunnerObstacle[] = [];
  private verticalVelocity = 0;
  private speed = initialSpeed;
  private distance = 0;
  private distanceUntilNextObstacle = firstObstacleDistance;

  constructor(private readonly random: RandomSource) {}

  update(elapsedMilliseconds: number): void {
    if (this.isOver) return;
    this.speed = Math.min(maxSpeed, this.speed + acceleration * elapsedMilliseconds);
    const travelledDistance = this.speed * elapsedMilliseconds;
    this.distance += travelledDistance;
    this.score = Math.floor(this.distance / pixelsPerPoint);
    this.moveHero(elapsedMilliseconds);
    this.moveObstacles(travelledDistance);
    this.isOver = this.obstacleList.some((obstacle) => this.isHittingObstacle(obstacle));
  }

  handleTap(): void {
    if (this.isOver || this.heroBottom < runnerGroundY) return;
    this.verticalVelocity = jumpVelocity;
  }

  draw(context: CanvasRenderingContext2D): void {
    fillBackground(context, crushClubTheme.background);
    context.fillStyle = crushClubTheme.peach;
    context.fillRect(0, runnerGroundY, waitingGameWidth, waitingGameHeight - runnerGroundY);
    context.fillStyle = crushClubTheme.muted;
    context.fillRect(0, runnerGroundY, waitingGameWidth, 2);
    for (let dashX = -(this.distance % groundDashSpacing); dashX < waitingGameWidth; dashX += groundDashSpacing) {
      context.fillRect(dashX, runnerGroundY + 10, 8, 2);
    }
    this.obstacleList.forEach((obstacle) => drawObstacle(context, obstacle));
    drawHeart(context, runnerHeroX + runnerHeroSize / 2, this.heroBottom - runnerHeroSize / 2, runnerHeroSize, crushClubTheme.brand);
  }

  private moveHero(elapsedMilliseconds: number): void {
    this.verticalVelocity += gravity * elapsedMilliseconds;
    this.heroBottom += this.verticalVelocity * elapsedMilliseconds;
    if (this.heroBottom >= runnerGroundY) {
      this.heroBottom = runnerGroundY;
      this.verticalVelocity = 0;
    }
  }

  private moveObstacles(travelledDistance: number): void {
    this.obstacleList.forEach((obstacle) => (obstacle.x -= travelledDistance));
    this.obstacleList = this.obstacleList.filter((obstacle) => obstacle.x + obstacle.width > 0);
    this.distanceUntilNextObstacle -= travelledDistance;
    if (this.distanceUntilNextObstacle > 0) return;
    const kind = this.random() < 0.5 ? 'rose' : 'brokenHeart';
    this.obstacleList.push({ kind, x: waitingGameWidth, ...obstacleSizeMap[kind] });
    // The gap always leaves room to land and jump again at the current speed.
    this.distanceUntilNextObstacle = this.speed * jumpAirMilliseconds + landingRoom + this.random() * obstacleGapVariation;
  }

  private isHittingObstacle(obstacle: RunnerObstacle): boolean {
    const heroLeft = runnerHeroX + collisionTolerance;
    const heroRight = runnerHeroX + runnerHeroSize - collisionTolerance;
    const heroBottom = this.heroBottom - collisionTolerance;
    const obstacleLeft = obstacle.x + collisionTolerance;
    const obstacleRight = obstacle.x + obstacle.width - collisionTolerance;
    const obstacleTop = runnerGroundY - obstacle.height + collisionTolerance;
    return heroRight > obstacleLeft && heroLeft < obstacleRight && heroBottom > obstacleTop;
  }
}

function drawObstacle(context: CanvasRenderingContext2D, obstacle: RunnerObstacle): void {
  const top = runnerGroundY - obstacle.height;
  const centerX = obstacle.x + obstacle.width / 2;
  if (obstacle.kind === 'brokenHeart') {
    drawHeart(context, centerX, top + obstacle.height / 2, obstacle.width, crushClubTheme.lover);
    context.strokeStyle = crushClubTheme.background;
    context.lineWidth = 2;
    context.beginPath();
    context.moveTo(centerX, top + 4);
    context.lineTo(centerX - 3, top + 9);
    context.lineTo(centerX + 2, top + 13);
    context.lineTo(centerX - 1, top + obstacle.height - 2);
    context.stroke();
    return;
  }
  context.fillStyle = crushClubTheme.muted;
  context.fillRect(centerX - 1.5, top + 8, 3, obstacle.height - 8);
  context.beginPath();
  context.moveTo(centerX - 1.5, top + 18);
  context.lineTo(centerX - 6, top + 15);
  context.lineTo(centerX - 1.5, top + 22);
  context.moveTo(centerX + 1.5, top + 26);
  context.lineTo(centerX + 6, top + 23);
  context.lineTo(centerX + 1.5, top + 30);
  context.fill();
  context.fillStyle = crushClubTheme.coral;
  context.beginPath();
  context.arc(centerX, top + 7, obstacle.width / 2, 0, Math.PI * 2);
  context.fill();
  context.fillStyle = crushClubTheme.brand;
  context.beginPath();
  context.arc(centerX, top + 7, obstacle.width / 4, 0, Math.PI * 2);
  context.fill();
}
