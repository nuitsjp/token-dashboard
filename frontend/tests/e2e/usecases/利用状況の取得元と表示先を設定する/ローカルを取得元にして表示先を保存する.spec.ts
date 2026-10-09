import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { localHome } from '../../support/local';

// The server has no task tray or USB output. Opening the page stands in for opening
// the window, and stopping the process stands in for Exit.
async function divider(page: Page) {
  const image = page.getByRole('img', { name: 'Display preview' });
  await expect(image).toBeVisible();
  return image.evaluate(async element => {
    const source = element as HTMLImageElement;
    await source.decode();
    const canvas = document.createElement('canvas');
    canvas.width = source.naturalWidth;
    canvas.height = source.naturalHeight;
    const context = canvas.getContext('2d')!;
    context.drawImage(source, 0, 0);
    return [...context.getImageData(356, 231, 1, 1).data.slice(0, 3)];
  });
}

test('Local と表示先を保存し、再起動後もローカルの利用状況を表示する', async ({ page }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-local-settings-e2e-'));
  // tokscale reads an empty home folder, so the usage shown does not depend on this PC.
  const { home, env } = localHome();
  let server = await startServer(dataDir, 34118, env);
  const source = page.getByRole('textbox', { name: 'Data source' });
  const device = page.getByRole('textbox', { name: 'Output device' });
  let selectedDevice = 'Automatic';
  const open = (name: 'Display' | 'Connection') => page.getByRole('link', { name }).click();
  try {
    await test.step('開始条件', async () => {
      await page.goto(server.url);
    });
    await test.step('手順1', async () => {
      await open('Connection');
      await expect(page.getByRole('heading', { name: 'Connection settings' })).toBeVisible();
      await expect(source).toHaveValue('Local');
      await source.click();
      await expect(page.getByRole('option', { name: 'Local' })).toBeVisible();
      await expect(page.getByRole('option', { name: 'Hub' })).toBeVisible();
      await page.keyboard.press('Escape');
      await open('Display');
      await device.click();
      await expect(page.getByRole('option', { name: 'Automatic' })).toBeVisible();
      const connected = page.getByRole('option', { name: /^TURZX.*\([0-9A-Z]{8}\)$/ });
      if (await connected.count()) selectedDevice = (await connected.first().textContent())?.trim() ?? 'Automatic';
      await page.keyboard.press('Escape');
    });
    await test.step('手順2', async () => {
      // The image checks read the Bars layout (the divider after Tokens).
      await open('Display');
      await page.getByRole('textbox', { name: 'Display style' }).click();
      await page.getByRole('option', { name: 'Bars' }).click();
      await open('Connection');
      await source.click();
      await page.getByRole('option', { name: 'Local' }).click();
      await expect(source).toHaveValue('Local');
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      const saved = JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
      expect(saved.source).toBe('Local');
      expect(saved.connection).toBeUndefined();
    });
    await test.step('手順3', async () => {
      await open('Display');
      await device.click();
      await page.getByRole('option', { name: selectedDevice, exact: true }).click();
      await expect(device).toHaveValue(selectedDevice);
      // The Display page has no Save button, and a saved choice shows no message.
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
      await expect(page.getByText('Saved.')).toHaveCount(0);
      await expect.poll(() => JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8')).displayName || 'Automatic').toBe(selectedDevice);
      const saved = JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
      expect(saved.displayName || 'Automatic').toBe(selectedDevice);
      const wide = await page.getByRole('img', { name: 'Display preview' }).evaluate(image => (image as HTMLImageElement).naturalWidth === 1920);
      await expect.poll(() => divider(page), { timeout: 90_000 }).toEqual(wide ? [42, 47, 58] : [15, 17, 23]);
    });
    await test.step('手順4', async () => {
      await server.stop();
      server = await startServer(dataDir, 34118, env);
      await page.goto(server.url);
      await open('Connection');
      await expect(source).toHaveValue('Local');
      await open('Display');
      await expect(device).toHaveValue(selectedDevice);
    });
    await test.step('受け入れ条件', async () => {
      await open('Connection');
      await expect(page.getByLabel('Hub URL')).toHaveCount(0);
      await expect(page.getByLabel(/Access token/)).toHaveCount(0);
      await expect(page.getByRole('heading', { name: 'Usage source' })).toBeVisible();
      await open('Display');
      const wide = await page.getByRole('img', { name: 'Display preview' }).evaluate(image => (image as HTMLImageElement).naturalWidth === 1920);
      await expect.poll(() => divider(page), { timeout: 90_000 }).toEqual(wide ? [42, 47, 58] : [15, 17, 23]);
      await expect(page.getByRole('heading', { name: 'Output' })).toBeVisible();
    });
  } finally {
    await server.stop();
    rmSync(dataDir, { recursive: true, force: true });
    rmSync(home, { recursive: true, force: true });
  }
});
