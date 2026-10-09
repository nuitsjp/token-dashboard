import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { selectWideDisplay, startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening
// the window, closing it for hiding the window, and stopping the process for Exit. The image is
// checked by the colours at fixed points of the 1920x462 layout.
const background = [15, 17, 23];
const line = [42, 47, 58]; // the divider and the empty part of a bar
const normal = [116, 102, 224]; // violet
const danger = [240, 97, 109];
const caution = [250, 178, 25];
const points = {
  divider: [356, 231], // between Tokens and Usage Limits
  alphaBarStart: [389, 124], // lowest remaining, so the first contract of the first column
  alphaBarMiddle: [560, 124],
  betaBarStart: [389, 268], // the next contract, stacked under the first
  betaBarEnd: [730, 268],
  secondColumn: [800, 124],
  firstColumnTop: [389, 124], // the first bar of each column's first contract
  firstColumnSecond: [389, 268], // the first bar of a contract under a one-window contract
  secondColumnTop: [770, 124],
  fourthColumnTop: [1532, 124],
} as const;

const token = 'e2e-hub-token';
// 4 seconds past the hour, so the shown minutes change 4 seconds after sending.
const hours = (h: number) => new Date(Date.now() + h * 3_600_000 + 4_000).toISOString();
function stats(alphaRemaining: number) {
  return {
    periods: { today: { totalTokens: 1234567, costUsd: 1.23 }, month: { totalTokens: 23456789, costUsd: 23.45 }, allTime: { totalTokens: 345678901, costUsd: 345.67 } },
    limits: { providers: [
      { provider: 'alpha', planLabel: 'Pro', windows: [
        { kind: 'session', label: '', showMeter: true, remainingPercent: alphaRemaining, usedPercent: 100 - alphaRemaining, resetsAt: hours(2) },
        { kind: 'billing', label: 'Credits', showMeter: false, remainingPercent: null, usedPercent: null, resetsAt: null },
      ] },
      { provider: 'gamma', accountLabel: 'Balance only', windows: [{ kind: 'billing', label: 'Credits', showMeter: false }] },
      { provider: 'beta', planLabel: 'Max', windows: [{ kind: 'weekly', label: 'Weekly', showMeter: true, remainingPercent: 80, usedPercent: 20, resetsAt: hours(50) }] },
    ] },
  };
}

// Contracts in Hub order whose lowest remaining percents put them in a different order on screen.
function ranked() {
  const contract = (provider: string, ...remaining: (number | null)[]) => ({
    provider, planLabel: 'Pro',
    windows: remaining.map((r, i) => ({ kind: 'weekly', label: `w${i}`, showMeter: true, remainingPercent: r, usedPercent: r === null ? null : 100 - r, resetsAt: hours(50) })),
  });
  return {
    ...stats(12),
    limits: { providers: [
      contract('four', 90, 95, 100, 100), contract('one', 10), contract('three', 30, 60, 90),
      contract('unknown', null), contract('late', 95, 95, 95, 95), contract('last', 99, 99, 99, 99),
    ] },
  };
}

async function preview(page: Page) {
  const image = page.getByRole('img', { name: 'Display preview' });
  await expect(image).toBeVisible();
  return (await image.getAttribute('src')) ?? '';
}

// Returns the RGB colour of each point of the image in src.
async function colours(page: Page, src: string) {
  return page.evaluate(async ({ src, points }) => {
    const img = new Image();
    img.src = src;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext('2d')!;
    context.drawImage(img, 0, 0);
    const result: Record<string, number[]> = { size: [img.naturalWidth, img.naturalHeight] };
    for (const [name, [x, y]] of Object.entries(points)) result[name] = [...context.getImageData(x, y, 1, 1).data.slice(0, 3)];
    return result;
  }, { src, points });
}

// The tokens of Today, Month and All, below each label and cost in the Tokens column.
const tokenRows = { today: [0, 75, 350, 110], month: [0, 215, 350, 250], all: [0, 355, 350, 390] } as const;

// Returns the leftmost and rightmost x of the pixels in each rect [left, top, right, bottom]
// that differ from the background.
async function extents(page: Page, src: string) {
  return page.evaluate(async ({ src, rects }) => {
    const img = new Image();
    img.src = src;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext('2d')!;
    context.drawImage(img, 0, 0);
    const result: Record<string, number[]> = {};
    for (const [name, [left, top, right, bottom]] of Object.entries(rects)) {
      const data = context.getImageData(left, top, right - left, bottom - top).data;
      let min = Infinity;
      let max = -Infinity;
      for (let i = 0; i < data.length; i += 4) {
        if (data[i] === 15 && data[i + 1] === 17 && data[i + 2] === 23) continue;
        const x = left + ((i / 4) % (right - left));
        min = Math.min(min, x);
        max = Math.max(max, x);
      }
      result[name] = [min, max];
    }
    return result;
  }, { src, rects: tokenRows });
}

// Compare the actual two encodings sent by the resident renderer, allowing JPEG compression.
async function rotationError(page: Page, png: string, jpeg: string) {
  return page.evaluate(async ({ png, jpeg }) => {
    const decode = async (type: string, data: string) => {
      const image = new Image();
      image.src = `data:image/${type};base64,${data}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = image.width;
      canvas.height = image.height;
      const context = canvas.getContext('2d')!;
      context.drawImage(image, 0, 0);
      return { width: image.width, height: image.height, pixels: context.getImageData(0, 0, image.width, image.height).data };
    };
    const [original, rotated] = await Promise.all([decode('png', png), decode('jpeg', jpeg)]);
    let clockwise = 0;
    let counterclockwise = 0;
    for (let y = 0; y < original.height; y++) for (let x = 0; x < original.width; x++) {
      const source = (y * original.width + x) * 4;
      const cw = (x * rotated.width + original.height - 1 - y) * 4;
      const ccw = ((original.width - 1 - x) * rotated.width + y) * 4;
      for (let channel = 0; channel < 3; channel++) {
        clockwise += Math.abs(original.pixels[source + channel] - rotated.pixels[cw + channel]);
        counterclockwise += Math.abs(original.pixels[source + channel] - rotated.pixels[ccw + channel]);
      }
    }
    const count = original.width * original.height * 3;
    return { size: [original.width, original.height, rotated.width, rotated.height], clockwise: clockwise / count, counterclockwise: counterclockwise / count };
  }, { png, jpeg });
}

test('実描画のJPEGがPNGと時計回り90度で対応し、描画要求の取得失敗後も更新を再開する', async ({ page }) => {
  test.setTimeout(60_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-canvas-e2e-'));
  selectWideDisplay(dataDir);
  const hub = await startHub();
  const server = await startServer(dataDir, 34131, shortIntervals);
  const frames = new Map<string, { png: string; jpeg: string }>();
  let assetsFailed = false;
  let failRequest = false;
  let injected = false;
  await page.route('**/wails/runtime', async route => {
    const body = route.request().postDataJSON();
    const method = body?.args?.methodName;
    if (!assetsFailed && method === 'token-monitor-turzx/internal/display.Service.AgentIcons') {
      assetsFailed = true;
      await route.fulfill({ status: 503, contentType: 'text/plain', body: 'Injected transient asset RPC failure' });
    } else if (failRequest && !injected && method === 'token-monitor-turzx/internal/display.Service.RenderRequest') {
      injected = true;
      await route.fulfill({ status: 503, contentType: 'text/plain', body: 'Injected transient renderer RPC failure' });
    } else await route.continue();
  });
  page.on('request', request => {
    if (!request.url().endsWith('/wails/runtime') || request.method() !== 'POST') return;
    const body = request.postDataJSON();
    if (body?.args?.methodName !== 'token-monitor-turzx/internal/display.Service.CompleteFrame') return;
    const [, png, jpeg, error] = body.args.args as [number, string, string, string];
    if (error || !png || !jpeg) return;
    frames.set(`data:image/png;base64,${png}`, { png, jpeg });
    while (frames.size > 8) frames.delete(frames.keys().next().value!);
  });
  const checkRotation = async () => {
    await expect.poll(async () => {
      const frame = frames.get(await preview(page));
      if (!frame) return 0;
      return (await rotationError(page, frame.png, frame.jpeg)).counterclockwise;
    }).toBeGreaterThan(5);
    const frame = frames.get(await preview(page))!;
    const result = await rotationError(page, frame.png, frame.jpeg);
    expect(result.size).toEqual([1920, 462, 462, 1920]);
    expect(result.clockwise).toBeLessThan(5);
    expect(result.counterclockwise).toBeGreaterThan(result.clockwise + 2);
  };
  try {
    await page.goto(server.url);
    await page.getByRole('link', { name: 'Connection' }).click();
    await page.getByRole('textbox', { name: 'Data source' }).click();
    await page.getByRole('option', { name: 'Hub' }).click();
    await page.getByLabel('Hub URL').fill(hub.url);
    await page.getByLabel(/Access token/).fill(token);
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Saved.')).toBeVisible();
    await expect.poll(() => hub.streams()).toBe(1);
    hub.send('snapshot', stats(12));
    await page.getByRole('link', { name: 'Display' }).click();
    await checkRotation();
    expect(assetsFailed).toBe(true);

    failRequest = true;
    await expect.poll(() => injected).toBe(true);
    const previous = await preview(page);
    await page.getByRole('textbox', { name: 'Display style' }).click();
    await page.getByRole('option', { name: 'Bars' }).click();
    expect(await preview(page)).toBe(previous);
    await expect.poll(async () => (await colours(page, await preview(page))).divider, { timeout: 20_000 }).toEqual(line);
    await checkRotation();
  } finally {
    await page.close();
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});

test('Display style を Bars に切り替えると、保存して Tokens の列と横棒の利用枠で描き直す', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-bars-e2e-'));
  selectWideDisplay(dataDir);
  const hub = await startHub();
  // The server redraws every second instead of every minute.
  let server = await startServer(dataDir, 34123, shortIntervals);
  // The page is replaced after the window is closed, so the locator is made when it is used.
  const style = () => page.getByRole('textbox', { name: 'Display style' });
  const file = () => JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
  try {
    await test.step('分岐条件', async () => {
      // The Hub connection is saved and a snapshot is shown. No style is saved, so the image is Gauges.
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Hub' }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel(/Access token/).fill(token);
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats(12));
      await page.getByRole('link', { name: 'Display' }).click();
    });
    let requests = 0;
    await test.step('手順1', async () => {
      // Output sits at the right end of the row of the page heading, above the Style card.
      // Measure one layout snapshot: the asynchronous update notice can move the whole page
      // between separate browser calls without changing the headings' relative placement.
      const headings = await page.getByRole('heading').evaluateAll(elements => Object.fromEntries(elements.map(element => {
        const { x, y, width, height } = element.getBoundingClientRect();
        return [element.textContent, { x, y, width, height }];
      })));
      const title = headings['Display settings'];
      const style1 = headings.Style;
      const output1 = headings.Output;
      expect(Math.abs(output1!.y - title!.y)).toBeLessThan(title!.height);
      expect(output1!.x).toBeGreaterThan(title!.x + title!.width);
      expect(output1!.y).toBeLessThan(style1!.y);
      await expect(style()).toHaveValue('Gauges');
      await expect.poll(async () => (await preview(page)).length).toBeGreaterThan(5000);
      const gauges = await preview(page);
      const shown = await colours(page, gauges);
      expect(shown.alphaBarStart).not.toEqual(danger);
      requests = hub.requests.length;
    });
    await test.step('手順2', async () => {
      // The choice is applied at once: no Save button, no message, and the Hub is not interrupted.
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
      await style().click();
      await page.getByRole('option', { name: 'Bars' }).click();
      await expect(page.getByText('Saved.')).toHaveCount(0);
      // A snapshot or timed redraw can change the image before the style is applied.
      await expect.poll(async () => colours(page, await preview(page))).toMatchObject({
        divider: line,
        alphaBarStart: danger,
        alphaBarMiddle: line,
        betaBarStart: normal,
        betaBarEnd: line,
      });
      await expect.poll(() => file().limitStyle).toBe('Bars');
      expect(hub.requests.length).toBe(requests);
      expect(hub.streams()).toBe(1);
    });
    await test.step('手順3', async () => {
      // A new usage from the Hub is drawn in the same style.
      hub.send('stats', stats(60));
      await expect.poll(async () => (await colours(page, await preview(page))).alphaBarMiddle).toEqual(normal);
      expect((await colours(page, await preview(page))).alphaBarStart).toEqual(normal);
      await expect(style()).toHaveValue('Bars');
    });
    await test.step('手順4', async () => {
      await page.close();
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34123, shortIntervals);
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats(12));
      page = await context.newPage();
      await page.goto(server.url);
      await expect(style()).toHaveValue('Bars');
      await expect.poll(async () => (await colours(page, await preview(page))).divider).toEqual(line);
      expect((await colours(page, await preview(page))).alphaBarStart).toEqual(danger);
    });
    await test.step('受け入れ条件', async () => {
      // Only Gauges and Bars can be chosen.
      await style().click();
      await expect(page.getByRole('option')).toHaveText(['Gauges', 'Bars']);
      await page.keyboard.press('Escape');
      // Taken before the shown minutes change, to see the image drawn again after that.
      const current = await preview(page);
      const shown = await colours(page, current);
      expect(shown.size).toEqual([1920, 462]);
      // Contracts without a meter take no column, and alpha and beta share the first one.
      expect(shown.secondColumn).toEqual(background);
      // The tokens end at the right edge of the cost (x = 330), so their widths differ on the left.
      for (const [left, right] of Object.values(await extents(page, await preview(page)))) {
        expect(right).toBeGreaterThanOrEqual(320);
        expect(right).toBeLessThan(331);
        expect(left).toBeGreaterThan(60);
      }
      // The image is drawn again without anything from the Hub when the shown minutes change.
      await expect.poll(() => preview(page), { timeout: 10_000, intervals: [250] }).not.toBe(current);
      await expect(page.getByText(token)).toHaveCount(0);
      // Lowest remaining first: one (10%) and three (30%) share the first column, four (90%),
      // late (95%) and last (99%) take the next three, and unknown, reporting nothing, finds no room.
      hub.send('stats', ranked());
      // The earlier image already shows danger at the top of the first column, so wait for the caution.
      await expect.poll(async () => (await colours(page, await preview(page))).firstColumnSecond).toEqual(caution);
      const ordered = await colours(page, await preview(page));
      expect(ordered.firstColumnTop).toEqual(danger);
      expect(ordered.secondColumnTop).toEqual(normal);
      expect(ordered.fourthColumnTop).toEqual(normal);
      // Choosing Gauges again saves it, draws the gauges and leaves the connection as it was.
      await style().click();
      await page.getByRole('option', { name: 'Gauges' }).click();
      await expect.poll(async () => (await colours(page, await preview(page))).firstColumnTop).not.toEqual(danger);
      await expect.poll(() => file().limitStyle).toBe('Gauges');
      expect(file().source).toBe('Hub');
      expect(file().connection).toBeTruthy();
    });
  } finally {
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
