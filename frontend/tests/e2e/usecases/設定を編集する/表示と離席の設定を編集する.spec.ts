import { test, expect } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { saveLargeDisplayFixture, startServer } from '../../support/server';

// The server build has no task tray. Opening the page stands in for opening the window, and stopping the
// process for Exit. A key is saved with the value the page sends, so the saved file is read for it.
test('Settings 画面で Cycle・Cycle time・Away time を Save なしで保存する', async ({ page, context }) => {
  test.setTimeout(120_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-settings-e2e-'));
  const displayID = saveLargeDisplayFixture(dataDir);
  let server = await startServer(dataDir, 34152);
  const cycle = () => page.getByRole('switch', { name: 'Cycle' });
  const time = () => page.getByRole('textbox', { name: 'Cycle time' });
  const away = () => page.getByRole('textbox', { name: 'Away time' });
  // The output fixture is saved; the setting values are still their defaults.
  const file = () => { try { return JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8')); } catch { return {}; } };
  const enter = async (input: ReturnType<typeof time>, value: string) => { await input.fill(value); await input.press('Enter'); };
  const leave = async (input: ReturnType<typeof time>, value: string) => { await input.fill(value); await input.blur(); };
  try {
    await test.step('開始条件', async () => {
      await page.goto(server.url);
      await expect(page.getByRole('navigation', { name: 'Menu' })).toBeVisible();
    });
    await test.step('手順1', async () => {
      // Settings is the last item of the menu. Separate cards hold 9.2-inch and common settings, and nothing
      // has been changed yet: Cycle is off, the time is 10 seconds and Away time is 10 minutes.
      await expect(page.getByRole('navigation', { name: 'Menu' }).getByRole('link')).toHaveText(['Display', 'Styles', 'Connection', 'Settings']);
      await page.getByRole('link', { name: 'Settings' }).click();
      await expect(page.getByRole('heading', { name: 'Settings', level: 2 })).toBeVisible();
      await expect(cycle()).toHaveAttribute('aria-checked', 'false');
      await expect(time()).toHaveValue('10 s');
      await expect(away()).toHaveValue('10 min');
      const card = (input: ReturnType<typeof time>) => input.locator('xpath=ancestor::section[contains(@class,"Card")][1]').elementHandle();
      expect(await (await card(time()))!.evaluate((a, b) => a === b, (await card(away()))!)).toBe(false);
      const large = page.getByRole('region', { name: '9.2-inch settings', exact: true });
      const common = page.getByRole('region', { name: 'Common settings', exact: true });
      await expect(large.getByRole('heading', { name: '9.2-inch only', exact: true })).toBeVisible();
      await expect(common.getByRole('heading', { name: 'All displays', exact: true })).toBeVisible();
      await expect(large.getByRole('textbox', { name: 'Away time' })).toHaveCount(0);
      await expect(common.getByRole('switch', { name: 'Cycle' })).toHaveCount(0);
      await expect(page.getByRole('heading', { name: /^(Display|Away)$/ })).toHaveCount(0);
      const sw = (await cycle().boundingBox())!;
      const t = (await time().boundingBox())!;
      expect(t.x).toBeGreaterThan(sw.x + sw.width);
      expect(await time().evaluate(e => getComputedStyle(e).textAlign)).toBe('right');
      expect(await away().evaluate(e => getComputedStyle(e).textAlign)).toBe('right');
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
    });
    await test.step('手順2', async () => {
      // Turning Cycle on or off saves at once, with no message.
      await cycle().click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'true');
      await expect.poll(() => file().cycle).toBe(true);
      await expect(page.getByText('Saved.')).toHaveCount(0);
      await cycle().click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'false');
      await expect.poll(() => file().cycle ?? false).toBe(false);
    });
    await test.step('手順3', async () => {
      // 5 to 3600 is saved at once; anything else is not saved and the saved value is shown again.
      await enter(time(), '5');
      await expect.poll(() => file().cycleSeconds).toBe(5);
      await expect(time()).toHaveValue('5 s');
      await enter(time(), '4');
      await expect(time()).toHaveValue('5 s');
      await leave(time(), '3601');
      await expect(time()).toHaveValue('5 s');
      expect(file().cycleSeconds).toBe(5);
    });
    await test.step('手順4', async () => {
      // 1 to 240 is saved at once; anything else is not saved and the saved value is shown again.
      await enter(away(), '2');
      await expect.poll(() => file().awayMinutes).toBe(2);
      await expect(away()).toHaveValue('2 min');
      await enter(away(), '0');
      await expect(away()).toHaveValue('2 min');
      await leave(away(), '241');
      await expect(away()).toHaveValue('2 min');
      expect(file().awayMinutes).toBe(2);
    });
    await test.step('手順5', async () => {
      // The saved values are shown again after another page and after a restart.
      await page.getByRole('link', { name: 'Display' }).click();
      await page.getByRole('link', { name: 'Settings' }).click();
      await expect(time()).toHaveValue('5 s');
      await expect(away()).toHaveValue('2 min');
      await page.close();
      await server.stop();
      server = await startServer(dataDir, 34152);
      page = await context.newPage();
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Settings' }).click();
      await expect(cycle()).toHaveAttribute('aria-checked', 'false');
      await expect(time()).toHaveValue('5 s');
      await expect(away()).toHaveValue('2 min');
    });
    await test.step('受け入れ条件', async () => {
      await page.setViewportSize({ width: 640, height: 850 });
      const large = (await page.getByRole('region', { name: '9.2-inch settings' }).boundingBox())!;
      const common = (await page.getByRole('region', { name: 'Common settings' }).boundingBox())!;
      expect(common.y).toBeGreaterThan(large.y + large.height);
      expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
      // They are on the Settings page only: the Display page keeps the style choice and has none of them.
      await page.getByRole('link', { name: 'Display' }).click();
      await expect(page.getByRole('textbox', { name: 'Display style' })).toBeVisible();
      await expect(page.getByRole('switch', { name: 'Cycle' })).toHaveCount(0);
      await expect(page.getByRole('textbox', { name: 'Cycle time' })).toHaveCount(0);
      await expect(page.getByRole('textbox', { name: 'Away time' })).toHaveCount(0);
      // Saving them leaves the other saved values as they were.
      const saved = file();
      expect(saved.source).toBe('Local');
      expect(saved.displayID).toBe(displayID);
      expect(saved.limitStyle ?? 'Gauges').toBe('Gauges');
      expect(saved.hiddenLimits ?? []).toEqual([]);
    });
  } finally {
    await page.close().catch(() => undefined);
    await server.stop();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
