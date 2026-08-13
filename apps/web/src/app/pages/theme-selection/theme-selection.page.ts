import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { CdkDrag, CdkDragDrop, CdkDropList, moveItemInArray } from '@angular/cdk/drag-drop';
import { IonButton, IonContent, IonIcon, IonInput } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

/** Shared theme-collection-and-ranking screen, reused by every uncapped mode (Fast Bio, 0 à 100, Situation). */
@Component({
  selector: 'app-theme-selection-page',
  imports: [CdkDrag, CdkDropList, IonButton, IonContent, IonIcon, IonInput, PageHeaderComponent, ReactiveFormsModule],
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
  private rankingInitializedForCandidates: readonly string[] | null = null;
  /** What the current player just proposed, kept locally since the server only tracks a yes/no flag. Empty string means "passed". */
  protected readonly mySubmittedTheme = signal<string | null>(null);

  protected readonly remainingSeconds = signal(0);
  protected readonly countdownLabel = computed(() => {
    const totalSeconds = this.remainingSeconds();
    const minuteCount = Math.floor(totalSeconds / 60);
    const secondCount = totalSeconds % 60;
    return `${minuteCount.toString().padStart(2, '0')}:${secondCount.toString().padStart(2, '0')}`;
  });
  protected readonly isCountdownWarning = computed(() => this.remainingSeconds() <= 30);
  protected readonly hasDeadline = computed(() => !!this.gameState.themeSelectionState()?.themeDeadline);

  private countdownInterval: ReturnType<typeof setInterval> | null = null;

  protected readonly themeForm = new FormGroup({
    theme: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(80)] }),
  });

  constructor() {
    effect(() => {
      const candidates = this.gameState.themeSelectionState()?.themeCandidates;
      if (!candidates || candidates === this.rankingInitializedForCandidates) {
        return;
      }
      this.rankingInitializedForCandidates = candidates;
      this.ranking.set(candidates);
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

  protected async onSkipTheme(): Promise<void> {
    if (this.isSubmittingTheme()) {
      return;
    }
    this.isSubmittingTheme.set(true);
    try {
      await this.gameState.submitTheme('');
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
