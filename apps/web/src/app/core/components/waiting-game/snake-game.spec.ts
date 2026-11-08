import { describe, expect, it } from 'vitest';
import { SnakeGame, snakeColumnCount, snakeInitialTickMilliseconds } from './snake-game';

const pickFirstFreeCell = (): number => 0;

describe('SnakeGame', () => {
  it('moves one cell per tick in the current direction', () => {
    const game = new SnakeGame(pickFirstFreeCell);
    game.update(snakeInitialTickMilliseconds);
    expect(game.segmentList[0]).toEqual({ column: 7, row: 6 });
    expect(game.segmentList).toHaveLength(3);
  });

  it('grows and scores when eating a heart', () => {
    const game = new SnakeGame(pickFirstFreeCell);
    game.foodCell = { column: 7, row: 6 };
    game.update(snakeInitialTickMilliseconds);
    expect(game.score).toBe(1);
    expect(game.segmentList).toHaveLength(4);
    expect(game.foodCell).toEqual({ column: 0, row: 0 });
  });

  it('ends the game when hitting a wall', () => {
    const game = new SnakeGame(pickFirstFreeCell);
    const moveCountToLastColumn = snakeColumnCount - 1 - game.segmentList[0].column;
    for (let tick = 0; tick < moveCountToLastColumn; tick++) game.update(snakeInitialTickMilliseconds);
    expect(game.isOver).toBe(false);
    game.update(snakeInitialTickMilliseconds);
    expect(game.isOver).toBe(true);
  });

  it('ignores a turn back onto itself', () => {
    const game = new SnakeGame(pickFirstFreeCell);
    game.handleDirection('left');
    game.update(snakeInitialTickMilliseconds);
    expect(game.segmentList[0]).toEqual({ column: 7, row: 6 });
    expect(game.isOver).toBe(false);
  });

  it('applies two quick turns on consecutive ticks', () => {
    const game = new SnakeGame(pickFirstFreeCell);
    game.handleDirection('up');
    game.handleDirection('left');
    game.update(snakeInitialTickMilliseconds);
    expect(game.segmentList[0]).toEqual({ column: 6, row: 5 });
    game.update(snakeInitialTickMilliseconds);
    expect(game.segmentList[0]).toEqual({ column: 5, row: 5 });
  });
});
