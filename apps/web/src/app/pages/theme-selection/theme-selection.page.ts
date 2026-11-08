import { ChangeDetectionStrategy, Component, computed, effect, inject, signal, untracked } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { CdkDrag, CdkDropList, moveItemInArray } from '@angular/cdk/drag-drop';
import type { CdkDragDrop } from '@angular/cdk/drag-drop';
import { IonButton, IonContent, IonIcon, IonInput } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { PhaseConfirmationComponent } from '@core/components/phase-confirmation/phase-confirmation.component';
import { PhaseTimerComponent } from '@core/components/phase-timer/phase-timer.component';
import { SecretRankingRulesComponent } from '@core/components/secret-ranking-rules/secret-ranking-rules.component';
import { WaitingGameComponent } from '@core/components/waiting-game/waiting-game.component';
import { ZeroToHundredScoringRulesComponent } from '@core/components/zero-to-100-scoring-rules/zero-to-100-scoring-rules.component';
import { GameStateService } from '@core/services/game-state.service';
import { secretRankingQuestion, secretRankingQuestionEnd, secretRankingQuestionStart } from '@core/services/secret-ranking';
import { wavelengthProposalHint, wavelengthRankingHint, wavelengthRoundsLabel } from './wavelength-vote-copy';

/**
 * Shared theme-collection-and-ranking screen, reused by every uncapped mode (Fast Bio, 0 à 100, Situation,
 * Wavelength, Classement secret). The secret ranking's themes complete "Pour combien d'argent accepterais-tu
 * de … ?". Wavelength votes on two-pole spectra instead of themes, and comes back here once the up to
 * three spectra of its last vote have been played.
 */
