import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  signal,
} from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';
import { PhotoPreparationError, PhotoService } from '../../core/services/photo.service';
import { PhotoSelectionTimer, photoSelectionDurationMs } from './photo-selection-timer';

const requiredPhotoCount = 4;

type PhotoOperationStage = 'preparing' | 'uploading' | 'deleting';

@Component({
  selector: 'app-photos-page',
  imports: [
    IonButton,
    IonContent,
    IonIcon,
    PageHeaderComponent,
  ],
  templateUrl: './photos.page.html',
  styleUrl: './photos.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PhotosPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  protected readonly photoService = inject(PhotoService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly remainingSelectionSeconds = signal(photoSelectionDurationMs / 1000);
  protected readonly selectionExpired = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly pendingPosition = signal<number | null>(null);
  protected readonly operationStage = signal<PhotoOperationStage | null>(null);
  protected readonly isFinalizing = signal(false);
  protected readonly slotIndexList = [0, 1, 2, 3] as const;
  protected readonly currentPhotoIdList = computed(() => this.gameState.currentPlayer().photoIds);
  protected readonly photoCount = computed(() => this.currentPhotoIdList().length);
  protected readonly isBusy = computed(() => this.pendingPosition() !== null || this.isFinalizing());
  protected readonly canComplete = computed(() => {
    return this.photoCount() === requiredPhotoCount &&
      !this.isBusy() &&
      this.gameState.isRealtimeConnected();
  });
  protected readonly countdownLabel = computed(() => {
    const remainingSeconds = this.remainingSelectionSeconds();
    const minuteCount = Math.floor(remainingSeconds / 60);
    const secondCount = remainingSeconds % 60;
    return `${minuteCount.toString().padStart(2, '0')}:${secondCount.toString().padStart(2, '0')}`;
  });
  protected readonly isCountdownWarning = computed(() => {
    return this.photoCount() < requiredPhotoCount && this.remainingSelectionSeconds() <= 30;
  });

  private readonly localPreviewUrlByPhotoId = signal<Readonly<Record<string, string>>>({});
  private readonly selectionTimer = new PhotoSelectionTimer();
  private targetPosition: number | null = null;

  private readonly onVisibilityChange = (): void => {
    if (typeof document === 'undefined' || document.visibilityState !== 'visible') {
      return;
    }
    this.selectionTimer.refresh();
  };

  async ngOnInit(): Promise<void> {
    const routeCode = this.route.snapshot.paramMap.get('code') ?? undefined;
    try {
      await this.gameState.refreshLobby(routeCode);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: routeCode } });
      return;
    }
    if (this.gameState.currentPlayer().readyStatus === 'ready') {
      await this.router.navigate(['/lobby', this.gameState.lobbyCode()]);
      return;
    }

    this.photoService.clearPreviewUrls();
    if (this.photoCount() < requiredPhotoCount) {
      this.selectionTimer.start(
        (remainingSeconds) => this.remainingSelectionSeconds.set(remainingSeconds),
        () => this.expirePhotoSelection(),
      );
      if (typeof document !== 'undefined') {
        document.addEventListener('visibilitychange', this.onVisibilityChange);
      }
    }
  }

  ngOnDestroy(): void {
    this.stopSelectionTimer();
    this.photoService.clearPreviewUrls();
  }

  protected onPickPhoto(position: number, inputElement: HTMLInputElement): void {
    if (this.isBusy() || position < 1 || position > this.photoCount() + 1) {
      return;
    }
    this.targetPosition = position;
    inputElement.click();
  }

  protected async onPhotoInputChange(event: Event): Promise<void> {
    const inputElement = event.target as HTMLInputElement;
    const file = inputElement.files?.item(0) ?? null;
    const position = this.targetPosition;
    inputElement.value = '';
    this.targetPosition = null;
    this.selectionTimer.refresh();
    if (!file || position === null || this.isBusy()) {
      return;
    }

    const previousPhotoIdList = [...this.currentPhotoIdList()];
    const existingPhotoId = previousPhotoIdList[position - 1];
    let preparedPreviewUrl: string | undefined;
    this.errorMessage.set(null);
    this.pendingPosition.set(position);
    this.operationStage.set('preparing');
    try {
      const preparedPhoto = await this.photoService.preparePhoto(file);
      preparedPreviewUrl = preparedPhoto.previewUrl;
      this.operationStage.set('uploading');
      if (existingPhotoId) {
        await this.gameState.replacePhoto(position, preparedPhoto.file);
      } else if (position === previousPhotoIdList.length + 1) {
        await this.gameState.addPhoto(preparedPhoto.file);
      } else {
        throw new PhotoPreparationError('Ajoute les photos dans l’ordre, une par une.');
      }

      const savedPhotoId = this.currentPhotoIdList()[position - 1];
      if (!savedPhotoId) {
        throw new Error('Uploaded photo missing from refreshed state');
      }
      this.rememberLocalPreview(savedPhotoId, preparedPreviewUrl, existingPhotoId);
      preparedPreviewUrl = undefined;
      if (this.photoCount() === requiredPhotoCount) {
        this.stopSelectionTimer();
      }
    } catch (error: unknown) {
      this.photoService.revokePreviewUrl(preparedPreviewUrl);
      const message = error instanceof PhotoPreparationError
        ? error.message
        : this.gameState.errorMessage() ?? 'Cette photo n’a pas pu être envoyée. Réessaie.';
      await this.reconcileAfterFailure();
      this.errorMessage.set(message);
    } finally {
      this.pendingPosition.set(null);
      this.operationStage.set(null);
    }
  }

  protected async onDeletePhoto(position: number): Promise<void> {
    const deletedPhotoId = this.currentPhotoIdList()[position - 1];
    if (!deletedPhotoId || this.isBusy()) {
      return;
    }
    this.errorMessage.set(null);
    this.pendingPosition.set(position);
    this.operationStage.set('deleting');
    try {
      await this.gameState.deletePhoto(position);
      this.forgetLocalPreview(deletedPhotoId);
    } catch {
      const message = this.gameState.errorMessage() ?? 'Cette photo n’a pas pu être supprimée.';
      await this.reconcileAfterFailure();
      this.errorMessage.set(message);
    } finally {
      this.pendingPosition.set(null);
      this.operationStage.set(null);
    }
  }

  protected async onComplete(): Promise<void> {
    if (!this.canComplete()) {
      return;
    }
    this.isFinalizing.set(true);
    this.errorMessage.set(null);
    try {
      await this.gameState.completePhotos();
      this.photoService.clearPreviewUrls();
      await this.router.navigate(['/lobby', this.gameState.lobbyCode()]);
    } catch {
      this.errorMessage.set(this.gameState.errorMessage() ?? 'Tes photos n’ont pas pu être validées.');
    } finally {
      this.isFinalizing.set(false);
    }
  }

  protected photoSource(photoId: string | undefined): string | null {
    if (!photoId) {
      return null;
    }
    return this.localPreviewUrlByPhotoId()[photoId] ?? this.gameState.photoUrl(photoId);
  }

  protected operationLabel(position: number): string | null {
    if (this.pendingPosition() !== position) {
      return null;
    }
    switch (this.operationStage()) {
      case 'preparing':
        return 'Préparation…';
      case 'uploading':
        return 'Envoi…';
      case 'deleting':
        return 'Suppression…';
      default:
        return null;
    }
  }

  private rememberLocalPreview(photoId: string, previewUrl: string, replacedPhotoId?: string): void {
    this.localPreviewUrlByPhotoId.update((current) => {
      const next = { ...current };
      if (replacedPhotoId) {
        this.photoService.revokePreviewUrl(next[replacedPhotoId]);
        delete next[replacedPhotoId];
      }
      next[photoId] = previewUrl;
      return next;
    });
  }

  private forgetLocalPreview(photoId: string): void {
    this.localPreviewUrlByPhotoId.update((current) => {
      const previewUrl = current[photoId];
      if (!previewUrl) {
        return current;
      }
      this.photoService.revokePreviewUrl(previewUrl);
      const next = { ...current };
      delete next[photoId];
      return next;
    });
  }

  private async reconcileAfterFailure(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.gameState.lobbyCode());
    } catch {
      // Keep the actionable operation error displayed; realtime may reconnect independently.
    }
  }

  private expirePhotoSelection(): void {
    this.stopSelectionTimer();
    this.selectionExpired.set(true);
  }

  private stopSelectionTimer(): void {
    this.selectionTimer.stop();
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', this.onVisibilityChange);
    }
  }
}
