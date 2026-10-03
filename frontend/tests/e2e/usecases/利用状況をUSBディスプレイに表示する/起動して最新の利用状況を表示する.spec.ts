import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening
// the window, closing it for hiding the window, and stopping the process for Exit. The image is
// checked by the colours in areas of the 1920x462 layout: the arcs of the gauges are violet (normal),
// yellow (caution) or red (danger), and the panels are a darker fill on the background.
type Rgb = readonly [number, number, number];
const background: Rgb = [15, 17, 23];
const panelFill: Rgb = [21, 24, 33];
const normal: Rgb = [144, 133, 233]; // violet
const danger: Rgb = [240, 97, 109];
const caution: Rgb = [250, 178, 25];
// [left, top, right, bottom] of the first panel (one circle) and of the second panel of ranked().
const firstPanel = [40, 86, 300, 432] as const;
const secondPanel = [314, 86, 1102, 432] as const;
const tokensStrip = [40, 30, 1880, 66] as const;
const wholeImage = [0, 0, 1920, 462] as const;
const panelRow = 90; // just below the top edge of the panels, where only the fill is

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
// Each window has its own label, so each gets its own circle.
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

// Counts the pixels of exactly the colour in the area, and the runs of that colour along one row.
async function look(page: Page, src: string, area: readonly [number, number, number, number], rgb: Rgb, row = panelRow) {
  return page.evaluate(async ({ src, area, rgb, row }) => {
    const img = new Image();
    img.src = src;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext('2d')!;
    context.drawImage(img, 0, 0);
    const [left, top, right, bottom] = area;
    const same = (d: Uint8ClampedArray, i: number) => d[i] === rgb[0] && d[i + 1] === rgb[1] && d[i + 2] === rgb[2];
    const data = context.getImageData(left, top, right - left, bottom - top).data;
    let count = 0;
    for (let i = 0; i < data.length; i += 4) if (same(data, i)) count++;
    const line = context.getImageData(0, row, img.naturalWidth, 1).data;
    let runs = 0;
    for (let x = 0; x < img.naturalWidth; x++) if (same(line, x * 4) && (x === 0 || !same(line, (x - 1) * 4))) runs++;
    return { count, runs, size: [img.naturalWidth, img.naturalHeight] };
  }, { src, area, rgb, row });
}

// The number of pixels that differ from the background in the area.
async function inked(page: Page, src: string, area: readonly [number, number, number, number]) {
  const all = await look(page, src, area, background);
  return (area[2] - area[0]) * (area[3] - area[1]) - all.count;
}

test('起動して、Hub の最新の利用状況をゲージでプレビューに表示し続ける', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-usage-e2e-'));
  const hub = await startHub();
  // The server redraws every second instead of every minute.
  let server = await startServer(dataDir, 34117, shortIntervals);
  try {
    await test.step('開始条件', async () => {
      // Precondition: the Hub connection is saved. Then the app starts as at sign-in.
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Hub' }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel(/Access token/).fill(token);
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34117, shortIntervals);
    });
    await test.step('手順1', async () => {
      await expect.poll(() => hub.streams()).toBe(1);
      const request = hub.requests.at(-1)!;
      expect(request.path).toBe('/api/stats/stream');
      expect(request.headers.authorization).toBe(`Bearer ${token}`);
      expect(request.headers['x-token-monitor-stream']).toBe('2');
      await page.goto(server.url);
      const waiting = await preview(page);
      // Waiting for Hub: no panel, no arc and no tokens.
      expect((await look(page, waiting, wholeImage, panelFill)).runs).toBe(0);
      expect((await look(page, waiting, wholeImage, normal)).count).toBe(0);
      expect(await inked(page, waiting, tokensStrip)).toBe(0);
    });
    let first = '';
    await test.step('手順2', async () => {
      const before = await preview(page);
      hub.send('snapshot', stats(12));
      await expect.poll(() => preview(page)).not.toBe(before);
      first = await preview(page);
      // Tokens run across the top. Below them, alpha (12% left, so danger) and beta (80%, normal)
      // each have a panel; gamma has no meter and takes none.
      expect(await inked(page, first, tokensStrip)).toBeGreaterThan(500);
      expect((await look(page, first, wholeImage, panelFill)).runs).toBe(2);
      expect((await look(page, first, firstPanel, danger)).count).toBeGreaterThan(100);
      expect((await look(page, first, secondPanel, normal)).count).toBeGreaterThan(100);
      expect((await look(page, first, secondPanel, danger)).count).toBe(0);
    });
    await test.step('手順3', async () => {
      await page.close();
      page = await context.newPage();
      await page.goto(server.url);
      expect(await preview(page)).toBe(first);
    });
    await test.step('手順4', async () => {
      hub.send('stats', stats(60));
      // 60% left of a window that is 40% over is on pace and above 40%: normal, so no danger remains.
      await expect.poll(async () => (await look(page, await preview(page), firstPanel, danger)).count).toBe(0);
      expect((await look(page, await preview(page), firstPanel, normal)).count).toBeGreaterThan(100);
    });
    await test.step('手順5', async () => {
      await page.close();
      hub.send('stats', stats(12));
      await expect.poll(() => hub.streams()).toBe(1);
      page = await context.newPage();
      await page.goto(server.url);
      await expect.poll(async () => (await look(page, await preview(page), firstPanel, danger)).count).toBeGreaterThan(100);
    });
    await test.step('手順6', async () => {
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34117, shortIntervals);
    });
    await test.step('受け入れ条件', async () => {
      await page.goto(server.url);
      // Nothing received is kept across a restart: the new process waits for the Hub again.
      expect((await look(page, await preview(page), wholeImage, panelFill)).runs).toBe(0);
      hub.send('snapshot', stats(12));
      await expect.poll(async () => (await look(page, await preview(page), wholeImage, panelFill)).runs).toBe(2);
      // Taken before the shown minutes change, to see the image drawn again after that.
      const current = await preview(page);
      expect((await look(page, current, wholeImage, panelFill)).size).toEqual([1920, 462]);
      // The image is drawn again without anything from the Hub when the shown minutes change.
      await expect.poll(() => preview(page), { timeout: 10_000, intervals: [250] }).not.toBe(current);
      await expect(page.getByText(token)).toHaveCount(0);
      // Lowest remaining first, left to right: one (10%, danger) takes the first panel and three
      // (30%, caution) the second. Four needs more width than is left, so it and every contract
      // after it are not shown.
      hub.send('stats', ranked());
      await expect.poll(async () => (await look(page, await preview(page), secondPanel, caution)).count).toBeGreaterThan(100);
      const ordered = await preview(page);
      expect((await look(page, ordered, wholeImage, panelFill)).runs).toBe(2);
      expect((await look(page, ordered, firstPanel, danger)).count).toBeGreaterThan(100);
      expect((await look(page, ordered, secondPanel, danger)).count).toBe(0);
    });
  } finally {
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
