import { ChangeDetectionStrategy, Component, computed, effect, inject, signal, untracked } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { prefersReducedMotion, ScreenWakeLock, vibrate } from '@core/services/device-feedback';
import { GameStateService } from '@core/services/game-state.service';
import { formatEuros, formatEurosCompact } from '@core/services/money-format';
import {
  breaksLine,
  duplicateSeatNames,
  groupTitle,
  isTableNumbered,
  nextCyclePosition,
  rankRangeLabel,
  revealedCount,
  revealSlotAt,
  revealStageAt,
  secretRankingQuestionEnd,
  secretRankingQuestionStart,
  seatColor,
} from '@core/services/secret-ranking';
import type { RevealStage, RevealTimeline } from '@core/services/secret-ranking';

/** Delay between the last tap on a phone and sending its number: quick taps send only the final one. */
const tapSendDelayMs = 300;
const countUpDurationMs = 700;
const digitLeaveMs = 200;
const lockDelayMs = 5000;
/** Below this age a flip is played; a phone that arrives later (reload) shows the result straight away. */
const liveFlipWindowMs = 600;
const confettiCount = 30;
const confettiColors = ['#ffd166', '#ef476f', '#06d6a0', '#118ab2', '#ffffff', '#f78c6b'];

interface DialDigit {
  key: number
  value: string
  leaving: boolean
}

interface ConfettiPiece {
  left: number
  delayMs: number
  durationMs: number
  color: string
  rotation: number
}

/**
 * Screen B: the phone lies on the table. It shows the player's name and number, which a tap moves on; once
 * the numbers lock, the same page plays the domino reveal so the animation is never cut by a navigation.
 */
