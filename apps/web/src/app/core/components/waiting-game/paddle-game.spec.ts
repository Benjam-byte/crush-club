import { describe, expect, it } from 'vitest';
import { PaddleGame, cpuPaddleX, paddleBallSize, paddleHeight, paddleLifeCount, paddleWidth, playerPaddleX } from './paddle-game';
import { waitingGameHeight } from './waiting-game-engine';

const serveStraight = (): number => 0.5;

function createRallyingGame(): PaddleGame {
  const game = new PaddleGame(serveStraight);
  game.remainingServeDelayMilliseconds = 0;
  return game;
}

describe('PaddleGame', () => {
  it('sends the ball back when it hits the player paddle', () => {
    const game = createRallyingGame();
    game.ball = { x: playerPaddleX + paddleWidth + 1, y: game.playerCenterY - paddleBallSize / 2, velocityX: -0.2, velocityY: 0 };
    game.update(16);
    expect(game.ball.velocityX).toBeGreaterThan(0);
    expect(game.lifeCount).toBe(paddleLifeCount);
  });

  it('costs a life on each miss and ends after the last one', () => {
    const game = createRallyingGame();
    for (let missIndex = 0; missIndex < paddleLifeCount; missIndex++) {
      expect(game.isOver).toBe(false);
      game.remainingServeDelayMilliseconds = 0;
      game.ball = { x: 2, y: 0, velocityX: -0.2, velocityY: 0 };
      game.update(100);
    }
    expect(game.lifeCount).toBe(0);
    expect(game.isOver).toBe(true);
  });

  it('scores when the CPU misses', () => {
    const game = createRallyingGame();
    game.cpuCenterY = paddleHeight / 2;
    game.ball = { x: cpuPaddleX + paddleWidth + 1, y: waitingGameHeight - paddleBallSize, velocityX: 0.3, velocityY: 0 };
    game.update(100);
    expect(game.score).toBe(1);
    expect(game.isOver).toBe(false);
  });

  it('follows the finger with the player paddle', () => {
    const game = createRallyingGame();
    game.handlePointerMove(0, 0.25);
    game.update(200);
    expect(game.playerCenterY).toBe(waitingGameHeight * 0.25);
  });
});
