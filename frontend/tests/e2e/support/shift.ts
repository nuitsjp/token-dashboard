import type { Page } from '@playwright/test';
import type { FrameRequest } from '../../../bindings/token-monitor-turzx/internal/display/models';

export type PNGFrame = { request: FrameRequest; png: string; device: string };

// Compare every device pixel against the unshifted preview, undoing only the device's rotation.
// The preview's corner is also the expected fill for every clipped row/column.
export async function checkPNGShift(page: Page, frames: PNGFrame[]) {
  return page.evaluate(async frames => {
    const decode = async (base64: string) => {
      const image = new Image(); image.src = `data:image/png;base64,${base64}`;
      await image.decode();
      const canvas = document.createElement('canvas'); canvas.width = image.width; canvas.height = image.height;
      const context = canvas.getContext('2d')!; context.drawImage(image, 0, 0);
      return { width: image.width, height: image.height, data: context.getImageData(0, 0, image.width, image.height).data };
    };
    for (const frame of frames) {
      const [preview, device] = await Promise.all([decode(frame.png), decode(frame.device)]);
      const r = frame.request;
      if (preview.width !== r.width || preview.height !== r.height || device.width !== r.width || device.height !== r.height) throw new Error('Wrong PNG dimensions');
      for (let y = 0; y < r.height; y++) for (let x = 0; x < r.width; x++) {
        const sx = x - r.shiftX, sy = y - r.shiftY;
        const source = sx < 0 || sy < 0 || sx >= r.width || sy >= r.height ? 0 : (sy * r.width + sx) * 4;
        const target = r.reverse180 ? ((r.height - 1 - y) * r.width + r.width - 1 - x) * 4 : (y * r.width + x) * 4;
        for (let channel = 0; channel < 4; channel++) {
          if (preview.data[source + channel] !== device.data[target + channel]) throw new Error(`Pixel mismatch: ${r.theme}, reverse=${r.reverse180}, shift=${r.shiftX},${r.shiftY}, pixel=${x},${y}`);
        }
      }
    }
  }, frames);
}

// The CompleteFrame arguments of the resident renderer: the PNG for the preview and the JPEG for the TURZX.
// measure finds the shift of each JPEG against its PNG: the offset that best moves the PNG onto the JPEG
// turned back clockwise, and the colour where the shift leaves a gap (display pixel 1,1).
export type Frame = { png: string; jpeg: string };

// The shift of each frame, and the colour of the JPEG where the shift leaves a gap (display pixel 1,1).
export async function measure(page: Page, frames: Frame[]) {
  return page.evaluate(async frames => {
    const decode = async (type: string, data: string) => {
      const image = new Image();
      image.src = `data:image/${type};base64,${data}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = image.width;
      canvas.height = image.height;
      const context = canvas.getContext('2d')!;
      context.drawImage(image, 0, 0);
      return { width: image.width, height: image.height, data: context.getImageData(0, 0, image.width, image.height).data };
    };
    const result: { shift: number[]; gap: number[] }[] = [];
    for (const frame of frames) {
      const [png, jpeg] = await Promise.all([decode('png', frame.png), decode('jpeg', frame.jpeg)]);
      // The display pixel (x, y) is the JPEG pixel (461 - y, x): the JPEG is turned clockwise.
      const shown = (x: number, y: number, channel: number) => jpeg.data[(x * jpeg.width + (jpeg.width - 1 - y)) * 4 + channel];
      let best = [0, 0];
      let least = Infinity;
      for (let dx = -6; dx <= 6; dx++) for (let dy = -6; dy <= 6; dy++) {
        let error = 0;
        for (let y = 20; y < png.height - 20; y += 6) for (let x = 20; x < png.width - 20; x += 6) {
          const source = ((y - dy) * png.width + (x - dx)) * 4;
          error += Math.abs(png.data[source] - shown(x, y, 0)) + Math.abs(png.data[source + 1] - shown(x, y, 1));
        }
        if (error < least) { least = error; best = [dx, dy]; }
      }
      result.push({ shift: best, gap: [0, 1, 2].map(channel => shown(1, 1, channel)) });
    }
    return result;
  }, frames);
}