@Component({
  selector: 'app-secret-ranking-table-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './secret-ranking-table.page.html',
  styleUrl: './secret-ranking-table.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SecretRankingTablePage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly wakeLock = new ScreenWakeLock();

  protected readonly round = this.gameState.secretRankingGame;
  protected readonly questionStart = secretRankingQuestionStart;
  protected readonly questionEnd = computed(() => secretRankingQuestionEnd(this.round()?.themeLabel ?? ''));
  protected readonly formatEuros = formatEuros;
  protected readonly formatEurosCompact = formatEurosCompact;
  protected readonly rankRangeLabel = rankRangeLabel;
  protected readonly isWakeLockSupported = this.wakeLock.isSupported;

  /** Server time, refreshed every 100 ms: every phone of the table paces the lock and the reveal on it. */
  protected readonly serverNow = signal(Date.now());
  /** This phone's number as last tapped, ahead of the server until it is sent. */
  protected readonly localPosition = signal<number | null>(null);
  protected readonly isTapPending = signal(false);
  protected readonly digits = signal<readonly DialDigit[]>([]);
  protected readonly isPeeking = signal(false);
  protected readonly isOpeningDebrief = signal(false);
  protected readonly displayedAmount = signal<number | null>(null);
  protected readonly skipFlipAnimation = signal(false);

  private readonly currentPlayerId = computed(() => this.gameState.state()?.currentPlayerId ?? '');
  protected readonly seats = computed(() => this.round()?.seats ?? []);
  protected readonly mySeat = computed(() => this.seats().find((seat) => seat.playerId === this.currentPlayerId()) ?? null);
  protected readonly isParticipant = computed(() => this.round()?.isParticipant === true);
  protected readonly participantCount = computed(() => this.round()?.participantCount ?? 0);
  protected readonly isPlacing = computed(() => this.round()?.roundPhase === 'placing');
  protected readonly isRevealing = computed(() => this.round()?.roundPhase === 'revealing');
  protected readonly myColor = computed(() => seatColor(this.mySeat()?.colorIndex ?? 0));

  // --- placing -------------------------------------------------------------

  protected readonly duplicates = computed(() =>
    duplicateSeatNames(this.seats(), this.currentPlayerId(), this.localPosition()),
  );
  /** The server's lock countdown, hidden as soon as this phone has a tap of its own in flight. */
  protected readonly lockRemainingMs = computed(() => {
    const lockAt = this.round()?.lockAt;
    if (!lockAt || !this.isPlacing() || this.isTapPending()) {
      return null;
    }
    return Math.max(0, Date.parse(lockAt) - this.serverNow());
  });
  protected readonly lockProgress = computed(() => (this.lockRemainingMs() ?? 0) / lockDelayMs);
  protected readonly lockSeconds = computed(() => Math.ceil((this.lockRemainingMs() ?? 0) / 1000));
  protected readonly isNumberedLocally = computed(() =>
    isTableNumbered(this.seats(), this.currentPlayerId(), this.localPosition()),
  );
  protected readonly numberedCount = computed(() => {
    const myId = this.currentPlayerId();
    return this.seats().filter((seat) => (seat.playerId === myId ? this.localPosition() : seat.position)).length;
  });
  protected readonly showRenumberNotice = computed(() =>
    this.isPlacing() && !!this.round()?.positionsResetAt && (this.round()?.numberedCount ?? 0) < this.participantCount(),
  );

  // --- revealing -----------------------------------------------------------

  protected readonly reveal = computed(() => this.round()?.reveal ?? []);
  private readonly timeline = computed<RevealTimeline | null>(() => {
    const round = this.round();
    if (!round?.revealAt || !round.revealEndsAt || !round.revealStepMs || !round.reveal?.length) {
      return null;
    }
    return {
      revealAtMs: Date.parse(round.revealAt),
      stepMs: round.revealStepMs,
      count: round.reveal.length,
      endsAtMs: Date.parse(round.revealEndsAt),
    };
  });
  protected readonly myEntry = computed(() => this.reveal().find((entry) => entry.playerId === this.currentPlayerId()) ?? null);
  protected readonly stage = computed<RevealStage | null>(() => {
    const timeline = this.timeline();
    if (!timeline || !this.isRevealing()) {
      return null;
    }
    return revealStageAt(timeline, this.myEntry()?.position ?? null, this.serverNow());
  });
  protected readonly isFlipped = computed(() => {
    const stage = this.stage();
    return stage === 'flipped' || stage === 'verdict' || stage === 'over';
  });
  protected readonly isVerdict = computed(() => this.stage() === 'verdict' || this.stage() === 'over');
  protected readonly breaksLine = computed(() => {
    const entry = this.myEntry();
    return entry !== null && breaksLine(this.reveal(), entry.position);
  });
  protected readonly flippedCount = computed(() => {
    const timeline = this.timeline();
    return timeline ? revealedCount(timeline, this.serverNow()) : 0;
  });
  protected readonly isPerfect = computed(() => {
    const round = this.round();
    return !!round && round.participantCount > 0 && round.wellPlacedCount === round.participantCount;
  });
  protected readonly groupTitle = computed(() => groupTitle(this.round()?.wellPlacedCount ?? 0, this.round()?.participantCount ?? 0));
  protected readonly confetti = computed<readonly ConfettiPiece[]>(() => {
    const seed = (this.myEntry()?.position ?? 1) * 7919 + (this.round()?.roundNumber ?? 1) * 104729;
    return Array.from({ length: confettiCount }, (_, index) => {
      const random = (salt: number) => {
        const value = Math.sin(seed + index * 12.9898 + salt * 78.233) * 43758.5453;
        return value - Math.floor(value);
      };
      return {
        left: random(1) * 100,
        delayMs: random(2) * 600,
        durationMs: 1600 + random(3) * 1200,
        color: confettiColors[Math.floor(random(4) * confettiColors.length)],
        rotation: random(5) * 720 - 360,
      };
    });
  });
  /** The perfect-line gold wave runs from the left of the table to the right. */
  protected readonly goldWaveDelayMs = computed(() => ((this.myEntry()?.position ?? 1) - 1) * 120);

  protected readonly background = computed(() => {
    if (!this.isRevealing()) {
      return this.isParticipant() ? this.myColor() : 'var(--color-ink-900, #2a2022)';
    }
    if (this.isVerdict()) {
      if (!this.myEntry()) {
        return this.isPerfect() ? 'var(--color-gold-500, #e6ad3e)' : 'var(--color-ink-900, #2a2022)';
      }
      return this.myEntry()?.wellPlaced ? 'var(--color-success-500, #4e9c65)' : 'var(--color-danger-500, #d44758)';
    }
    return '#1d1426';
  });

  private tickInterval: ReturnType<typeof setInterval> | null = null;
  private tapTimer: ReturnType<typeof setTimeout> | null = null;
  private isSendingTap = false;
  private digitKey = 0;
  private countUpFrame: number | null = null;
  private previousStage: RevealStage | null = null;
  private announcedRound: number | null = null;

  constructor() {
    // Follow the server's number unless this phone has a tap of its own to send.
    effect(() => {
      const serverPosition = this.round()?.myPosition ?? null;
      if (!this.isTapPending()) {
        this.localPosition.set(serverPosition);
      }
    });
    effect(() => this.showDigit(this.localPosition()));
    // Arrival on the table: buzz once per round.
    effect(() => {
      const round = this.round();
      if (round?.roundPhase !== 'placing' || !round.roundNumber || round.roundNumber === this.announcedRound) {
        return;
      }
      this.announcedRound = round.roundNumber;
      if (round.isParticipant && round.numberedCount === 0) {
        vibrate([60, 40, 60]);
      }
    });
    effect(() => {
      const stage = this.stage();
      untracked(() => this.onStageChange(stage));
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
      return;
    }
    this.tick();
    this.tickInterval = setInterval(() => this.tick(), 100);
    await this.wakeLock.request();
  }

  ngOnDestroy(): void {
    if (this.tickInterval !== null) {
      clearInterval(this.tickInterval);
    }
    if (this.tapTimer !== null) {
      clearTimeout(this.tapTimer);
    }
    if (this.countUpFrame !== null) {
      cancelAnimationFrame(this.countUpFrame);
    }
    void this.wakeLock.release();
  }

  protected onTap(): void {
    if (!this.isPlacing() || !this.isParticipant()) {
      return;
    }
    this.localPosition.set(nextCyclePosition(this.localPosition(), this.participantCount()));
    this.isTapPending.set(true);
    vibrate(15);
    if (this.tapTimer !== null) {
      clearTimeout(this.tapTimer);
    }
    this.tapTimer = setTimeout(() => void this.sendPosition(), tapSendDelayMs);
  }

  protected onPeekStart(event: PointerEvent): void {
    event.preventDefault();
    this.isPeeking.set(true);
  }

  protected onPeekEnd(): void {
    this.isPeeking.set(false);
  }

  protected async onOpenDebrief(): Promise<void> {
    if (this.isOpeningDebrief()) {
      return;
    }
    this.isOpeningDebrief.set(true);
    try {
      await this.gameState.openSecretRankingDebrief();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isOpeningDebrief.set(false);
    }
  }

  private async sendPosition(): Promise<void> {
    this.tapTimer = null;
    const position = this.localPosition();
    if (position === null || this.isSendingTap) {
      // A send is in flight: it re-checks the latest tap when it returns.
      return;
    }
    this.isSendingTap = true;
    try {
      await this.gameState.setSecretRankingPosition(position);
    } catch {
      // GameState exposes the API error; the server's number wins again below.
    } finally {
      this.isSendingTap = false;
    }
    if (this.tapTimer !== null) {
      return;
    }
    if (this.localPosition() !== position && this.isPlacing()) {
      // Tapped again while the previous number was on its way.
      this.tapTimer = setTimeout(() => void this.sendPosition(), 0);
      return;
    }
    this.isTapPending.set(false);
  }

  /** The dial's odometer: the old number slides up and out while the new one comes in from below. */
  private showDigit(position: number | null): void {
    const value = position === null ? '–' : `${position}`;
    untracked(() => {
      const current = this.digits().find((digit) => !digit.leaving);
      if (current?.value === value) {
        return;
      }
      const key = ++this.digitKey;
      this.digits.set([
        ...(current ? [{ ...current, leaving: true }] : []),
        { key, value, leaving: false },
      ]);
      setTimeout(() => this.digits.update((digits) => digits.filter((digit) => !digit.leaving || digit.key > key)), digitLeaveMs);
    });
  }

  private tick(): void {
    this.serverNow.set(Date.now() + this.gameState.serverClockOffsetMs());
  }

  /**
   * Plays what happens on this phone at each step of the reveal: the flip with its count-up and buzz, then
   * the long buzz of a perfect line. A phone that arrives after its flip (reload, reconnection) shows the
   * amount straight away instead of replaying it.
   */
  private onStageChange(stage: RevealStage | null): void {
    const previous = this.previousStage;
    this.previousStage = stage;
    const entry = this.myEntry();
    const timeline = this.timeline();
    if (stage === previous) {
      return;
    }
    if (stage === 'verdict' && previous !== null && this.isPerfect()) {
      vibrate(200);
    }
    const isFlipStage = stage === 'flipped' || stage === 'verdict' || stage === 'over';
    if (!isFlipStage || !entry || !timeline) {
      this.displayedAmount.set(null);
      this.skipFlipAnimation.set(false);
      return;
    }
    if (this.displayedAmount() !== null) {
      // Already flipped on this phone: only the verdict changes.
      return;
    }
    const wasWaiting = previous === 'silence' || previous === 'drumroll';
    const flipAgeMs = this.serverNow() - revealSlotAt(timeline, entry.position);
    if (stage === 'flipped' && wasWaiting && flipAgeMs < liveFlipWindowMs) {
      this.skipFlipAnimation.set(false);
      vibrate(this.breaksLine() ? [80, 50, 80] : 40);
      this.countUp(entry.amount);
      return;
    }
    this.skipFlipAnimation.set(true);
    this.displayedAmount.set(entry.amount);
  }

  private countUp(amount: number): void {
    if (this.countUpFrame !== null) {
      cancelAnimationFrame(this.countUpFrame);
    }
    if (prefersReducedMotion()) {
      this.displayedAmount.set(amount);
      return;
    }
    const startedAt = performance.now();
    const step = (now: number) => {
      const progress = Math.min(1, (now - startedAt) / countUpDurationMs);
      const eased = 1 - Math.pow(1 - progress, 3);
      this.displayedAmount.set(Math.round(amount * eased));
      this.countUpFrame = progress < 1 ? requestAnimationFrame(step) : null;
    };
    this.displayedAmount.set(0);
    this.countUpFrame = requestAnimationFrame(step);
  }
}
