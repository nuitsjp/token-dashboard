import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening
// the window, closing it for hiding the window, and stopping the process for Exit. The image is
// checked by the colours at fixed points of the 1920x462 layout.
const background = [15, 17, 23];
const line = [42, 47, 58]; // the divider and the empty part of a bar
const normal = [144, 133, 233]; // violet
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

test('Display style を Bars に切り替えると、保存して Tokens の列と横棒の利用枠で描き直す', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-bars-e2e-'));
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
    let gauges = '';
    let requests = 0;
    await test.step('手順1', async () => {
      // The Style card (the style choice and the preview) comes before Output.
      const style1 = await page.getByRole('heading', { name: 'Style' }).boundingBox();
      const output1 = await page.getByRole('heading', { name: 'Output' }).boundingBox();
      expect(style1!.y).toBeLessThan(output1!.y);
      await expect(style()).toHaveValue('Gauges');
      await expect.poll(async () => (await preview(page)).length).toBeGreaterThan(5000);
      gauges = await preview(page);
      const shown = await colours(page, gauges);
      expect(shown.alphaBarStart).not.toEqual(danger);
      requests = hub.requests.length;
    });
    await test.step('手順2', async () => {
      // The choice is applied at once: no Save button, no message, and the Hub is not interrupted.
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
      await style().click();
      await page.getByRole('option', { name: 'Bars' }).click();
      await expect.poll(() => preview(page)).not.toBe(gauges);
      await expect(page.getByText('Saved.')).toHaveCount(0);
      const shown = await colours(page, await preview(page));
      expect(shown.divider).toEqual(line);
      expect(shown.alphaBarStart).toEqual(danger);
      expect(shown.alphaBarMiddle).toEqual(line);
      expect(shown.betaBarStart).toEqual(normal);
      expect(shown.betaBarEnd).toEqual(line);
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
      const bars = await preview(page);
      await style().click();
      await page.getByRole('option', { name: 'Gauges' }).click();
      await expect.poll(() => preview(page)).not.toBe(bars);
      await expect.poll(() => file().limitStyle).toBe('Gauges');
      expect((await colours(page, await preview(page))).firstColumnTop).not.toEqual(danger);
      expect(file().source).toBe('Hub');
      expect(file().connection).toBeTruthy();
    });
  } finally {
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
