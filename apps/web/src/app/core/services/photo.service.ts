import { Injectable } from '@angular/core';

const acceptedImageMimeTypes = new Set([
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/heic',
  'image/heif',
  'image/heic-sequence',
  'image/heif-sequence',
]);
const acceptedImageExtensions = new Set(['jpg', 'jpeg', 'png', 'webp', 'heic', 'heif']);
const heicImageMimeTypes = new Set([
  'image/heic',
  'image/heif',
  'image/heic-sequence',
  'image/heif-sequence',
]);
const maximumOutputDimension = 2048;
const maximumOutputSizeBytes = 7 << 20;
const jpegQualityList = [0.85, 0.75, 0.65] as const;

interface DecodedImage {
  source: CanvasImageSource
  width: number
  height: number
  dispose: () => void
}

export interface PreparedPhoto {
  file: File
  previewUrl: string
}

export class PhotoPreparationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'PhotoPreparationError';
  }
}

export function normalizedPhotoDimensions(width: number, height: number): {
  width: number
  height: number
} {
  const longestSide = Math.max(width, height);
  if (longestSide <= maximumOutputDimension) {
    return { width, height };
  }
  const scale = maximumOutputDimension / longestSide;
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  };
}

export function isAcceptedPhotoFile(file: Pick<File, 'name' | 'type'>): boolean {
  const mimeType = file.type.toLowerCase();
  if (acceptedImageMimeTypes.has(mimeType)) {
    return true;
  }
  if (mimeType !== '') {
    return false;
  }
  const extension = file.name.split('.').pop()?.toLowerCase() ?? '';
  return acceptedImageExtensions.has(extension);
}

@Injectable({
  providedIn: 'root',
})
export class PhotoService {
  private readonly previewUrlSet = new Set<string>();

  async preparePhoto(file: File): Promise<PreparedPhoto> {
    if (!isAcceptedPhotoFile(file)) {
      throw new PhotoPreparationError('Choisis une image JPEG, PNG, WebP, HEIC ou HEIF.');
    }

    let decodedImage: DecodedImage | null = null;
    if (this.isHeicCandidate(file)) {
      try {
        const { heicTo, isHeic } = await import('heic-to/csp');
        if (await isHeic(file)) {
          const bitmap = await heicTo({
            blob: file,
            type: 'bitmap',
            options: { imageOrientation: 'from-image' },
          });
          if (bitmap.width <= 0 || bitmap.height <= 0) {
            bitmap.close();
            throw new Error('Invalid HEIC dimensions');
          }
          decodedImage = {
            source: bitmap,
            width: bitmap.width,
            height: bitmap.height,
            dispose: () => bitmap.close(),
          };
        }
      } catch {
        throw new PhotoPreparationError(
          'Cette photo HEIC n’a pas pu être convertie. Essaie une autre photo ou exporte-la en JPEG.',
        );
      }
    }

    decodedImage ??= await this.decodeImage(file);
    try {
      const dimensions = normalizedPhotoDimensions(decodedImage.width, decodedImage.height);
      const canvas = document.createElement('canvas');
      canvas.width = dimensions.width;
      canvas.height = dimensions.height;
      const context = canvas.getContext('2d');
      if (!context) {
        throw new PhotoPreparationError('Ton navigateur ne peut pas préparer cette photo.');
      }
      context.fillStyle = '#ffffff';
      context.fillRect(0, 0, dimensions.width, dimensions.height);
      context.drawImage(decodedImage.source, 0, 0, dimensions.width, dimensions.height);

      let outputBlob: Blob | null = null;
      for (const quality of jpegQualityList) {
        outputBlob = await this.canvasToBlob(canvas, quality);
        if (outputBlob.size <= maximumOutputSizeBytes) {
          break;
        }
      }
      if (!outputBlob || outputBlob.size > maximumOutputSizeBytes) {
        throw new PhotoPreparationError('Cette photo reste trop volumineuse après compression.');
      }

      const outputName = `${this.fileBaseName(file.name)}-crush-club.jpg`;
      const preparedFile = new File([outputBlob], outputName, {
        type: 'image/jpeg',
        lastModified: Date.now(),
      });
      const previewUrl = URL.createObjectURL(preparedFile);
      this.previewUrlSet.add(previewUrl);
      return { file: preparedFile, previewUrl };
    } finally {
      decodedImage.dispose();
    }
  }

  revokePreviewUrl(previewUrl: string | undefined): void {
    if (!previewUrl || !this.previewUrlSet.delete(previewUrl)) {
      return;
    }
    URL.revokeObjectURL(previewUrl);
  }

  clearPreviewUrls(): void {
    for (const previewUrl of this.previewUrlSet) {
      URL.revokeObjectURL(previewUrl);
    }
    this.previewUrlSet.clear();
  }

  private isHeicCandidate(file: File): boolean {
    const extension = file.name.split('.').pop()?.toLowerCase() ?? '';
    return heicImageMimeTypes.has(file.type.toLowerCase()) || extension === 'heic' || extension === 'heif';
  }

  private async decodeImage(blob: Blob): Promise<DecodedImage> {
    if (typeof createImageBitmap === 'function') {
      try {
        const bitmap = await createImageBitmap(blob, { imageOrientation: 'from-image' });
        if (bitmap.width > 0 && bitmap.height > 0) {
          return {
            source: bitmap,
            width: bitmap.width,
            height: bitmap.height,
            dispose: () => bitmap.close(),
          };
        }
        bitmap.close();
      } catch {
        // Older Safari versions use the HTML image fallback below.
      }
    }

    const sourceUrl = URL.createObjectURL(blob);
    const image = new Image();
    try {
      await new Promise<void>((resolve, reject) => {
        image.onload = () => resolve();
        image.onerror = () => reject(new Error('Image decode failed'));
        image.src = sourceUrl;
      });
      if (image.naturalWidth <= 0 || image.naturalHeight <= 0) {
        throw new Error('Invalid image dimensions');
      }
      return {
        source: image,
        width: image.naturalWidth,
        height: image.naturalHeight,
        dispose: () => URL.revokeObjectURL(sourceUrl),
      };
    } catch {
      URL.revokeObjectURL(sourceUrl);
      throw new PhotoPreparationError('Cette image est illisible ou endommagée.');
    }
  }

  private canvasToBlob(canvas: HTMLCanvasElement, quality: number): Promise<Blob> {
    return new Promise((resolve, reject) => {
      canvas.toBlob((blob) => {
        if (blob) {
          resolve(blob);
          return;
        }
        reject(new PhotoPreparationError('La conversion de cette photo a échoué.'));
      }, 'image/jpeg', quality);
    });
  }

  private fileBaseName(fileName: string): string {
    const withoutExtension = fileName.replace(/\.[^.]+$/, '').trim();
    return withoutExtension || 'photo';
  }
}
