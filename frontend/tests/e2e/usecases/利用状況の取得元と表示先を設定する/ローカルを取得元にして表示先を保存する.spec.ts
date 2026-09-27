import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';

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
  let server = await startServer(dataDir, 34118);
  const source = page.getByRole('textbox', { name: 'Data source' });
  const device = page.getByRole('textbox', { name: 'Output device' });
  let selectedDevice = 'Automatic';
  try {
    await test.step('開始条件', async () => {
      await page.goto(server.url);
    });
    await test.step('手順1', async () => {
      await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
      await expect(source).toHaveValue('Local');
      await source.click();
      await expect(page.getByRole('option', { name: 'Local' })).toBeVisible();
      await expect(page.getByRole('option', { name: 'Hub' })).toBeVisible();
      await page.keyboard.press('Escape');
      await device.click();
      await expect(page.getByRole('option', { name: 'Automatic' })).toBeVisible();
      const connected = page.getByRole('option', { name: /^TURZX.*\([0-9A-F]{8}\)$/ });
      if (await connected.count()) selectedDevice = (await connected.first().textContent())?.trim() ?? 'Automatic';
      await page.keyboard.press('Escape');
    });
    await test.step('手順2', async () => {
      await source.click();
      await page.getByRole('option', { name: 'Local' }).click();
      await device.click();
      await page.getByRole('option', { name: selectedDevice, exact: true }).click();
      await expect(source).toHaveValue('Local');
      await expect(device).toHaveValue(selectedDevice);
      await expect(page.getByText('Saved.')).toHaveCount(0);
    });
    await test.step('手順3', async () => {
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      const saved = JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
      expect(saved.source).toBe('Local');
      expect(saved.displayName || 'Automatic').toBe(selectedDevice);
      expect(saved.connection).toBeUndefined();
      await expect.poll(() => divider(page), { timeout: 90_000 }).toEqual([42, 47, 58]);
    });
    await test.step('手順4', async () => {
      await server.stop();
      server = await startServer(dataDir, 34118);
      await page.goto(server.url);
      await expect(source).toHaveValue('Local');
      await expect(device).toHaveValue(selectedDevice);
    });
    await test.step('受け入れ条件', async () => {
      await expect(page.getByLabel('Hub URL')).toHaveCount(0);
      await expect(page.getByLabel(/Access token/)).toHaveCount(0);
      await expect.poll(() => divider(page), { timeout: 90_000 }).toEqual([42, 47, 58]);
      await expect(page.getByRole('heading', { name: 'Usage source' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible();
    });
  } finally {
    await server.stop();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
