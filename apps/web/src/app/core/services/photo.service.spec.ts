import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  isAcceptedPhotoFile,
  normalizedPhotoDimensions,
  PhotoPreparationError,
  PhotoService,
} from './photo.service';

const heicMocks = vi.hoisted(() => ({
  isHeic: vi.fn(),
  heicTo: vi.fn(),
}));

vi.mock('heic-to/csp', () => heicMocks);

describe('photo helpers', () => {
  it.each([
    ['portrait.jpg', 'image/jpeg', true],
    ['portrait.HEIC', '', true],
    ['portrait.heif', 'image/heif', true],
    ['animation.gif', 'image/gif', false],
    ['animation.jpg', 'image/gif', false],
    ['document.pdf', 'application/pdf', false],
  ])('valide %s avec le type %s', (name, type, expected) => {
    expect(isAcceptedPhotoFile({ name, type })).toBe(expected);
  });

  it('limite le bord le plus long à 2048 pixels', () => {
    expect(normalizedPhotoDimensions(4032, 3024)).toEqual({ width: 2048, height: 1536 });
    expect(normalizedPhotoDimensions(1200, 1600)).toEqual({ width: 1200, height: 1600 });
  });
});

describe('PhotoService', () => {
  const createObjectUrl = vi.fn(() => 'blob:prepared-photo');
  const revokeObjectUrl = vi.fn();
  const closeBitmap = vi.fn();
  const drawImage = vi.fn();
  const fillRect = vi.fn();
  const toBlob = vi.fn((callback: BlobCallback) => callback(new Blob(['jpeg'], { type: 'image/jpeg' })));
  const canvas = {
    width: 0,
    height: 0,
    getContext: vi.fn(() => ({
      fillStyle: '',
      fillRect,
      drawImage,
    })),
    toBlob,
  };

  beforeEach(() => {
    createObjectUrl.mockClear();
    revokeObjectUrl.mockClear();
    closeBitmap.mockClear();
    drawImage.mockClear();
    fillRect.mockClear();
    toBlob.mockClear();
    toBlob.mockImplementation((callback: BlobCallback) => callback(new Blob(['jpeg'], { type: 'image/jpeg' })));
    canvas.width = 0;
    canvas.height = 0;
    heicMocks.isHeic.mockReset();
    heicMocks.heicTo.mockReset();
    vi.stubGlobal('URL', {
      createObjectURL: createObjectUrl,
      revokeObjectURL: revokeObjectUrl,
    });
    vi.stubGlobal('document', {
      createElement: vi.fn(() => canvas),
    });
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({
      width: 4032,
      height: 3024,
      close: closeBitmap,
    })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('normalise une photo standard en JPEG orienté et redimensionné', async () => {
    const service = new PhotoService();
    const input = new File(['source'], 'portrait.png', { type: 'image/png' });

    const result = await service.preparePhoto(input);

    expect(result.file.name).toBe('portrait-crush-club.jpg');
    expect(result.file.type).toBe('image/jpeg');
    expect(result.previewUrl).toBe('blob:prepared-photo');
    expect(canvas.width).toBe(2048);
    expect(canvas.height).toBe(1536);
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 0, 0, 2048, 1536);
    expect(closeBitmap).toHaveBeenCalledOnce();
  });

  it('convertit un fichier HEIC avant sa normalisation', async () => {
    const service = new PhotoService();
    const input = new File(['heic'], 'IMG_0001.HEIC', { type: 'image/heic' });
    const converted = { width: 4032, height: 3024, close: closeBitmap };
    heicMocks.isHeic.mockResolvedValue(true);
    heicMocks.heicTo.mockResolvedValue(converted);

    await service.preparePhoto(input);

    expect(heicMocks.isHeic).toHaveBeenCalledWith(input);
    expect(heicMocks.heicTo).toHaveBeenCalledWith({
      blob: input,
      type: 'bitmap',
      options: { imageOrientation: 'from-image' },
    });
    expect(createImageBitmap).not.toHaveBeenCalled();
    expect(closeBitmap).toHaveBeenCalledOnce();
  });

  it('réduit la qualité si le premier JPEG dépasse 7 Mo', async () => {
    const service = new PhotoService();
    const largeBlob = new Blob([new Uint8Array((7 << 20) + 1)], { type: 'image/jpeg' });
    toBlob
      .mockImplementationOnce((callback: BlobCallback) => callback(largeBlob))
      .mockImplementationOnce((callback: BlobCallback) => callback(new Blob(['small'], { type: 'image/jpeg' })));

    await service.preparePhoto(new File(['source'], 'large.jpg', { type: 'image/jpeg' }));

    expect(toBlob).toHaveBeenCalledTimes(2);
    expect(toBlob.mock.calls[0]?.[2]).toBe(0.85);
    expect(toBlob.mock.calls[1]?.[2]).toBe(0.75);
  });

  it('refuse une photo qui reste supérieure à 7 Mo', async () => {
    const service = new PhotoService();
    const largeBlob = new Blob([new Uint8Array((7 << 20) + 1)], { type: 'image/jpeg' });
    toBlob.mockImplementation((callback: BlobCallback) => callback(largeBlob));

    await expect(
      service.preparePhoto(new File(['source'], 'large.jpg', { type: 'image/jpeg' })),
    ).rejects.toThrow('trop volumineuse');
    expect(toBlob).toHaveBeenCalledTimes(3);
    expect(closeBitmap).toHaveBeenCalledOnce();
  });

  it('signale une image endommagée et libère son URL de décodage', async () => {
    const service = new PhotoService();
    vi.stubGlobal('createImageBitmap', vi.fn(async () => Promise.reject(new Error('broken'))));
    class BrokenImage {
      naturalWidth = 0;
      naturalHeight = 0;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;

      set src(_value: string) {
        this.onerror?.();
      }
    }
    vi.stubGlobal('Image', BrokenImage);
    createObjectUrl.mockReturnValueOnce('blob:decode-source');

    await expect(
      service.preparePhoto(new File(['broken'], 'broken.jpg', { type: 'image/jpeg' })),
    ).rejects.toThrow('illisible ou endommagée');
    expect(revokeObjectUrl).toHaveBeenCalledWith('blob:decode-source');
  });

  it('refuse les formats non pris en charge avant le décodage', async () => {
    const service = new PhotoService();

    await expect(
      service.preparePhoto(new File(['gif'], 'animation.gif', { type: 'image/gif' })),
    ).rejects.toBeInstanceOf(PhotoPreparationError);
    expect(createImageBitmap).not.toHaveBeenCalled();
  });

  it('révoque les aperçus individuellement puis globalement', async () => {
    const service = new PhotoService();
    createObjectUrl.mockReturnValueOnce('blob:first').mockReturnValueOnce('blob:second');
    const first = await service.preparePhoto(new File(['one'], 'one.jpg', { type: 'image/jpeg' }));
    await service.preparePhoto(new File(['two'], 'two.jpg', { type: 'image/jpeg' }));

    service.revokePreviewUrl(first.previewUrl);
    service.clearPreviewUrls();

    expect(revokeObjectUrl).toHaveBeenCalledWith('blob:first');
    expect(revokeObjectUrl).toHaveBeenCalledWith('blob:second');
    expect(revokeObjectUrl).toHaveBeenCalledTimes(2);
  });
});
