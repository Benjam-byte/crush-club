import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon, IonTextarea } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-situation-propose-page',
  imports: [IonButton, IonContent, IonIcon, IonTextarea, PageHeaderComponent, ReactiveFormsModule],
  templateUrl: './situation-propose.page.html',
  styleUrl: './situation-propose.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SituationProposePage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmitting = signal(false);
  protected readonly selectedPlayerId = signal<string | null>(null);
  protected readonly remainingSeconds = signal(0);
  protected readonly countdownLabel = computed(() => {
    const totalSeconds = this.remainingSeconds();
    const minuteCount = Math.floor(totalSeconds / 60);
    const secondCount = totalSeconds % 60;
    return `${minuteCount.toString().padStart(2, '0')}:${secondCount.toString().padStart(2, '0')}`;
  });
  protected readonly isCountdownWarning = computed(() => this.remainingSeconds() <= 30);
  protected readonly otherPlayers = computed(() =>
    this.gameState.playerList().filter((player) => !player.isCurrentPlayer),
  );

  protected readonly reasonForm = new FormGroup({
    reason: new FormControl('', {
      nonNullable: true,
      validators: [Validators.required, Validators.minLength(1), Validators.maxLength(100)],
    }),
  });

  private countdownInterval: ReturnType<typeof setInterval> | null = null;

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

  protected onSelectPlayer(playerId: string): void {
    if (this.isSubmitting()) {
      return;
    }
    this.selectedPlayerId.set(playerId);
  }

  protected async onSubmit(): Promise<void> {
    const chosenPlayerId = this.selectedPlayerId();
    if (this.isSubmitting() || this.reasonForm.invalid || !chosenPlayerId) {
      return;
    }
    this.isSubmitting.set(true);
    try {
      await this.gameState.submitSituationProposal(chosenPlayerId, this.reasonForm.controls.reason.value.trim());
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmitting.set(false);
    }
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }

  private refreshCountdown(): void {
    const deadline = this.gameState.situationGame()?.proposalDeadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const remainingMs = new Date(deadline).getTime() - Date.now();
    this.remainingSeconds.set(Math.max(0, Math.ceil(remainingMs / 1000)));
  }
}
