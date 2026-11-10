import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

export interface FinalLeaderboardEntry {
  id: string;
  name: string;
  score: number;
  subtitle?: string;
  photo?: string | null;
}

export interface RankedEntry extends FinalLeaderboardEntry {
  rank: number;
}

/** The default score display: "12 pts". */
export function pointsLabel(score: number): string {
  return `${score} pts`;
}

/** Ties share a rank ("1, 1, 3"), so two players on the same score never look ranked apart. Entries arrive sorted by score. */
export function rankEntries(entries: readonly FinalLeaderboardEntry[]): RankedEntry[] {
  return entries.map((entry) => ({
    ...entry,
    rank: entries.findIndex((candidate) => candidate.score === entry.score) + 1,
  }));
}

/** Final ranking: a podium for the top three, then a scrolling list. Entries arrive sorted by score. */
@Component({
  selector: 'app-final-leaderboard',
  templateUrl: './final-leaderboard.component.html',
  styleUrl: './final-leaderboard.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FinalLeaderboardComponent {
  readonly entries = input.required<readonly FinalLeaderboardEntry[]>();
  readonly accent = input<'coral' | 'lover'>('coral');
  /** How a score reads, "12 pts" by default; a mode can count something else ("4 bonnes places"). */
  readonly formatScore = input<(score: number) => string>(pointsLabel);

  private readonly rankedEntries = computed(() => {
    const formatScore = this.formatScore();
    return rankEntries(this.entries()).map((entry) => ({ ...entry, scoreLabel: formatScore(entry.score) }));
  });
  protected readonly podium = computed(() => this.rankedEntries().slice(0, 3));
  protected readonly remaining = computed(() => this.rankedEntries().slice(3));
  /** Shows the podium as second, first, third while screen readers keep the ranking order. */
  protected readonly podiumOrder = [2, 1, 3];

  protected rankLabel(rank: number): string {
    return rank === 1 ? '1er' : `${rank}e`;
  }

  protected initials(name: string): string {
    return name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
