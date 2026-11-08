import { crushClubTheme } from '../../theme/crush-club-theme';
import { drawHeart, fillBackground, fillRoundedRect, waitingGameHeight, waitingGameWidth } from './waiting-game-engine';
import type { RandomSource, WaitingGameDirection, WaitingGameEngine } from './waiting-game-engine';

export interface PaddleBall {
  x: number;
  y: number;
  velocityX: number;
  velocityY: number;
}

export const paddleWidth = 8;
export const paddleHeight = 44;
export const playerPaddleX = 12;
export const cpuPaddleX = waitingGameWidth - playerPaddleX - paddleWidth;
export const paddleBallSize = 8;
export const paddleLifeCount = 3;
export const paddleServeDelayMilliseconds = 700;
const initialBallSpeed = 0.2;
const ballSpeedupPerHit = 0.015;
const maxBallSpeed = 0.45;
// Slower than the ball's steepest shots, so the CPU can be beaten by hitting with the paddle edges.
const cpuMaxSpeed = 0.15;
const playerMaxSpeed = 0.9;
const keyboardStep = 36;
const maxBounceAngle = Math.PI / 3.5;
const centerLineDashHeight = 8;

export class PaddleGame implements WaitingGameEngine {
  score = 0;
  isOver = false;
  lifeCount = paddleLifeCount;
  playerCenterY = waitingGameHeight / 2;
  cpuCenterY = waitingGameHeight / 2;
  ball: PaddleBall;
  remainingServeDelayMilliseconds = paddleServeDelayMilliseconds;
  private playerTargetY = waitingGameHeight / 2;
  private ballSpeed = initialBallSpeed;

  constructor(private readonly random: RandomSource) {
    this.ball = this.createServedBall();
  }

  update(elapsedMilliseconds: number): void {
    if (this.isOver) return;
    this.playerCenterY = movePaddleToward(this.playerCenterY, this.playerTargetY, playerMaxSpeed * elapsedMilliseconds);
    const cpuTargetY = this.ball.velocityX > 0 ? this.ball.y + paddleBallSize / 2 : waitingGameHeight / 2;
    this.cpuCenterY = movePaddleToward(this.cpuCenterY, cpuTargetY, cpuMaxSpeed * elapsedMilliseconds);
    if (this.remainingServeDelayMilliseconds > 0) {
      this.remainingServeDelayMilliseconds -= elapsedMilliseconds;
      return;
    }
    this.moveBall(elapsedMilliseconds);
  }

  handlePointerMove(_ratioX: number, ratioY: number): void {
    this.playerTargetY = ratioY * waitingGameHeight;
  }

  handleDirection(direction: WaitingGameDirection): void {
    if (direction === 'up') this.playerTargetY = this.playerCenterY - keyboardStep;
    if (direction === 'down') this.playerTargetY = this.playerCenterY + keyboardStep;
  }

  draw(context: CanvasRenderingContext2D): void {
    fillBackground(context, crushClubTheme.background);
    context.fillStyle = crushClubTheme.peach;
    for (let dashY = 0; dashY < waitingGameHeight; dashY += centerLineDashHeight * 2) {
      context.fillRect(waitingGameWidth / 2 - 1, dashY, 2, centerLineDashHeight);
    }
    fillRoundedRect(context, playerPaddleX, this.playerCenterY - paddleHeight / 2, paddleWidth, paddleHeight, 4, crushClubTheme.brand);
    fillRoundedRect(context, cpuPaddleX, this.cpuCenterY - paddleHeight / 2, paddleWidth, paddleHeight, 4, crushClubTheme.lover);
    drawHeart(context, this.ball.x + paddleBallSize / 2, this.ball.y + paddleBallSize / 2, paddleBallSize + 4, crushClubTheme.gold);
    for (let lifeIndex = 0; lifeIndex < this.lifeCount; lifeIndex++) {
      drawHeart(context, 34 + lifeIndex * 12, 12, 9, crushClubTheme.brand);
    }
  }

