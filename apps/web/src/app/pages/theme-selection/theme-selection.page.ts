import { ChangeDetectionStrategy, Component, effect, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon, IonInput } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

/** Shared theme-collection-and-ranking screen, reused by every uncapped mode (Fast Bio, 0 à 100). */
@Component({
  selector: 'app-theme-selection-page',
  imports: [IonButton, IonContent, IonIcon, IonInput, PageHeaderComponent, ReactiveFormsModule],
  templateUrl: './theme-selection.page.html',
  styleUrl: './theme-selection.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ThemeSelectionPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmittingTheme = signal(false);
  protected readonly isSubmittingRanking = signal(false);
  protected readonly ranking = signal<readonly string[]>([]);
  private rankingInitializedForCandidates: readonly string[] | null = null;

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
    }
  }

  protected async onSubmitTheme(): Promise<void> {
    if (this.isSubmittingTheme() || this.themeForm.invalid) {
      return;
    }
    this.isSubmittingTheme.set(true);
    try {
      await this.gameState.submitTheme(this.themeForm.controls.theme.value.trim());
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
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmittingTheme.set(false);
    }
  }

  protected moveUp(index: number): void {
    if (index <= 0) {
      return;
    }
    this.ranking.update((current) => {
      const next = [...current];
      [next[index - 1], next[index]] = [next[index], next[index - 1]];
      return next;
    });
  }

  protected moveDown(index: number): void {
    this.ranking.update((current) => {
      if (index >= current.length - 1) {
        return current;
      }
      const next = [...current];
      [next[index], next[index + 1]] = [next[index + 1], next[index]];
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
}
