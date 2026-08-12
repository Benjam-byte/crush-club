import { ChangeDetectionStrategy, Component, signal } from '@angular/core';

interface BurstItem {
  id: number
  emoji: string
  left: number
}

/**
 * Overlay that spawns floating emoji which rise and fade out, used to show
 * Fast Bio reactions live as they come in over the websocket. Purely
 * presentational: call `push(emoji)` whenever a reaction event arrives.
 */
@Component({
  selector: 'app-reaction-burst',
  template: `
    <div class="reaction-burst-layer" aria-hidden="true">
      @for (item of items(); track item.id) {
        <span class="reaction-burst-emoji" [style.left.%]="item.left">{{ item.emoji }}</span>
      }
    </div>
  `,
  styleUrl: './reaction-burst.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ReactionBurstComponent {
  private readonly itemsState = signal<readonly BurstItem[]>([]);
  private nextId = 0;

  readonly items = this.itemsState.asReadonly();

  push(emoji: string): void {
    const id = this.nextId++;
    const left = 15 + Math.random() * 60;
    this.itemsState.update((current) => [...current, { id, emoji, left }]);
    setTimeout(() => {
      this.itemsState.update((current) => current.filter((item) => item.id !== id));
    }, 1600);
  }

  clear(): void {
    this.itemsState.set([]);
  }
}
