import { test, expect } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';

// The server build has no task tray. Opening the page stands in for opening the
// window from the tray, and stopping the process stands in for Exit.
test('保存した接続先と表示先を、再起動後も復元する', async ({ page }) => {
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-settings-e2e-'));
  let server = await startServer(dataDir, 34116);
  const hubURL = page.getByLabel('Hub URL');
  const token = page.getByLabel(/Access token/);
  const device = page.getByRole('textbox', { name: 'Output device' });
  const open = (name: 'Display' | 'Connection') => page.getByRole('link', { name }).click();
  try {
    await test.step('分岐条件', async () => {
      await page.goto(server.url);
      await open('Connection');
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Hub' }).click();
    });
    await test.step('手順1', async () => {
      await expect(page.getByRole('heading', { name: 'Connection settings' })).toBeVisible();
      await expect(hubURL).toHaveValue('');
      await expect(page.getByText('Not set', { exact: true })).toBeVisible();
      await expect(token).toHaveValue('');
      await open('Display');
      await expect(page.getByRole('heading', { name: 'Display settings' })).toBeVisible();
      await expect(device).toHaveValue('Automatic');
    });
    await test.step('手順2', async () => {
      await open('Connection');
      await hubURL.fill('https://hub.example.com/');
      await token.fill('e2e-secret-token');
      await expect(page.getByText('Saved.')).toHaveCount(0);
    });
    await test.step('手順3', async () => {
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect(token).toHaveValue('');
      await expect(page.getByText('Set', { exact: true })).toBeVisible();
      await expect(hubURL).toHaveValue('https://hub.example.com');
    });
    await test.step('手順4', async () => {
      await open('Display');
      await device.click();
      await page.getByRole('option', { name: 'Automatic' }).click();
      await expect(device).toHaveValue('Automatic');
      // The Display page has no Save button, and a saved choice shows no message.
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
      await expect(page.getByText('Saved.')).toHaveCount(0);
    });
    await test.step('手順5', async () => {
      await server.stop();
      server = await startServer(dataDir, 34116);
      await page.goto(server.url);
      await open('Connection');
      await expect(hubURL).toHaveValue('https://hub.example.com');
      await expect(page.getByText('Set', { exact: true })).toBeVisible();
      await open('Display');
      await expect(device).toHaveValue('Automatic');
    });
    await test.step('受け入れ条件', async () => {
      const saved = readFileSync(join(dataDir, 'settings.json'), 'utf8');
      expect(saved).not.toContain('hub.example.com');
      expect(saved).not.toContain('e2e-secret-token');
      // Two pages in a side menu. Input is kept across pages. A choice on Display saves at once and does not save the Connection input.
      await expect(page.getByRole('link', { name: 'Display' })).toBeVisible();
      await expect(page.getByRole('link', { name: 'Connection' })).toBeVisible();
      await open('Connection');
      await hubURL.fill('http://127.0.0.1:9999');
      await open('Display');
      await open('Connection');
      await expect(hubURL).toHaveValue('http://127.0.0.1:9999');
      await open('Display');
      await device.click();
      await page.getByRole('option', { name: 'Automatic' }).click();
      await page.reload();
      await open('Connection');
      await expect(hubURL).toHaveValue('https://hub.example.com');
      // An empty token keeps the saved one.
      await hubURL.fill('http://127.0.0.1:8080');
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect(page.getByText('Set', { exact: true })).toBeVisible();
      // Only http(s)://host[:port] is accepted.
      await hubURL.fill('https://hub.example.com/api');
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Enter an http:// or https:// URL with a host and an optional port.')).toBeVisible();
      await page.reload();
      await expect(hubURL).toHaveValue('http://127.0.0.1:8080');
      // Headings and field labels are English.
      await expect(page.getByRole('heading', { name: 'Hub connection' })).toBeVisible();
      await open('Display');
      await expect(page.getByRole('heading', { name: 'Output' })).toBeVisible();
    });
  } finally {
    await server.stop();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
