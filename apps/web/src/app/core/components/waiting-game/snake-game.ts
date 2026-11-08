import { crushClubTheme } from '../../theme/crush-club-theme';
import { drawHeart, fillBackground, fillRoundedRect, waitingGameWidth } from './waiting-game-engine';
import type { RandomSource, WaitingGameDirection, WaitingGameEngine } from './waiting-game-engine';

export interface GridCell {
  column: number;
  row: number;
}

export const snakeColumnCount = 16;
export const snakeRowCount = 12;
export const snakeInitialTickMilliseconds = 140;
const cellSize = waitingGameWidth / snakeColumnCount;
const minimumTickMilliseconds = 80;
const tickSpeedupPerHeart = 4;
// Two buffered turns let a quick "up then left" swipe register within a single tick.
const maxQueuedDirectionCount = 2;
const checkerboardOpacity = 0.4;

const directionOffsetMap: Record<WaitingGameDirection, GridCell> = {
  up: { column: 0, row: -1 },
  down: { column: 0, row: 1 },
  left: { column: -1, row: 0 },
  right: { column: 1, row: 0 },
};

const oppositeDirectionMap: Record<WaitingGameDirection, WaitingGameDirection> = {
  up: 'down',
  down: 'up',
  left: 'right',
  right: 'left',
};

export class SnakeGame implements WaitingGameEngine {
  score = 0;
  isOver = false;
  /** Head first. */
  segmentList: GridCell[] = [
    { column: 6, row: 6 },
    { column: 5, row: 6 },
    { column: 4, row: 6 },
  ];
  foodCell: GridCell;
  private direction: WaitingGameDirection = 'right';
  private queuedDirectionList: WaitingGameDirection[] = [];
  private elapsedSinceTickMilliseconds = 0;

  constructor(private readonly random: RandomSource) {
    this.foodCell = this.findFreeCell() ?? { column: 0, row: 0 };
  }

  update(elapsedMilliseconds: number): void {
    if (this.isOver) return;
    this.elapsedSinceTickMilliseconds += elapsedMilliseconds;
    while (!this.isOver && this.elapsedSinceTickMilliseconds >= this.tickMilliseconds()) {
      this.elapsedSinceTickMilliseconds -= this.tickMilliseconds();
      this.moveOneCell();
    }
  }

  handleDirection(direction: WaitingGameDirection): void {
    const plannedDirection = this.queuedDirectionList.at(-1) ?? this.direction;
    const isIgnoredTurn = direction === plannedDirection || direction === oppositeDirectionMap[plannedDirection];
    if (isIgnoredTurn || this.queuedDirectionList.length >= maxQueuedDirectionCount) return;
    this.queuedDirectionList.push(direction);
  }

  draw(context: CanvasRenderingContext2D): void {
    fillBackground(context, crushClubTheme.background);
    context.fillStyle = crushClubTheme.peach;
    context.globalAlpha = checkerboardOpacity;
    for (let row = 0; row < snakeRowCount; row++) {
      for (let column = row % 2; column < snakeColumnCount; column += 2) {
        context.fillRect(column * cellSize, row * cellSize, cellSize, cellSize);
      }
    }
    context.globalAlpha = 1;
    drawHeart(context, (this.foodCell.column + 0.5) * cellSize, (this.foodCell.row + 0.5) * cellSize, cellSize * 0.8, crushClubTheme.gold);
    this.segmentList.forEach((segment, index) => {
      const color = index === 0 ? crushClubTheme.brand : crushClubTheme.coral;
      fillRoundedRect(context, segment.column * cellSize + 1, segment.row * cellSize + 1, cellSize - 2, cellSize - 2, 6, color);
    });
  }

  private tickMilliseconds(): number {
    return Math.max(minimumTickMilliseconds, snakeInitialTickMilliseconds - this.score * tickSpeedupPerHeart);
  }

  private moveOneCell(): void {
    this.direction = this.queuedDirectionList.shift() ?? this.direction;
    const offset = directionOffsetMap[this.direction];
    const head = this.segmentList[0];
    const nextHead = { column: head.column + offset.column, row: head.row + offset.row };
    const isEating = isSameCell(nextHead, this.foodCell);
    // The tail leaves its cell on this tick unless the snake grows, so moving into it is allowed.
    const blockingSegmentList = isEating ? this.segmentList : this.segmentList.slice(0, -1);
    const isOutside = nextHead.column < 0 || nextHead.column >= snakeColumnCount || nextHead.row < 0 || nextHead.row >= snakeRowCount;
    if (isOutside || blockingSegmentList.some((segment) => isSameCell(segment, nextHead))) {
      this.isOver = true;
      return;
    }
    this.segmentList.unshift(nextHead);
    if (!isEating) {
      this.segmentList.pop();
      return;
    }
    this.score++;
    const nextFoodCell = this.findFreeCell();
    if (nextFoodCell === null) {
      this.isOver = true;
      return;
    }
    this.foodCell = nextFoodCell;
  }

  private findFreeCell(): GridCell | null {
    const freeCellList: GridCell[] = [];
    for (let row = 0; row < snakeRowCount; row++) {
      for (let column = 0; column < snakeColumnCount; column++) {
        const cell = { column, row };
        if (!this.segmentList.some((segment) => isSameCell(segment, cell))) freeCellList.push(cell);
      }
    }
    if (freeCellList.length === 0) return null;
    return freeCellList[Math.floor(this.random() * freeCellList.length)];
  }
}

function isSameCell(firstCell: GridCell, secondCell: GridCell): boolean {
  return firstCell.column === secondCell.column && firstCell.row === secondCell.row;
}
