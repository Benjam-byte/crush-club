import { describe, expect, it } from 'vitest';
import { RunnerGame, runnerGroundY, runnerHeroX } from './runner-game';

const pickRose = (): number => 0;
const frameMilliseconds = 50;

describe('RunnerGame', () => {
  it('jumps on tap and lands back on the ground', () => {
    const game = new RunnerGame(pickRose);
    game.handleTap();
    game.update(frameMilliseconds);
    expect(game.heroBottom).toBeLessThan(runnerGroundY);
    for (let frame = 0; frame < 16; frame++) game.update(frameMilliseconds);
    expect(game.heroBottom).toBe(runnerGroundY);
  });

  it('scores with the distance travelled', () => {
    const game = new RunnerGame(pickRose);
    for (let frame = 0; frame < 20; frame++) game.update(frameMilliseconds);
    expect(game.score).toBeGreaterThan(0);
    expect(game.isOver).toBe(false);
  });

  it('ends the game when running into an obstacle', () => {
    const game = new RunnerGame(pickRose);
    game.obstacleList.push({ kind: 'rose', x: runnerHeroX, width: 14, height: 34 });
    game.update(16);
    expect(game.isOver).toBe(true);
  });

  it('clears a low obstacle while in the air', () => {
    const game = new RunnerGame(pickRose);
    game.handleTap();
    for (let frame = 0; frame < 5; frame++) game.update(frameMilliseconds);
    game.obstacleList.push({ kind: 'brokenHeart', x: runnerHeroX, width: 22, height: 20 });
    game.update(16);
    expect(game.isOver).toBe(false);
  });
});
