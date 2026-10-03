import { test, expect, type Page } from '@playwright/test';
import { copyFileSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening
// the window and stopping the process for Exit. The image is checked by counting pixels of the
// colours that the gauge rules give each window.
const background = [15, 17, 23];
const normal = [144, 133, 233]; // violet
const danger = [240, 97, 109];
const caution = [250, 178, 25];
// A drawn arc or bar has far more exact pixels than this; the edges are blended.
const drawn = 30;

const token = 'e2e-hub-token';
const unit = '\u001f';
const keyOf = (provider: string, account: string, kind: string, label: string) => [provider, account, kind, label].join(unit);
const alphaSession = keyOf('alpha', 'alpha@example.com', 'session', '5-hour');
const betaWeekly = keyOf('beta', 'beta@example.com', 'weekly', 'Weekly');

// 4 seconds past the hour, so the shown minutes change 4 seconds after sending.
const hours = (h: number) => new Date(Date.now() + h * 3_600_000 + 4_000).toISOString();
const meter = (kind: string, label: string, remaining: number, resetHours: number) =>
  ({ kind, label, showMeter: true, remainingPercent: remaining, usedPercent: 100 - remaining, resetsAt: hours(resetHours) });
const contract = (provider: string, plan: string, ...windows: ReturnType<typeof meter>[]) =>
  ({ provider, accountLabel: `${provider}@example.com`, planLabel: plan, windows });

// Seven contracts of one circle each, but only six fit in the width, in the order of the lowest
// remaining percent of the windows that are drawn: alpha (10, red; 90 without its 5-hour), beta (45),
// c1 to c4 (50) and zulu (79). Only zulu is drawn in caution, by the pace rule (200 hours to reset
// exceeds the week), so the caution colour shows when zulu is drawn. Only alpha's 5-hour is drawn in
// danger.
function stats(session = 10, withAlpha = true) {
  return {
    periods: { today: { totalTokens: 1234567, costUsd: 1.23 }, month: { totalTokens: 23456789, costUsd: 23.45 }, allTime: { totalTokens: 345678901, costUsd: 345.67 } },
    limits: { providers: [
      contract('zulu', 'Pro', meter('weekly', 'Weekly', 79, 200)),
      ...(withAlpha ? [contract('alpha', 'Pro', meter('session', '5-hour', session, 2), meter('weekly', 'Weekly', 90, 160))] : []),
      contract('beta', 'Max', meter('weekly', 'Weekly', 45, 50)),
      ...['c1', 'c2', 'c3', 'c4'].map(name => contract(name, 'Pro', meter('weekly', 'Weekly', 50, 50))),
    ] },
  };
}

async function preview(page: Page) {
  const image = page.getByRole('img', { name: 'Display preview' });
  await expect(image).toBeVisible();
  return (await image.getAttribute('src')) ?? '';
}

// Counts the pixels of the image in src that equal colour, or differ from the background when colour is
// null, inside rect [left, top, right, bottom].
async function count(page: Page, src: string, colour: number[] | null, rect = [0, 0, 1920, 462]) {
  return page.evaluate(async ({ src, colour, rect, background }) => {
    const img = new Image();
    img.src = src;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext('2d')!;
    context.drawImage(img, 0, 0);
    const [left, top, right, bottom] = rect;
    const data = context.getImageData(left, top, right - left, bottom - top).data;
    const target = colour ?? background;
    let n = 0;
    for (let i = 0; i < data.length; i += 4) {
      const same = data[i] === target[0] && data[i + 1] === target[1] && data[i + 2] === target[2];
      if (colour ? same : !same) n++;
    }
    return n;
  }, { src, colour, rect, background });
}

test('表示する枠を契約と枠で選ぶと、保存してプレビューを描き直し、再起動後も保つ', async ({ page, context }) => {
  test.setTimeout(240_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-limits-e2e-'));
  const hub = await startHub();
  // The server redraws every second instead of every minute.
  let server = await startServer(dataDir, 34124, shortIntervals);
  const settingsFile = join(dataDir, 'settings.json');
  const file = () => JSON.parse(readFileSync(settingsFile, 'utf8'));
  // The page is replaced after a restart, so the locators are made when they are used.
  const sw = (name: string) => page.getByRole('checkbox', { name, exact: true });
  const colourCount = async (colour: number[] | null, rect?: number[]) => count(page, await preview(page), colour, rect);
  let requests = 0;
  let withoutAlpha = 0;
  try {
    await test.step('分岐条件', async () => {
      // The Hub connection is saved. Until the first usage arrives, the list is empty and says so.
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Hub' }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel(/Access token/).fill(token);
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect.poll(() => hub.streams()).toBe(1);
      await page.getByRole('link', { name: 'Display' }).click();
      await expect(page.getByRole('heading', { name: 'Usage Limits' })).toBeVisible();
      await expect(page.getByText('Waiting for usage.')).toBeVisible();
      await expect(page.getByRole('checkbox')).toHaveCount(0);
      hub.send('snapshot', stats());
    });
    await test.step('手順1', async () => {
      // Usage Limits sits below the Style card, and each contract is a card with a switch and its windows.
      await expect(sw('alpha Pro')).toBeVisible();
      const style = await page.getByRole('heading', { name: 'Style' }).boundingBox();
      const limits = await page.getByRole('heading', { name: 'Usage Limits' }).boundingBox();
      expect(style!.y).toBeLessThan(limits!.y);
      // Seven contracts and eight windows, all on, with the remaining percent beside each window.
      await expect(page.getByRole('checkbox')).toHaveCount(7 + 8);
      for (const name of ['zulu Pro', 'alpha Pro', 'beta Max', 'c1 Pro', 'c2 Pro', 'c3 Pro', 'c4 Pro']) await expect(sw(name)).toHaveAttribute('aria-checked', 'true');
      await expect(sw('alpha Pro 5-hour')).toContainText('10%');
      await expect(sw('alpha Pro Weekly')).toContainText('90%');
      // Six of the seven contracts fit: alpha's 5-hour is drawn, and zulu, the last, is not.
      await expect.poll(() => colourCount(danger)).toBeGreaterThan(drawn);
      expect(await colourCount(caution)).toBeLessThan(drawn);
      expect(file().hiddenLimits).toBeUndefined();
      requests = hub.requests.length;
    });
    await test.step('手順2', async () => {
      // A window is hidden at once: no Save button, no message, the contract shows as partly on,
      // and the Hub is not interrupted.
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
      await sw('alpha Pro 5-hour').click();
      await expect(sw('alpha Pro 5-hour')).toHaveAttribute('aria-checked', 'false');
      await expect(sw('alpha Pro Weekly')).toHaveAttribute('aria-checked', 'true');
      await expect(sw('alpha Pro')).toHaveAttribute('aria-checked', 'mixed');
      await expect(page.getByText('Saved.')).toHaveCount(0);
      await expect.poll(() => colourCount(danger)).toBeLessThan(drawn);
      // The order uses the windows that are drawn: alpha, now at 90%, moves behind zulu (79%), which
      // takes its room, and alpha, the seventh, does not fit.
      await expect.poll(() => colourCount(caution)).toBeGreaterThan(drawn);
      withoutAlpha = await colourCount(normal);
      expect(file().hiddenLimits).toEqual([alphaSession]);
      expect(hub.requests.length).toBe(requests);
      expect(hub.streams()).toBe(1);
    });
    await test.step('手順3', async () => {
      // Hiding a contract hides its windows, which frees room for alpha, whose 90% arc adds to the
      // violet that beta's 45% arc had.
      await sw('beta Max').click();
      await expect(sw('beta Max')).toHaveAttribute('aria-checked', 'false');
      await expect(sw('beta Max Weekly')).toHaveAttribute('aria-checked', 'false');
      await expect.poll(() => colourCount(normal)).toBeGreaterThan(withoutAlpha + 200);
      expect(await colourCount(caution)).toBeGreaterThan(drawn);
      expect(file().hiddenLimits).toEqual([alphaSession, betaWeekly].sort());
    });
    await test.step('手順4', async () => {
      // Showing the contract again shows all its windows, and alpha is pushed out again.
      await sw('beta Max').click();
      await expect(sw('beta Max')).toHaveAttribute('aria-checked', 'true');
      await expect(sw('beta Max Weekly')).toHaveAttribute('aria-checked', 'true');
      await expect.poll(() => colourCount(normal)).toBeLessThan(withoutAlpha + 200);
      expect(file().hiddenLimits).toEqual([alphaSession]);
    });
    await test.step('手順5', async () => {
      // A new usage from the Hub keeps the selection and updates the percent in the list.
      hub.send('stats', stats(11));
      await expect(sw('alpha Pro 5-hour')).toContainText('11%');
      await expect(sw('alpha Pro 5-hour')).toHaveAttribute('aria-checked', 'false');
      await expect(sw('alpha Pro')).toHaveAttribute('aria-checked', 'mixed');
      expect(await colourCount(danger)).toBeLessThan(drawn);
    });
    await test.step('手順6', async () => {
      await page.close();
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34124, shortIntervals);
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats());
      page = await context.newPage();
      await page.goto(server.url);
      await expect(sw('alpha Pro 5-hour')).toHaveAttribute('aria-checked', 'false');
      await expect(sw('alpha Pro Weekly')).toHaveAttribute('aria-checked', 'true');
      await expect(sw('alpha Pro')).toHaveAttribute('aria-checked', 'mixed');
      await expect(sw('beta Max')).toHaveAttribute('aria-checked', 'true');
      await expect.poll(() => colourCount(normal)).toBeGreaterThan(drawn);
      expect(await colourCount(danger)).toBeLessThan(drawn);
    });
    await test.step('受け入れ条件', async () => {
      // The same selection applies to Bars, and the Tokens column stays.
      await page.getByRole('textbox', { name: 'Display style' }).click();
      await page.getByRole('option', { name: 'Bars' }).click();
      await expect.poll(() => file().limitStyle).toBe('Bars');
      await expect.poll(async () => colourCount(null, [0, 0, 350, 462])).toBeGreaterThan(drawn);
      await expect.poll(() => colourCount(danger)).toBeLessThan(drawn);
      expect(await colourCount(normal)).toBeGreaterThan(drawn);
      await page.getByRole('textbox', { name: 'Display style' }).click();
      await page.getByRole('option', { name: 'Gauges' }).click();
      await expect.poll(() => file().limitStyle).toBe('Gauges');

      // A selection that cannot be saved leaves the list as it was and shows the error.
      copyFileSync(settingsFile, join(dataDir, 'settings.good'));
      writeFileSync(settingsFile, '{broken');
      await sw('alpha Pro 5-hour').click();
      await expect(page.getByText(/saved settings cannot be read/).first()).toBeVisible();
      await expect(sw('alpha Pro 5-hour')).toHaveAttribute('aria-checked', 'false');
      copyFileSync(join(dataDir, 'settings.good'), settingsFile);
      await page.reload();
      await expect(sw('alpha Pro 5-hour')).toHaveAttribute('aria-checked', 'false');
      expect(file().hiddenLimits).toEqual([alphaSession]);

      // The saved choice of a window that is not in the list is kept while others change.
      hub.send('stats', stats(10, false));
      await expect(sw('alpha Pro')).toHaveCount(0);
      await sw('c1 Pro Weekly').click();
      await expect(sw('c1 Pro Weekly')).toHaveAttribute('aria-checked', 'false');
      expect(file().hiddenLimits).toContain(alphaSession);
      await sw('c1 Pro Weekly').click();
      await expect(sw('c1 Pro Weekly')).toHaveAttribute('aria-checked', 'true');
      expect(file().hiddenLimits).toEqual([alphaSession]);
      hub.send('stats', stats());
      await expect(sw('alpha Pro')).toHaveAttribute('aria-checked', 'mixed');

      // With every contract off, only Tokens is drawn at the top.
      for (const name of ['zulu Pro', 'alpha Pro', 'beta Max', 'c1 Pro', 'c2 Pro', 'c3 Pro', 'c4 Pro']) {
        if ((await sw(name).getAttribute('aria-checked')) === 'mixed') {
          await sw(name).click();
          await expect(sw(name)).toHaveAttribute('aria-checked', 'true');
        }
        await sw(name).click();
        await expect(sw(name)).toHaveAttribute('aria-checked', 'false');
      }
      await expect.poll(() => colourCount(null, [0, 90, 1920, 462])).toBe(0);
      expect(await colourCount(null, [0, 0, 1920, 90])).toBeGreaterThan(drawn);

      // The selection did not change the source, the connection, the display or the style.
      expect(file().source).toBe('Hub');
      expect(file().connection).toBeTruthy();
      expect(file().displayID).toBe('');
      expect(file().limitStyle).toBe('Gauges');
      expect(hub.streams()).toBe(1);
      await expect(page.getByText(token)).toHaveCount(0);
    });
  } finally {
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
