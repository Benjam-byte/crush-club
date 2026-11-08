import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  NgZone,
  afterNextRender,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import type { ElementRef } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { pickWaitingGameDefinition, readBestScore, saveBestScore } from './waiting-game-catalog';
import { findSwipeDirection, waitingGameHeight, waitingGameWidth } from './waiting-game-engine';
import type { WaitingGameDirection, WaitingGameKind } from './waiting-game-engine';

type WaitingGameStatus = 'idle' | 'playing' | 'paused' | 'over';

// Caps the step after a stalled frame so the game never jumps ahead.
const maxFrameMilliseconds = 50;

const keyDirectionMap: Partial<Record<string, WaitingGameDirection>> = {
  ArrowUp: 'up',
  KeyW: 'up',
  ArrowDown: 'down',
  KeyS: 'down',
  ArrowLeft: 'left',
  KeyA: 'left',
  ArrowRight: 'right',
  KeyD: 'right',
};
const tapKeyCodeList = ['Space', 'ArrowUp', 'KeyW'];

const playLabelMap: Record<WaitingGameStatus, string> = {
  idle: 'Jouer',
  playing: '',
  paused: 'Reprendre',
  over: 'Rejouer',
};

// Shared across instances so the next waiting screen picks a different game.
let previousWaitingGameKind: WaitingGameKind | null = null;

