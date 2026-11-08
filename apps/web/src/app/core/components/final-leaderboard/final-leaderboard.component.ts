import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

export interface FinalLeaderboardEntry {
  id: string;
  name: string;
  score: number;
  subtitle?: string;
  photo?: string | null;
}

@Component({
  selector: 'app-final-leaderboard',
  templateUrl: './final-leaderboard.component.html',
  styleUrl: './final-leaderboard.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FinalLeaderboardComponent {
  readonly entries = input.required<readonly FinalLeaderboardEntry[]>();
  readonly accent = input<'coral' | 'lover'>('coral');
  protected readonly podium = computed(() => this.entries().slice(0, 3));
  protected readonly remaining = computed(() => this.entries().slice(3));

  protected initials(name: string): string {
    return name.trim().split(/\s+/).slice(0, 2).map(part => part[0]?.toUpperCase() ?? '').join('');
  }
}
