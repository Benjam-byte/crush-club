import { InfoSheetComponent } from '@core/components/info-sheet/info-sheet.component';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { ZeroToHundredScaleComponent, nomineeColor, nomineeTextColor } from '@core/components/zero-to-100-scale/zero-to-100-scale.component';
import type { ScalePoint } from '@core/components/zero-to-100-scale/zero-to-100-scale.component';
import type { ZeroToHundredRevealEntryView } from '@core/models/game.models';
import { GameStateService } from '@core/services/game-state.service';

/** "+5" or "0", for the points column. */
function signedPoints(points: number): string {
  return points > 0 ? `+${points}` : '0';
}

function nomineeLabel(entry: ZeroToHundredRevealEntryView): string {
  return entry.isCurrentPlayer ? 'Toi' : entry.displayName;
}

@Component({
  selector: 'app-zero-to-100-results-page',
  imports: [InfoSheetComponent, IonButton, IonContent, IonIcon, PageHeaderComponent, WaitingCardComponent, ZeroToHundredScaleComponent],
  templateUrl: './zero-to-100-results.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ZeroToHundredResultsPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isStartingNextRound = signal(false);
  protected readonly reveal = computed(() => this.gameState.zeroToHundredGame()?.reveal ?? []);
  protected readonly roundScore = computed(() => this.gameState.zeroToHundredGame()?.roundScore ?? 0);
  protected readonly roundScoreLabel = computed(() => {
    const roundScore = this.roundScore();
    return `${roundScore} ${roundScore > 1 ? 'pts' : 'pt'}`;
  });
  protected readonly orderBonus = computed(() => this.gameState.zeroToHundredGame()?.orderBonus ?? 0);
  protected readonly orderBonusLabel = computed(() => signedPoints(this.orderBonus()));
  protected readonly myBar = computed(() => this.barPoints((entry) => entry.myPosition));
  protected readonly targetBar = computed(() => this.barPoints((entry) => entry.targetPosition));
  protected readonly themeLabel = computed(() => this.gameState.zeroToHundredGame()?.themeLabel ?? '');

  private readonly focusSelection = signal<string | null>(null);
  protected readonly focusOptions = computed(() =>
    this.reveal().map((entry, seat) => ({
      id: entry.playerId,
      label: nomineeLabel(entry),
      color: nomineeColor(seat),
      isCurrentPlayer: entry.isCurrentPlayer,
    })),
  );
  /** The nominee shown alone on the bars, or null for everyone (also once they leave the reveal). */
  protected readonly focusedOption = computed(
    () => this.focusOptions().find((option) => option.id === this.focusSelection()) ?? null,
  );
  protected readonly activeFocusId = computed(() => this.focusedOption()?.id ?? null);
  protected readonly showsMyBar = computed(() => this.barShowsFocus(this.myBar()));
  protected readonly showsTargetBar = computed(() => this.barShowsFocus(this.targetBar()));
  protected readonly targetBarNote = computed(() => {
    const option = this.focusedOption();
    if (option === null) {
      return 'Aucun nominé n’a de cible.';
    }
    return option.isCurrentPlayer
      ? 'Tu n’as pas de cible : pas placé·e à temps.'
      : `${option.label} n’a pas de cible : pas placé·e à temps.`;
  });
  /** "Camille s’est mis·e 80 en « thème »", one per nominee in seat order, following the focused dot. */
  protected readonly selfPlacementLines = computed(() => {
    const focusId = this.activeFocusId();
    return this.reveal().flatMap((entry, seat) =>
      focusId !== null && entry.playerId !== focusId
        ? []
        : [
            {
              playerId: entry.playerId,
              isCurrentPlayer: entry.isCurrentPlayer,
              name: entry.isCurrentPlayer ? 'Tu' : entry.displayName,
              value: entry.truePosition,
              color: nomineeColor(seat),
              textColor: nomineeTextColor(seat),
            },
          ],
    );
  });
  /** One row per nominee you could score on: your own self-placement never earns points. */
  protected readonly pointRows = computed(() =>
    this.reveal().flatMap((entry, seat) =>
      entry.isCurrentPlayer
        ? []
        : [
            {
              playerId: entry.playerId,
              label: entry.displayName,
              color: nomineeColor(seat),
              myPositionLabel: entry.myPosition === undefined ? '—' : `${entry.myPosition}`,
              targetLabel: entry.targetPosition === undefined ? 'absent' : `${entry.targetPosition}`,
              pointsLabel: signedPoints(entry.points),
              hasPoints: entry.points > 0,
            },
          ],
    ),
  );
  protected readonly orderBonusReason = computed(() => {
    if (this.orderBonus() > 0) {
      return 'bon ordre';
    }
    if (this.targetBar().length < 2) {
      return 'moins de 2 placés';
    }
    if (this.myBar().length === 0) {
      return 'pas de réponse';
    }
    return 'ordre différent';
  });
  protected readonly isLastRound = computed(() => {
    const zeroToHundred = this.gameState.zeroToHundredGame();
    return zeroToHundred?.roundNumber === zeroToHundred?.totalRounds;
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected async onStartNextRound(): Promise<void> {
    if (this.isStartingNextRound() || !this.gameState.isHost()) {
      return;
    }
    this.isStartingNextRound.set(true);
    try {
      await this.gameState.startNextZeroToHundredRound();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isStartingNextRound.set(false);
    }
  }

  /** Shows only this nominee on the bars; tapping the same dot again shows everyone. */
  protected toggleFocus(playerId: string): void {
    this.focusSelection.update((current) => (current === playerId ? null : playerId));
  }

  private barShowsFocus(points: readonly ScalePoint[]): boolean {
    const focusId = this.activeFocusId();
    return points.some((point) => focusId === null || point.id === focusId);
  }

  /** One scale point per nominee that has a position on this bar; the seat keeps colors aligned across bars. */
  private barPoints(positionOf: (entry: ZeroToHundredRevealEntryView) => number | undefined): ScalePoint[] {
    return this.reveal().flatMap((entry, seat) => {
      const value = positionOf(entry);
      return value === undefined ? [] : [{ id: entry.playerId, label: nomineeLabel(entry), value, colorIndex: seat }];
    });
  }
}