/** Small solo game shown on waiting screens; it never blocks the flow and disappears with its screen. */
@Component({
  selector: 'app-waiting-game',
  imports: [IonIcon],
  templateUrl: './waiting-game.component.html',
  styleUrl: './waiting-game.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WaitingGameComponent {
  private readonly ngZone = inject(NgZone);
  private readonly destroyRef = inject(DestroyRef);
  private readonly canvasRef = viewChild.required<ElementRef<HTMLCanvasElement>>('gameCanvas');

  protected readonly definition = pickWaitingGameDefinition(Math.random, previousWaitingGameKind);
  protected readonly status = signal<WaitingGameStatus>('idle');
  protected readonly score = signal(0);
  protected readonly bestScore = signal(readBestScore(this.definition.kind));
  protected readonly isNewRecord = signal(false);
  protected readonly isPlaying = computed(() => this.status() === 'playing');
  protected readonly displayedBestScore = computed(() => Math.max(this.bestScore(), this.score()));
  protected readonly playLabel = computed(() => playLabelMap[this.status()]);
  protected readonly gameOverMessage = computed(() =>
    this.isNewRecord() ? `Nouveau record : ${this.score()} pts !` : `Perdu ! ${this.score()} pts`,
  );

  private engine = this.definition.createEngine(Math.random);
  private context: CanvasRenderingContext2D | null = null;
  private animationFrameId: number | null = null;
  private lastFrameTimestamp: number | null = null;
  private swipeOrigin: { x: number; y: number } | null = null;

  constructor() {
    previousWaitingGameKind = this.definition.kind;
    // Pointer and frame callbacks fire constantly; running them outside the zone avoids an app-wide check on each one.
    afterNextRender(() => this.ngZone.runOutsideAngular(() => this.setUpCanvas()));
    this.destroyRef.onDestroy(() => {
      this.stopLoop();
      saveBestScore(this.definition.kind, this.engine.score);
    });
  }

  protected onPlay(): void {
    if (this.status() === 'over') {
      this.engine = this.definition.createEngine(Math.random);
      this.score.set(0);
      this.isNewRecord.set(false);
    }
    this.status.set('playing');
    this.canvasRef().nativeElement.focus({ preventScroll: true });
    this.ngZone.runOutsideAngular(() => this.startLoop());
  }

  private setUpCanvas(): void {
    const canvas = this.canvasRef().nativeElement;
    this.context = canvas.getContext('2d');
    const resizeObserver = new ResizeObserver(() => this.resizeCanvas());
    // Ionic keeps previous pages alive with display:none after a forward navigation, so leaving the viewport is the reliable pause signal.
    const intersectionObserver = new IntersectionObserver((entryList) => {
      if (entryList.at(-1)?.isIntersecting === false) this.pause();
    });
    const handleVisibilityChange = (): void => {
      if (document.hidden) this.pause();
    };
    resizeObserver.observe(canvas);
    intersectionObserver.observe(canvas);
    document.addEventListener('visibilitychange', handleVisibilityChange);
    canvas.addEventListener('pointerdown', (event) => this.handlePointerDown(event));
    canvas.addEventListener('pointermove', (event) => this.handlePointerMove(event));
    canvas.addEventListener('pointerup', () => this.handlePointerEnd());
    canvas.addEventListener('pointercancel', () => this.handlePointerEnd());
    canvas.addEventListener('keydown', (event) => this.handleKeyDown(event));
    this.destroyRef.onDestroy(() => {
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    });
  }

  private resizeCanvas(): void {
    const canvas = this.canvasRef().nativeElement;
    const pixelRatio = window.devicePixelRatio || 1;
    canvas.width = Math.round(canvas.clientWidth * pixelRatio);
    canvas.height = Math.round(canvas.clientHeight * pixelRatio);
    this.context?.setTransform(canvas.width / waitingGameWidth, 0, 0, canvas.height / waitingGameHeight, 0, 0);
    this.drawFrame();
  }

  private startLoop(): void {
    this.lastFrameTimestamp = null;
    this.animationFrameId = requestAnimationFrame((timestamp) => this.runFrame(timestamp));
  }

  private stopLoop(): void {
    if (this.animationFrameId !== null) cancelAnimationFrame(this.animationFrameId);
    this.animationFrameId = null;
  }

  private runFrame(timestamp: number): void {
    const elapsedMilliseconds = this.lastFrameTimestamp === null ? 0 : Math.min(maxFrameMilliseconds, timestamp - this.lastFrameTimestamp);
    this.lastFrameTimestamp = timestamp;
    this.engine.update(elapsedMilliseconds);
    this.drawFrame();
    if (this.engine.score !== this.score()) this.score.set(this.engine.score);
    if (this.engine.isOver) {
      this.finishGame();
      return;
    }
    this.animationFrameId = requestAnimationFrame((nextTimestamp) => this.runFrame(nextTimestamp));
  }

  private finishGame(): void {
    this.animationFrameId = null;
    const finalScore = this.engine.score;
    this.isNewRecord.set(finalScore > this.bestScore());
    saveBestScore(this.definition.kind, finalScore);
    this.bestScore.set(Math.max(this.bestScore(), finalScore));
    this.status.set('over');
  }

  private pause(): void {
    if (!this.isPlaying()) return;
    this.stopLoop();
    this.swipeOrigin = null;
    this.status.set('paused');
  }

  private drawFrame(): void {
    if (this.context) this.engine.draw(this.context);
  }

  private handlePointerDown(event: PointerEvent): void {
    if (!this.isPlaying()) return;
    event.preventDefault();
    this.canvasRef().nativeElement.setPointerCapture(event.pointerId);
    this.swipeOrigin = { x: event.clientX, y: event.clientY };
    this.engine.handleTap?.();
    this.forwardPointerPosition(event);
  }

  private handlePointerMove(event: PointerEvent): void {
    if (!this.isPlaying()) return;
    if (this.swipeOrigin !== null) {
      event.preventDefault();
      const swipeDirection = findSwipeDirection(event.clientX - this.swipeOrigin.x, event.clientY - this.swipeOrigin.y);
      if (swipeDirection !== null) {
        this.swipeOrigin = { x: event.clientX, y: event.clientY };
        this.engine.handleDirection?.(swipeDirection);
      }
    }
    // Sent after the swipe so a dragged paddle follows the finger rather than the swipe step.
    this.forwardPointerPosition(event);
  }

  private handlePointerEnd(): void {
    this.swipeOrigin = null;
  }

  private forwardPointerPosition(event: PointerEvent): void {
    if (!this.engine.handlePointerMove) return;
    const canvasBounds = this.canvasRef().nativeElement.getBoundingClientRect();
    if (canvasBounds.width === 0 || canvasBounds.height === 0) return;
    this.engine.handlePointerMove(
      (event.clientX - canvasBounds.left) / canvasBounds.width,
      (event.clientY - canvasBounds.top) / canvasBounds.height,
    );
  }

  private handleKeyDown(event: KeyboardEvent): void {
    if (!this.isPlaying()) return;
    const direction = keyDirectionMap[event.code];
    const isTapKey = tapKeyCodeList.includes(event.code);
    if (direction === undefined && !isTapKey) return;
    event.preventDefault();
    if (direction !== undefined) this.engine.handleDirection?.(direction);
    if (isTapKey && !event.repeat) this.engine.handleTap?.();
  }
}
