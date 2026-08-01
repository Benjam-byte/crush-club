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
import { PhotoService } from '../../core/services/photo.service';
import { PhotoSelectionTimer, photoSelectionDurationMs } from './photo-selection-timer';

const requiredPhotoCount = 4;

type PhotoSelectionPhase = 'pending' | 'local';

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

  protected readonly selectedPhotoIndexList = signal<number[]>([]);
  protected readonly selectedSlotIndex = signal(0);
  protected readonly selectionPhase = signal<PhotoSelectionPhase>('pending');
  protected readonly remainingSelectionSeconds = signal(photoSelectionDurationMs / 1000);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly isUploading = signal(false);
  protected readonly slotIndexList = [0, 1, 2, 3] as const;
  protected readonly countdownLabel = computed(() => {
    const remainingSeconds = this.remainingSelectionSeconds();
    const minuteCount = Math.floor(remainingSeconds / 60);
    const secondCount = remainingSeconds % 60;
    return `${minuteCount.toString().padStart(2, '0')}:${secondCount.toString().padStart(2, '0')}`;
  });
  protected readonly isCountdownWarning = computed(() => {
    return this.selectionPhase() === 'pending' && this.remainingSelectionSeconds() <= 30;
  });

  private readonly selectionTimer = new PhotoSelectionTimer();
  private hasValidated = false;

  private readonly onVisibilityChange = (): void => {
    if (typeof document === 'undefined' || document.visibilityState !== 'visible') {
      return;
    }

    this.selectionTimer.refresh();
  };

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
      return;
    }
    this.photoService.clearPhotoList();
    this.selectionTimer.start(
      (remainingSeconds) => this.remainingSelectionSeconds.set(remainingSeconds),
      () => this.expirePhotoSelection(),
    );

    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', this.onVisibilityChange);
    }
  }

  ngOnDestroy(): void {
    this.stopSelectionTimer();
    this.photoService.clearPhotoList();
  }

  protected onPhotoInputChange(event: Event): void {
    const inputElement = event.target as HTMLInputElement;
    this.selectionTimer.refresh();

    if (this.selectionPhase() !== 'pending') {
      inputElement.value = '';
      return;
    }

    if (inputElement.files === null || inputElement.files.length === 0) {
      return;
    }

    const errorMessage = this.photoService.loadPhotoList(inputElement.files);
    inputElement.value = '';
    this.errorMessage.set(errorMessage);

    if (errorMessage !== null) {
      return;
    }

    this.selectedPhotoIndexList.set([0, 1, 2, 3]);
    this.selectedSlotIndex.set(0);
    this.selectionPhase.set('local');
    this.stopSelectionTimer();
  }

  protected onSlotSelect(slotIndex: number): void {
    this.selectedSlotIndex.set(slotIndex);
  }

  protected onMoveSelectedPhoto(direction: -1 | 1): void {
    const currentIndex = this.selectedSlotIndex();
    const destinationIndex = currentIndex + direction;

    if (destinationIndex < 0 || destinationIndex >= requiredPhotoCount) {
      return;
    }

    this.selectedPhotoIndexList.update((selectedPhotoIndexList) => {
      const nextPhotoIndexList = [...selectedPhotoIndexList];
      const currentPhotoIndex = nextPhotoIndexList[currentIndex];
      const destinationPhotoIndex = nextPhotoIndexList[destinationIndex];

      if (currentPhotoIndex === undefined || destinationPhotoIndex === undefined) {
        return selectedPhotoIndexList;
      }

      nextPhotoIndexList[currentIndex] = destinationPhotoIndex;
      nextPhotoIndexList[destinationIndex] = currentPhotoIndex;
      return nextPhotoIndexList;
    });
    this.selectedSlotIndex.set(destinationIndex);
  }

  protected async onValidate(): Promise<void> {
    if (
      this.hasValidated ||
      this.selectionPhase() === 'pending' ||
      this.selectedPhotoIndexList().length !== requiredPhotoCount
    ) {
      return;
    }

    this.hasValidated = true;
    this.stopSelectionTimer();
    this.isUploading.set(true);
    this.errorMessage.set(null);
    try {
      const orderedPhotoList = this.photoService.orderedFileList(this.selectedPhotoIndexList());
      await this.gameState.uploadPhotos(orderedPhotoList);
      this.photoService.clearPhotoList();
      await this.router.navigate(['/lobby', this.gameState.lobbyCode()]);
    } catch {
      this.hasValidated = false;
      this.errorMessage.set(this.gameState.errorMessage() ?? 'Les photos n’ont pas pu être envoyées.');
    } finally {
      this.isUploading.set(false);
    }
  }

  private expirePhotoSelection(): void {
    if (this.selectionPhase() !== 'pending') {
      return;
    }

    this.stopSelectionTimer();
    this.errorMessage.set('Le délai indicatif est écoulé, mais tu peux toujours choisir tes quatre photos.');
  }

  private stopSelectionTimer(): void {
    this.selectionTimer.stop();

    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', this.onVisibilityChange);
    }
  }
}
