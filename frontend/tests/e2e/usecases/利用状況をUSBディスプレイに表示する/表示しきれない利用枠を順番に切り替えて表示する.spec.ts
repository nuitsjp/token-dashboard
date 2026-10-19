import { test, expect, type Locator, type Page } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { saveLargeDisplayFixture, startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening the
// window and stopping the process for Exit. Cycle is set on the Settings page of `page`, and the image is
// watched on the Display page of `view`, a second window onto the same server. The server waits a tenth of the saved Cycle time, and the
// page on show is told by the colour of the first bar of the Bars style: ten contracts of two windows
// put eight on the first page, led by the lowest remaining (red), and two on the second (violet).
const danger = [240, 97, 109];
const normal = [116, 102, 224];
const firstBar = [389, 124];
const token = 'e2e-hub-token';

// Half an hour past the hour, so the shown time until reset does not change while a test runs.
const hours = (h: number) => new Date(Date.now() + h * 3_600_000 + 1_800_000).toISOString();
const meter = (label: string, remaining: number) =>
  ({ kind: 'weekly', label, showMeter: true, remainingPercent: remaining, usedPercent: 100 - remaining, resetsAt: hours(100) });
// c0 has the lowest remaining (10%), c9 the highest. With 3 contracts everything fits on the first page.
function stats(count = 10) {
  return {
    periods: { today: { totalTokens: 1234567, costUsd: 1.23 }, month: { totalTokens: 23456789, costUsd: 23.45 }, allTime: { totalTokens: 345678901, costUsd: 345.67 } },
    limits: { providers: Array.from({ length: count }, (_, i) => ({
      provider: `c${i}`, accountLabel: `c${i}@example.com`, planLabel: 'Pro',
      windows: [meter('A', 10 + i * 8), meter('B', 15 + i * 8)],
    })) },
  };
}

const imageSrc = async (page: Page) => (await page.getByRole('img', { name: 'Display preview' }).getAttribute('src')) ?? '';

async function colourAt(page: Page, [x, y]: number[]) {
  return page.evaluate(async ({ src, x, y }) => {
    const img = new Image();
    img.src = src;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext('2d')!;
    context.drawImage(img, 0, 0);
    return [...context.getImageData(x, y, 1, 1).data.slice(0, 3)];
  }, { src: await imageSrc(page), x, y });
}
const shown = (page: Page) => colourAt(page, firstBar);

test('Cycle を入れると、表示しきれない契約をページとして Cycle time ごとに順番に表示する', async ({ page, context }) => {
  let view: Page = page;
  test.setTimeout(240_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-cycle-e2e-'));
  saveLargeDisplayFixture(dataDir);
  const hub = await startHub();
  let server = await startServer(dataDir, 34140, shortIntervals);
  const style = () => view.getByRole('textbox', { name: 'Display style' });
  const cycle = () => page.getByRole('switch', { name: 'Cycle' });
  const time = () => page.getByRole('textbox', { name: 'Cycle time' });
  const file = () => JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
  // Enter is pressed once a value in range is shown, so that the page has taken it; a smaller one is
  // made 5 by the input itself.
  const setTime = async (value: string) => {
    await time().fill(value);
    if (Number(value) >= 5) await expect(time()).toHaveValue(new RegExp(`^${value}( s)?$`));
    await time().press('Enter');
  };
  // Keeps watching the page on show, for a page that must not change.
  const stays = async (colour: number[], ms = 2_500) => {
    const end = Date.now() + ms;
    while (Date.now() < end) { expect(await shown(view)).toEqual(colour); await page.waitForTimeout(150); }
  };
  // How many times the image changes in the given time.
  const changes = async (ms: number) => {
    const end = Date.now() + ms;
    let last = '';
    let n = 0;
    while (Date.now() < end) {
      const now = await imageSrc(view);
      if (last && now !== last) n++;
      last = now;
      await page.waitForTimeout(100);
    }
    return n;
  };
  try {
    await test.step('分岐条件', async () => {
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Hub' }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel(/Access token/).fill(token);
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats());
      view = await context.newPage();
      await view.goto(server.url);
      await style().click();
      await view.getByRole('option', { name: 'Bars', exact: true }).click();
      await expect.poll(() => file().limitStyle).toBe('bars');
      await expect.poll(() => shown(view)).toEqual(danger);
    });
    await test.step('手順1', async () => {
      // Cycle and Cycle time sit in one row of the card on the Settings page, the switch left of
      // the time, which is right-aligned. The switch has the design of the Usage Limits contracts.
      // The window of `page` reads the saved style again, as the one window of the app always has it.
      await page.reload();
      await page.getByRole('link', { name: 'Settings' }).click();
      const sw = (await cycle().boundingBox())!;
      const input = (await time().boundingBox())!;
      expect(input.x).toBeGreaterThan(sw.x + sw.width);
      expect(Math.abs(sw.y + sw.height / 2 - (input.y + input.height / 2))).toBeLessThan(8);
      expect(await time().evaluate(e => getComputedStyle(e).textAlign)).toBe('right');
      await expect(cycle()).toHaveAttribute('aria-checked', 'false');
      await expect(time()).toHaveValue('10 s');
      const contractSwitch = view.getByRole('checkbox', { name: 'c0 Pro', exact: true });
      const design = (el: Locator) => el.evaluate(e => { const s = getComputedStyle(e); return [s.width, s.height, s.borderRadius]; });
      expect(await design(cycle())).toEqual(await design(contractSwitch));
      const background = (el: Locator) => el.evaluate(e => getComputedStyle(e).backgroundColor);
      await cycle().click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'true');
      // The colour changes over a short transition.
      await expect.poll(() => background(cycle())).toBe(await background(contractSwitch));
      await cycle().click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'false');
      await expect.poll(() => file().cycle ?? false).toBe(false);
      // The image is redrawn for the first page, which then stays.
      await expect.poll(() => shown(view)).toEqual(danger);
      await stays(danger, 1_500);
      // The Display page no longer has them.
      await expect(view.getByRole('switch', { name: 'Cycle' })).toHaveCount(0);
      await expect(view.getByRole('textbox', { name: 'Cycle time' })).toHaveCount(0);
    });
    await test.step('手順2', async () => {
      // Turning it on saves at once, with no Save button, and starts at the first page.
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
      await cycle().click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'true');
      await expect.poll(() => file().cycle).toBe(true);
      expect(file().limitStyle).toBe('bars');
      expect(file().source).toBe('Hub');
      expect(file().connection).toBeTruthy();
      await expect.poll(() => shown(view)).toEqual(danger);
    });
    await test.step('手順3', async () => {
      // With 10 seconds the server waits 1 second: the second page comes, and the first again after it.
      await expect.poll(() => shown(view), { timeout: 10_000 }).toEqual(normal);
      await expect.poll(() => shown(view), { timeout: 10_000 }).toEqual(danger);
      await expect.poll(() => shown(view), { timeout: 10_000 }).toEqual(normal);
      // Tokens do not move: the colour at a point of Today's tokens is the same on both pages.
      const left = await colourAt(view, [60, 90]);
      await expect.poll(() => shown(view), { timeout: 10_000 }).toEqual(danger);
      expect(await colourAt(view, [60, 90])).toEqual(left);
    });
    await test.step('手順4', async () => {
      // A page stays for 10 seconds (100 seconds of the saved time). A new usage keeps the page number.
      // Fewer pages bring the first page, which stays when everything fits on it.
      await setTime('100');
      await expect.poll(() => file().cycleSeconds).toBe(100);
      await expect.poll(() => shown(view), { timeout: 15_000 }).toEqual(normal);
      hub.send('stats', stats());
      await page.waitForTimeout(1_000);
      expect(await shown(view)).toEqual(normal);
      hub.send('stats', stats(3));
      await expect.poll(() => shown(view)).toEqual(danger);
      hub.send('stats', stats());
      await page.waitForTimeout(500);
      expect(await shown(view)).toEqual(danger);
    });
    await test.step('手順5', async () => {
      // The entered time applies at once: 5 seconds make the server wait half a second. A value out of
      // range is refused, and the saved time is shown again.
      await setTime('5');
      await expect.poll(() => file().cycleSeconds).toBe(5);
      await expect(time()).toHaveValue('5 s');
      expect(await changes(6_000)).toBeGreaterThanOrEqual(6);
      await setTime('4');
      await expect(time()).toHaveValue('5 s');
      await time().fill('3601');
      await time().blur();
      await expect(time()).toHaveValue('5 s');
      expect(file().cycleSeconds).toBe(5);
    });
    await test.step('手順6', async () => {
      // Turning it off keeps the time and goes back to the first page, which then stays.
      await cycle().click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'false');
      await expect.poll(() => file().cycle ?? false).toBe(false);
      expect(file().cycleSeconds).toBe(5);
      await expect.poll(() => shown(view)).toEqual(danger);
      await stays(danger);
    });
    await test.step('手順7', async () => {
      await cycle().click();
      await expect.poll(() => file().cycle).toBe(true);
      await page.close();
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34140, shortIntervals);
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats());
      await view.close();
      page = await context.newPage();
      view = await context.newPage();
      await view.goto(server.url);
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Settings' }).click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'true');
      await expect(time()).toHaveValue('5 s');
      await expect.poll(() => shown(view), { timeout: 30_000 }).toEqual(normal);
      await expect.poll(() => shown(view), { timeout: 30_000 }).toEqual(danger);
    });
    await test.step('受け入れ条件', async () => {
      // Gauges cycles too. With everything on one page nothing changes though Cycle is on, and the
      // other saved values stay as they were.
      await style().click();
      await view.getByRole('option', { name: 'Gauges', exact: true }).click();
      await expect.poll(() => file().limitStyle).toBe('gauges');
      expect(await changes(4_000)).toBeGreaterThanOrEqual(3);
      hub.send('stats', stats(3));
      await page.waitForTimeout(1_500);
      expect(await changes(3_000)).toBe(0);
      expect(file().source).toBe('Hub');
      expect(file().connection).toBeTruthy();
      expect(file().cycle).toBe(true);
      expect(file().cycleSeconds).toBe(5);
    });
  } finally {
    await view.close().catch(() => undefined);
    await page.close().catch(() => undefined);
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
