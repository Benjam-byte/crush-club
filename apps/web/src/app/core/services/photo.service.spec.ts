import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PhotoService } from './photo.service';

function createFileList(fileCount: number, type = 'image/jpeg'): FileList {
  const fileList = Array.from({ length: fileCount }, (_, index) => {
    return { name: `photo-${index}.jpg`, type } as File;
  });

  return Object.assign(fileList, {
    item: (index: number) => fileList[index] ?? null,
  }) as unknown as FileList;
}

describe('PhotoService', () => {
  const createObjectUrl = vi.fn((file: File) => `blob:${file.name}`);
  const revokeObjectUrl = vi.fn();

  beforeEach(() => {
    createObjectUrl.mockClear();
    revokeObjectUrl.mockClear();
    vi.stubGlobal('URL', {
      createObjectURL: createObjectUrl,
      revokeObjectURL: revokeObjectUrl,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([3, 5])('refuse une sélection de %i photos', (fileCount) => {
    const service = new PhotoService();

    expect(service.loadPhotoList(createFileList(fileCount))).toBe(
      'Sélectionne exactement quatre photos.',
    );
    expect(service.photoPreviewList()).toEqual([]);
    expect(createObjectUrl).not.toHaveBeenCalled();
  });

  it('charge exactement quatre photos locales', () => {
    const service = new PhotoService();

    expect(service.loadPhotoList(createFileList(4))).toBeNull();
    expect(service.photoPreviewList()).toHaveLength(4);
    expect(service.photoPreviewList().every((photo) => photo.kind === 'local')).toBe(true);
  });

  it('conserve une sélection valide après une tentative invalide', () => {
    const service = new PhotoService();
    service.loadPhotoList(createFileList(4));
    const validPhotoList = service.photoPreviewList();

    service.loadPhotoList(createFileList(2));

    expect(service.photoPreviewList()).toBe(validPhotoList);
    expect(revokeObjectUrl).not.toHaveBeenCalled();
  });

  it('remplace les photos locales par les quatre portraits par défaut', () => {
    const service = new PhotoService();
    service.loadPhotoList(createFileList(4));

    service.useDefaultPhotoList();

    expect(revokeObjectUrl).toHaveBeenCalledTimes(4);
    expect(service.photoPreviewList()).toEqual([
      expect.objectContaining({ kind: 'default', avatarIndex: 0 }),
      expect.objectContaining({ kind: 'default', avatarIndex: 1 }),
      expect.objectContaining({ kind: 'default', avatarIndex: 2 }),
      expect.objectContaining({ kind: 'default', avatarIndex: 3 }),
    ]);
  });
});
