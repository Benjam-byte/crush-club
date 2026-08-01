import { Injectable, signal } from '@angular/core';
import type { DefaultPhotoCandidate, PhotoCandidate } from '../models/game.models';

const requiredPhotoCount = 4;

const defaultPhotoList: readonly DefaultPhotoCandidate[] = [0, 1, 2, 3].map(
  (avatarIndex) => ({
    id: `default-photo-${avatarIndex}`,
    kind: 'default',
    atlas: 'camille',
    avatarIndex,
    isObjectUrl: false,
  }),
);

@Injectable({
  providedIn: 'root',
})
export class PhotoService {
  private readonly photoPreviewListState = signal<readonly PhotoCandidate[]>([]);

  readonly photoPreviewList = this.photoPreviewListState.asReadonly();

  loadPhotoList(fileList: FileList): string | null {
    const selectedFileList = Array.from(fileList);
    const hasInvalidFile = selectedFileList.some((file) => !file.type.startsWith('image/'));

    if (hasInvalidFile) {
      return 'Choisis uniquement des fichiers image.';
    }

    if (selectedFileList.length !== requiredPhotoCount) {
      return 'Sélectionne exactement quatre photos.';
    }

    this.revokeObjectUrlList();
    const photoPreviewList = selectedFileList.map((file, index) => {
      return {
        id: `local-photo-${index}`,
        kind: 'local' as const,
        source: URL.createObjectURL(file),
        isObjectUrl: true as const,
      };
    });
    this.photoPreviewListState.set(photoPreviewList);

    return null;
  }

  useDefaultPhotoList(): void {
    this.revokeObjectUrlList();
    this.photoPreviewListState.set(defaultPhotoList);
  }

  clearPhotoList(): void {
    this.revokeObjectUrlList();
    this.photoPreviewListState.set([]);
  }

  private revokeObjectUrlList(): void {
    for (const photoPreview of this.photoPreviewList()) {
      if (photoPreview.isObjectUrl) {
        URL.revokeObjectURL(photoPreview.source);
      }
    }
  }
}
