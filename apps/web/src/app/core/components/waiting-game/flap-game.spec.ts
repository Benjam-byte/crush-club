import { describe, expect, it } from 'vitest';
import { FlapGame, flapColumnWidth, flapGapMargin, flapGroundY, flapHeroSize, flapHeroX, flapMinimumGapHeight } from './flap-game';

const centerGap = (): number => 0.5;
const frameMilliseconds = 50;

describe('FlapGame', () => {
  it('hovers in place until the first tap', () => {
    const game = new FlapGame(centerGap);
    for (let frame = 0; frame < 20; frame++) game.update(frameMilliseconds);
    expect(Math.abs(game.heroCenterY - flapGroundY / 2)).toBeLessThanOrEqual(4);
    expect(game.columnList).toHaveLength(0);
    expect(game.isOver).toBe(false);
  });

  it('climbs on tap, then falls to the ground without new taps', () => {
    const game = new FlapGame(centerGap);
    const startCenterY = game.heroCenterY;
    game.handleTap();
    game.update(frameMilliseconds);
    expect(game.heroCenterY).toBeLessThan(startCenterY);
    for (let frame = 0; frame < 30; frame++) game.update(frameMilliseconds);
    expect(game.isOver).toBe(true);
  });

  it('scores one point per column passed', () => {
    const game = new FlapGame(centerGap);
    game.handleTap();
    game.columnList.push({ x: flapHeroX - flapHeroSize / 2 - flapColumnWidth, gapTop: 0, gapHeight: 10, isPassed: false });
    game.update(16);
    expect(game.score).toBe(1);
    expect(game.isOver).toBe(false);
  });

  it('ends the game when hitting a column', () => {
    const game = new FlapGame(centerGap);
    game.handleTap();
    game.columnList.push({ x: flapHeroX - 5, gapTop: 0, gapHeight: 10, isPassed: false });
    game.update(16);
    expect(game.isOver).toBe(true);
  });

  it('keeps every gap inside the sky, even at its smallest', () => {
    for (const randomValue of [0, 0.999]) {
      const game = new FlapGame(() => randomValue);
      game.score = 100;
      game.handleTap();
      game.update(16);
      const column = game.columnList[0];
      expect(column.gapHeight).toBe(flapMinimumGapHeight);
      expect(column.gapTop).toBeGreaterThanOrEqual(flapGapMargin);
      expect(column.gapTop + column.gapHeight).toBeLessThanOrEqual(flapGroundY - flapGapMargin);
    }
  });
});
