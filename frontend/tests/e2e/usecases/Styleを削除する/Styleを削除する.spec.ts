import { test, expect, type Page } from '@playwright/test';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { saveLargeDisplayFixture, startServer } from '../../support/server';

// The server build has no task tray. Opening the page stands in for opening the window.
const root = resolve(import.meta.dirname, '../../../../..');

function writeStyle(dir: string, id: string, name: string) {
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, 'theme.json'), JSON.stringify({
    formatVersion: 1, id, name, width: 1920, height: 462, template: 'template.hbs', stylesheet: 'style.css',
  }));
  copyFileSync(join(root, 'themes', 'bars', 'template.hbs'), join(dir, 'template.hbs'));
  writeFileSync(join(dir, 'style.css'), 'body { color: black; }');
}

const names = (page: Page) => page.getByRole('article').evaluateAll(nodes => nodes.map(node => node.getAttribute('aria-label')));
const kept = (dataDir: string) => readdirSync(join(dataDir, 'styles')).filter(name => name !== '.incoming').sort();

test('追加した表示スタイルを1つアプリの定義から取り除く', async ({ page }) => {
  test.setTimeout(120_000);
  const work = mkdtempSync(join(tmpdir(), 'turzx-delete-style-e2e-'));
  const dataDir = join(work, 'data');
  writeStyle(join(dataDir, 'styles', 'night'), 'night', 'Night');
  writeStyle(join(dataDir, 'styles', 'dawn'), 'dawn', 'Dawn');
  writeFileSync(join(dataDir, 'settings.json'), JSON.stringify({ source: 'Local', displayID: '', displayName: '', limitStyle: 'Gauges' }));
  saveLargeDisplayFixture(dataDir);
  const settings = readFileSync(join(dataDir, 'settings.json'), 'utf8');
  let server = await startServer(dataDir, 34131);
  try {
    await test.step('開始条件', async () => {
      await page.goto(`${server.url}/#/styles`);
      await expect(page.getByRole('heading', { name: 'Styles' })).toBeVisible();
      await expect.poll(() => names(page)).toEqual(['Gauges', 'Bars', 'Bars 3.5" Landscape', 'Bars 3.5" Portrait', 'Gauges 3.5" Landscape', 'Gauges 3.5" Portrait', 'Dawn', 'Night']);
    });
    await test.step('手順1', async () => {
      await expect(page.getByRole('article', { name: 'Gauges' }).getByRole('button')).toHaveCount(0);
      await expect(page.getByRole('article', { name: 'Bars' }).getByRole('button')).toHaveCount(0);
      await page.getByRole('button', { name: 'Delete Night' }).click();
      const dialog = page.getByRole('dialog', { name: 'Delete this style?' });
      await expect(dialog).toContainText('Night');
      await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeVisible();
      await expect(dialog.getByRole('button', { name: 'Delete' })).toBeVisible();
    });
    await test.step('受け入れ条件（取消）', async () => {
      await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect.poll(() => names(page)).toEqual(['Gauges', 'Bars', 'Bars 3.5" Landscape', 'Bars 3.5" Portrait', 'Gauges 3.5" Landscape', 'Gauges 3.5" Portrait', 'Dawn', 'Night']);
      expect(kept(dataDir)).toEqual(['dawn', 'night']);
      await page.getByRole('button', { name: 'Delete Night' }).click();
      await page.keyboard.press('Escape');
      await expect(page.getByRole('dialog')).toHaveCount(0);
      expect(kept(dataDir)).toEqual(['dawn', 'night']);
    });
    await test.step('手順2', async () => {
      await page.getByRole('button', { name: 'Delete Night' }).click();
      await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
      await expect.poll(() => names(page)).toEqual(['Gauges', 'Bars', 'Bars 3.5" Landscape', 'Bars 3.5" Portrait', 'Gauges 3.5" Landscape', 'Gauges 3.5" Portrait', 'Dawn']);
      expect(kept(dataDir)).toEqual(['dawn']);
      expect(existsSync(join(dataDir, 'styles', '.incoming')) ? readdirSync(join(dataDir, 'styles', '.incoming')) : []).toEqual([]);
    });
    await test.step('受け入れ条件', async () => {
      expect(readFileSync(join(dataDir, 'settings.json'), 'utf8')).toBe(settings);
      await server.stop();
      server = await startServer(dataDir, 34131);
      await page.goto(`${server.url}/#/styles`);
      await expect.poll(() => names(page)).toEqual(['Gauges', 'Bars', 'Bars 3.5" Landscape', 'Bars 3.5" Portrait', 'Gauges 3.5" Landscape', 'Gauges 3.5" Portrait', 'Dawn']);
      await expect(page.getByRole('article', { name: 'Gauges' }).getByText('In use', { exact: true })).toBeVisible();
      await page.getByRole('link', { name: 'Display', exact: true }).click();
      await page.getByRole('textbox', { name: 'Display style', exact: true }).click();
      await expect(page.getByRole('option', { name: 'Night', exact: true })).toHaveCount(0);
      await expect(page.getByRole('option', { name: 'Dawn', exact: true })).toBeVisible();
      await page.keyboard.press('Escape');
      await page.getByRole('link', { name: 'Styles', exact: true }).click();
      await page.getByRole('button', { name: 'Delete Dawn', exact: true }).click();
      // A second client selects the style while its deletion confirmation is open.
      await page.evaluate(async request => {
        const response = await fetch('/wails/runtime', { method: 'POST', body: JSON.stringify({ object: 0, method: 0,
          args: { 'call-id': crypto.randomUUID(), methodName: 'token-monitor-turzx/internal/settings.Service.Save', args: [request] },
        }) });
        if (!response.ok) throw new Error(await response.text());
      }, { source: 'Local', displayID: JSON.parse(settings).displayID, limitStyle: 'dawn' });
      const selected = readFileSync(join(dataDir, 'settings.json'), 'utf8');
      await page.getByRole('dialog').getByRole('button', { name: 'Delete', exact: true }).click();
      await expect(page.getByRole('alert', { name: 'IN_USE' })).toContainText('Switch to another style before deleting.');
      expect(kept(dataDir)).toEqual(['dawn']);
      expect(readFileSync(join(dataDir, 'settings.json'), 'utf8')).toBe(selected);
      await page.getByRole('link', { name: 'Display', exact: true }).click();
      await expect(page.getByRole('textbox', { name: 'Display style', exact: true })).toHaveValue('Dawn');
      await page.getByRole('link', { name: 'Styles', exact: true }).click();
      await expect(page.getByRole('button', { name: 'Delete Dawn', exact: true })).toBeDisabled();
    });
  } finally {
    await server.stop();
    rmSync(work, { recursive: true, force: true });
  }
});