  private moveBall(elapsedMilliseconds: number): void {
    const previousX = this.ball.x;
    this.ball.x += this.ball.velocityX * elapsedMilliseconds;
    this.ball.y += this.ball.velocityY * elapsedMilliseconds;
    if (this.ball.y < 0) {
      this.ball.y = 0;
      this.ball.velocityY = Math.abs(this.ball.velocityY);
    }
    if (this.ball.y + paddleBallSize > waitingGameHeight) {
      this.ball.y = waitingGameHeight - paddleBallSize;
      this.ball.velocityY = -Math.abs(this.ball.velocityY);
    }
    // Compares against the previous position so a fast ball cannot tunnel through a paddle on a slow frame.
    const playerFaceX = playerPaddleX + paddleWidth;
    const hasCrossedPlayerFace = this.ball.velocityX < 0 && previousX >= playerFaceX && this.ball.x < playerFaceX;
    if (hasCrossedPlayerFace && this.isBallAlignedWith(this.playerCenterY)) {
      this.ball.x = playerFaceX;
      this.bounceBall(this.playerCenterY, 1);
    }
    const hasCrossedCpuFace = this.ball.velocityX > 0 && previousX + paddleBallSize <= cpuPaddleX && this.ball.x + paddleBallSize > cpuPaddleX;
    if (hasCrossedCpuFace && this.isBallAlignedWith(this.cpuCenterY)) {
      this.ball.x = cpuPaddleX - paddleBallSize;
      this.bounceBall(this.cpuCenterY, -1);
    }
    if (this.ball.x + paddleBallSize < 0) this.losePoint();
    if (this.ball.x > waitingGameWidth) this.winPoint();
  }

  private isBallAlignedWith(paddleCenterY: number): boolean {
    return this.ball.y + paddleBallSize > paddleCenterY - paddleHeight / 2 && this.ball.y < paddleCenterY + paddleHeight / 2;
  }

  /** The further from the paddle center the ball hits, the steeper it bounces back. */
  private bounceBall(paddleCenterY: number, horizontalDirection: 1 | -1): void {
    const hitOffset = (this.ball.y + paddleBallSize / 2 - paddleCenterY) / (paddleHeight / 2);
    const bounceAngle = Math.max(-1, Math.min(1, hitOffset)) * maxBounceAngle;
    this.ballSpeed = Math.min(maxBallSpeed, this.ballSpeed + ballSpeedupPerHit);
    this.ball.velocityX = horizontalDirection * this.ballSpeed * Math.cos(bounceAngle);
    this.ball.velocityY = this.ballSpeed * Math.sin(bounceAngle);
  }

  private losePoint(): void {
    this.lifeCount--;
    if (this.lifeCount === 0) {
      this.isOver = true;
      return;
    }
    this.serveBall();
  }

  private winPoint(): void {
    this.score++;
    this.serveBall();
  }

  private serveBall(): void {
    this.ballSpeed = initialBallSpeed;
    this.ball = this.createServedBall();
    this.remainingServeDelayMilliseconds = paddleServeDelayMilliseconds;
  }

  private createServedBall(): PaddleBall {
    const serveAngle = (this.random() - 0.5) * maxBounceAngle;
    return {
      x: (waitingGameWidth - paddleBallSize) / 2,
      y: (waitingGameHeight - paddleBallSize) / 2,
      velocityX: -initialBallSpeed * Math.cos(serveAngle),
      velocityY: initialBallSpeed * Math.sin(serveAngle),
    };
  }
}

function movePaddleToward(currentCenterY: number, targetCenterY: number, maxDistance: number): number {
  const clampedTargetY = Math.max(paddleHeight / 2, Math.min(waitingGameHeight - paddleHeight / 2, targetCenterY));
  const distance = clampedTargetY - currentCenterY;
  return currentCenterY + Math.max(-maxDistance, Math.min(maxDistance, distance));
}
