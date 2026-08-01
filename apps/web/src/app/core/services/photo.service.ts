import { Injectable, signal } from '@angular/core';
import type { PhotoCandidate } from '../models/game.models';

const requiredPhotoCount = 4;

@Injectable({
  providedIn: 'root',
})
export class PhotoService {
  private readonly photoPreviewListState = signal<readonly PhotoCandidate[]>([]);
  private readonly photoFileListState = signal<readonly File[]>([]);

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
    this.photoFileListState.set(selectedFileList);
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

  orderedFileList(photoIndexList: readonly number[]): readonly File[] {
    const fileList = this.photoFileListState();
    return photoIndexList.flatMap((index) => fileList[index] ? [fileList[index]] : []);
  }

  clearPhotoList(): void {
    this.revokeObjectUrlList();
    this.photoPreviewListState.set([]);
    this.photoFileListState.set([]);
  }

  private revokeObjectUrlList(): void {
    for (const photoPreview of this.photoPreviewList()) {
      if (photoPreview.isObjectUrl) {
        URL.revokeObjectURL(photoPreview.source);
      }
    }
  }
}
