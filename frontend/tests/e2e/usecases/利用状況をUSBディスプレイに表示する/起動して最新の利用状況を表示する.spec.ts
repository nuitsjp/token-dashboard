import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening
// the window, closing it for hiding the window, and stopping the process for Exit. The image is
// checked by the colours at fixed points of the 1920x462 layout.
const background = [15, 17, 23];
const line = [42, 47, 58]; // the divider and the empty part of a bar
const green = [74, 222, 128];
const red = [248, 113, 113];
const points = {
  divider: [960, 119],
  alphaBarStart: [45, 239], // first contract, first bar
  alphaBarMiddle: [208, 239],
  betaBarStart: [45, 383], // second single-window contract, stacked under the first
  betaBarEnd: [360, 383],
  secondColumn: [420, 239],
} as const;

const token = 'e2e-hub-token';
// 10 seconds past the hour, so the shown minutes change 10 seconds after sending.
const hours = (h: number) => new Date(Date.now() + h * 3_600_000 + 10_000).toISOString();
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

test('起動して、Hub の最新の利用状況をプレビューに表示し続ける', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-usage-e2e-'));
  const hub = await startHub();
  let server = await startServer(dataDir, 34117);
  try {
    await test.step('開始条件', async () => {
      // Precondition: the Hub connection is saved. Then the app starts as at sign-in.
      await page.goto(server.url);
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel(/Access token/).fill(token);
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34117);
    });
    await test.step('手順1', async () => {
      await expect.poll(() => hub.streams()).toBe(1);
      const request = hub.requests.at(-1)!;
      expect(request.path).toBe('/api/stats/stream');
      expect(request.headers.authorization).toBe(`Bearer ${token}`);
      expect(request.headers['x-token-monitor-stream']).toBe('2');
      await page.goto(server.url);
      const waiting = await colours(page, await preview(page));
      // Waiting for Hub: no divider and no bars.
      expect(waiting.divider).toEqual(background);
      expect(waiting.alphaBarStart).toEqual(background);
    });
    let first = '';
    await test.step('手順2', async () => {
      const before = await preview(page);
      hub.send('snapshot', stats(12));
      await expect.poll(() => preview(page)).not.toBe(before);
      first = await preview(page);
      const shown = await colours(page, first);
      expect(shown.divider).toEqual(line);
      expect(shown.alphaBarStart).toEqual(red);
      expect(shown.alphaBarMiddle).toEqual(line);
      expect(shown.betaBarStart).toEqual(green);
      expect(shown.betaBarEnd).toEqual(line);
    });
    await test.step('手順3', async () => {
      await page.close();
      page = await context.newPage();
      await page.goto(server.url);
      expect(await preview(page)).toBe(first);
    });
    await test.step('手順4', async () => {
      hub.send('stats', stats(60));
      await expect.poll(async () => (await colours(page, await preview(page))).alphaBarMiddle).toEqual(green);
      expect((await colours(page, await preview(page))).alphaBarStart).toEqual(green);
    });
    await test.step('手順5', async () => {
      await page.close();
      hub.send('stats', stats(12));
      await expect.poll(() => hub.streams()).toBe(1);
      page = await context.newPage();
      await page.goto(server.url);
      await expect.poll(async () => (await colours(page, await preview(page))).alphaBarStart).toEqual(red);
    });
    await test.step('手順6', async () => {
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34117);
    });
    await test.step('受け入れ条件', async () => {
      await page.goto(server.url);
      // Nothing received is kept across a restart: the new process waits for the Hub again.
      expect((await colours(page, await preview(page))).divider).toEqual(background);
      hub.send('snapshot', stats(12));
      await expect.poll(async () => (await colours(page, await preview(page))).divider).toEqual(line);
      const shown = await colours(page, await preview(page));
      expect(shown.size).toEqual([1920, 462]);
      // Contracts without a meter take no column, and single-window contracts share one.
      expect(shown.secondColumn).toEqual(background);
      // The image is drawn again within a minute without anything from the Hub.
      const current = await preview(page);
      await expect.poll(() => preview(page), { timeout: 70_000, intervals: [5_000] }).not.toBe(current);
      await expect(page.getByText(token)).toHaveCount(0);
    });
  } finally {
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
