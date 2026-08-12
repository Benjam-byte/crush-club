import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon, IonTextarea } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';
import { PhotoPreparationError, PhotoService } from '@core/services/photo.service';

@Component({
  selector: 'app-fast-bio-assignment-page',
  imports: [IonButton, IonContent, IonIcon, IonTextarea, PageHeaderComponent, ReactiveFormsModule],
  templateUrl: './fast-bio-assignment.page.html',
  styleUrl: './fast-bio-assignment.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FastBioAssignmentPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  protected readonly photoService = inject(PhotoService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmitting = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly selectedPreviewUrl = signal<string | null>(null);
  protected readonly remainingSeconds = signal(0);
  protected readonly countdownLabel = computed(() => {
    const totalSeconds = this.remainingSeconds();
    const minuteCount = Math.floor(totalSeconds / 60);
    const secondCount = totalSeconds % 60;
    return `${minuteCount.toString().padStart(2, '0')}:${secondCount.toString().padStart(2, '0')}`;
  });
  protected readonly isCountdownWarning = computed(() => this.remainingSeconds() <= 30);

  protected readonly bioForm = new FormGroup({
    bio: new FormControl('', {
      nonNullable: true,
      validators: [Validators.required, Validators.minLength(1), Validators.maxLength(280)],
    }),
  });

  private selectedFile: File | null = null;
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
    if (this.selectedPreviewUrl()) {
      this.photoService.revokePreviewUrl(this.selectedPreviewUrl() ?? undefined);
    }
  }

  protected onPickPhoto(inputElement: HTMLInputElement): void {
    if (this.isSubmitting()) {
      return;
    }
    inputElement.click();
  }

  protected async onPhotoInputChange(event: Event): Promise<void> {
    const inputElement = event.target as HTMLInputElement;
    const file = inputElement.files?.item(0) ?? null;
    inputElement.value = '';
    if (!file) {
      return;
    }
    this.errorMessage.set(null);
    try {
      const prepared = await this.photoService.preparePhoto(file);
      const previousPreview = this.selectedPreviewUrl();
      if (previousPreview) {
        this.photoService.revokePreviewUrl(previousPreview);
      }
      this.selectedFile = prepared.file;
      this.selectedPreviewUrl.set(prepared.previewUrl);
    } catch (error: unknown) {
      const message = error instanceof PhotoPreparationError
        ? error.message
        : 'Cette photo n’a pas pu être préparée. Réessaie.';
      this.errorMessage.set(message);
    }
  }

  protected async onSubmit(): Promise<void> {
    if (this.isSubmitting() || this.bioForm.invalid || !this.selectedFile) {
      return;
    }
    this.isSubmitting.set(true);
    this.errorMessage.set(null);
    try {
      await this.gameState.submitFastBioProposal(this.selectedFile, this.bioForm.controls.bio.value.trim());
    } catch {
      this.errorMessage.set(this.gameState.errorMessage() ?? 'Ta proposition n’a pas pu être envoyée.');
    } finally {
      this.isSubmitting.set(false);
    }
  }

  private refreshCountdown(): void {
    const deadline = this.gameState.fastBioGame()?.submissionDeadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const remainingMs = new Date(deadline).getTime() - Date.now();
    this.remainingSeconds.set(Math.max(0, Math.ceil(remainingMs / 1000)));
  }
}