@Component({
  selector: 'app-theme-selection-page',
  imports: [
    CdkDrag,
    CdkDropList,
    IonButton,
    IonContent,
    IonIcon,
    IonInput,
    PageHeaderComponent,
    PhaseConfirmationComponent,
    PhaseTimerComponent,
    ReactiveFormsModule,
    SecretRankingRulesComponent,
    WaitingGameComponent,
    ZeroToHundredScoringRulesComponent,
  ],
  templateUrl: './theme-selection.page.html',
  styleUrl: './theme-selection.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ThemeSelectionPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmittingTheme = signal(false);
  protected readonly isSubmittingRanking = signal(false);
  protected readonly ranking = signal<readonly string[]>([]);
  /** What the current player just proposed, kept locally since the server only tracks a yes/no flag. Empty string means "passed". */
  protected readonly mySubmittedTheme = signal<string | null>(null);

  protected readonly remainingSeconds = signal(0);
  protected readonly showCountdown = computed(() => {
    const selection = this.gameState.themeSelectionState();
    if (!selection?.themeDeadline) {
      return false;
    }
    if (selection.phase === 'collecting_themes') {
      return !selection.themeSubmitted;
    }
    if (selection.phase === 'ranking_themes') {
      return !selection.themeRanked;
    }
    return false;
  });

  private countdownInterval: ReturnType<typeof setInterval> | null = null;

  protected readonly themeForm = new FormGroup({
    theme: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(80)] }),
  });
  protected readonly spectrumForm = new FormGroup({
    left: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(40)] }),
    right: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(40)] }),
  });

  protected readonly pageTitle = computed(() => {
    if (this.gameState.isZeroToHundredMode()) {
      return 'Thèmes 0 à 100';
    }
    if (this.gameState.isSituationMode()) {
      return 'Situations';
    }
    if (this.gameState.isWavelengthMode()) {
      return 'Wavelength';
    }
    if (this.gameState.isSecretRankingMode()) {
      return 'Classement secret';
    }
    return 'Thèmes Fast Bio';
  });
  /** The rounds the current Wavelength vote prepares, shown above each step. */
  private readonly wavelengthVote = computed(() => {
    const wavelength = this.gameState.wavelengthGame();
    return wavelength
      ? { roundNumber: wavelength.roundNumber ?? 1, totalRounds: wavelength.totalRounds ?? 1, spectraToPlay: wavelength.spectraToPlay ?? 1 }
      : null;
  });
  protected readonly wavelengthRoundLabel = computed(() => {
    const vote = this.wavelengthVote();
    return vote ? wavelengthRoundsLabel(vote.roundNumber, vote.totalRounds, vote.spectraToPlay) : null;
  });
  protected readonly wavelengthProposalHint = computed(() => wavelengthProposalHint(this.wavelengthVote()?.spectraToPlay ?? 1));
  protected readonly wavelengthRankingHint = computed(() => {
    const vote = this.wavelengthVote();
    return vote ? wavelengthRankingHint(vote.roundNumber, vote.totalRounds, vote.spectraToPlay) : '';
  });
  protected readonly secretRankingQuestionStart = secretRankingQuestionStart;
  protected readonly secretRankingQuestion = secretRankingQuestion;
  protected readonly secretRankingQuestionEnd = secretRankingQuestionEnd;
  /** 0 à 100 and the secret ranking explain their rules while players wait, instead of the mini-game alone. */
  protected readonly showsRulesWhileWaiting = computed(() =>
    this.gameState.isZeroToHundredMode() || this.gameState.isSecretRankingMode(),
  );
  protected readonly rankingSentMessage = computed(() => {
    if (!this.gameState.isWavelengthMode()) {
      return 'La première manche démarrera automatiquement.';
    }
    return (this.wavelengthVote()?.spectraToPlay ?? 1) > 1
      ? 'La première de ces manches démarrera automatiquement.'
      : 'La manche démarrera automatiquement.';
  });

  constructor() {
    // Every realtime push carries a fresh candidates array: reconcile by content so
    // another player's validation never resets the order this player is building.
    effect(() => {
      const candidates = this.gameState.themeSelectionState()?.themeCandidates;
      if (!candidates) {
        return;
      }
      const current = untracked(this.ranking);
      const kept = current.filter((theme) => candidates.includes(theme));
      const added = candidates.filter((theme) => !kept.includes(theme));
      if (added.length === 0 && kept.length === current.length) {
        return;
      }
      this.ranking.set([...kept, ...added]);
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
      return;
    }
    this.refreshCountdown();
    this.countdownInterval = setInterval(() => this.refreshCountdown(), 250);
  }

  ngOnDestroy(): void {
    if (this.countdownInterval !== null) {
      clearInterval(this.countdownInterval);
    }
  }

  protected async onSubmitTheme(): Promise<void> {
    if (this.isSubmittingTheme() || this.themeForm.invalid) {
      return;
    }
    const theme = this.themeForm.controls.theme.value.trim();
    this.isSubmittingTheme.set(true);
    try {
      await this.gameState.submitTheme(theme);
      this.mySubmittedTheme.set(theme);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmittingTheme.set(false);
    }
  }

  protected isSpectrumComplete(): boolean {
    const { left, right } = this.spectrumForm.getRawValue();
    return left.trim() !== '' && right.trim() !== '';
  }

  protected async onSubmitSpectrum(): Promise<void> {
    if (this.isSubmittingTheme() || this.spectrumForm.invalid || !this.isSpectrumComplete()) {
      return;
    }
    const left = this.spectrumForm.controls.left.value.trim();
    const right = this.spectrumForm.controls.right.value.trim();
    this.isSubmittingTheme.set(true);
    try {
      await this.gameState.submitWavelengthTheme(left, right);
      this.mySubmittedTheme.set(`${left} ↔ ${right}`);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmittingTheme.set(false);
    }
  }

  protected async onSkipTheme(): Promise<void> {
    if (this.isSubmittingTheme()) {
      return;
    }
    this.isSubmittingTheme.set(true);
    try {
      if (this.gameState.isWavelengthMode()) {
        await this.gameState.submitWavelengthTheme('', '');
      } else {
        await this.gameState.submitTheme('');
      }
      this.mySubmittedTheme.set('');
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmittingTheme.set(false);
    }
  }

  protected onDropTheme(event: CdkDragDrop<readonly string[]>): void {
    this.ranking.update((current) => {
      const next = [...current];
      moveItemInArray(next, event.previousIndex, event.currentIndex);
      return next;
    });
  }

  protected async onSubmitRanking(): Promise<void> {
    if (this.isSubmittingRanking() || this.ranking().length === 0) {
      return;
    }
    this.isSubmittingRanking.set(true);
    try {
      await this.gameState.rankThemes(this.ranking());
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmittingRanking.set(false);
    }
  }

  private refreshCountdown(): void {
    const deadline = this.gameState.themeSelectionState()?.themeDeadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const remainingMs = new Date(deadline).getTime() - Date.now();
    this.remainingSeconds.set(Math.max(0, Math.ceil(remainingMs / 1000)));
  }
}
