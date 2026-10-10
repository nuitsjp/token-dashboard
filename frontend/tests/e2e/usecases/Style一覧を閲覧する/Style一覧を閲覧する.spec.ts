import { test, expect, type Page } from '@playwright/test';
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening
// the window. The image sent towards TURZX is the PNG of CompleteFrame, and the Display preview
// shows that same current style. Points below are the Bars layout.
const line = [42, 47, 58];
const danger = [240, 97, 109];
const normal = [116, 102, 224];
const points = {
  divider: [356, 231],
  alphaBarStart: [389, 124],
  alphaBarMiddle: [560, 124],
} as const;

const token = 'e2e-hub-token';
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

// Themes compiled into this build. Built-in cards stay even when a manifest is absent, and any
// other manifest is a card of its own, sorted by name and then id.
function definedStyles() {
  const root = join(import.meta.dirname, '../../../../../themes');
  const seen = new Set<string>();
  const manifests: { id: string; name: string }[] = [];
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const file = join(root, entry.name, 'theme.json');
    if (!entry.isDirectory() || !existsSync(file)) continue;
    const item = JSON.parse(readFileSync(file, 'utf8')) as { id?: unknown; name?: unknown };
    const id = typeof item.id === 'string' ? item.id.trim() : '';
    const name = typeof item.name === 'string' ? item.name.trim() : '';
    if (!id || !name || seen.has(id)) continue;
    seen.add(id);
    manifests.push({ id, name });
  }
  const extras = manifests.filter(style => style.id !== 'gauges' && style.id !== 'bars')
    .sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
  return [
    { name: 'Gauges', builtin: true },
    { name: 'Bars', builtin: true },
    ...extras.map(style => ({ name: style.name, builtin: false })),
  ];
}

async function preview(page: Page, name: string) {
  const image = page.getByRole('img', { name });
  await expect(image).toBeVisible();
  return (await image.getAttribute('src')) ?? '';
}

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

async function cards(page: Page) {
  return page.getByRole('article').evaluateAll(nodes => nodes.map(article => (article as HTMLElement).innerText.replace(/\s+/g, ' ').trim()));
}

test('定義された表示スタイルを、現在の表示を変えずに見比べる', async ({ page }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-styles-e2e-'));
  const hub = await startHub();
  const server = await startServer(dataDir, 34130, shortIntervals);
  const file = () => JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
  let official = '';
  let saved: ReturnType<typeof file>;
  page.on('request', request => {
    if (!request.url().endsWith('/wails/runtime') || request.method() !== 'POST') return;
    const body = request.postDataJSON();
    if (body?.args?.methodName !== 'token-monitor-turzx/internal/display.Service.CompleteFrame') return;
    const [, png, , error] = body.args.args as [number, string, string, string];
    if (error || !png) return;
    official = `data:image/png;base64,${png}`;
  });
  try {
    await test.step('開始条件', async () => {
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
      await expect.poll(async () => (await preview(page, 'Display preview')).length).toBeGreaterThan(5000);
      saved = file();
      expect(saved.source).toBe('Hub');
      expect(saved.limitStyle ?? 'Gauges').toBe('Gauges');
    });
    await test.step('手順1', async () => {
      await expect(page.getByRole('navigation', { name: 'Menu' }).getByRole('link')).toHaveText(['Display', 'Styles', 'Connection']);
      await page.getByRole('link', { name: 'Styles' }).click();
      await expect(page.getByRole('heading', { name: 'Styles' })).toBeVisible();
      const heading = await page.getByRole('heading', { name: 'Styles' }).boundingBox();
      expect(heading!.height).toBeGreaterThan(20);
      await expect.poll(() => cards(page)).toEqual(['Gauges Built-in In use', 'Bars Built-in']);
      for (const name of ['Gauges', 'Bars']) {
        const image = page.getByRole('article', { name }).getByRole('img', { name: `${name} preview` });
        await expect.poll(() => image.evaluate(img => {
          const element = img as HTMLImageElement;
          return [element.naturalWidth, element.naturalHeight];
        })).toEqual([1920, 462]);
        await expect(page.getByRole('article', { name }).getByRole('button')).toHaveCount(0);
        await expect(page.getByRole('article', { name }).getByRole('textbox')).toHaveCount(0);
      }
      await expect.poll(async () => (await preview(page, 'Gauges preview')).length).toBeGreaterThan(5000);
      await expect.poll(async () => (await preview(page, 'Bars preview')).length).toBeGreaterThan(5000);
      const gauges = await colours(page, await preview(page, 'Gauges preview'));
      expect(gauges.size).toEqual([1920, 462]);
      expect(gauges.alphaBarStart).not.toEqual(danger);
      // The waiting image is already full size. The divider exists only after the Hub snapshot is drawn.
      await expect.poll(async () => {
        const sample = await colours(page, await preview(page, 'Bars preview'));
        return [sample.size, sample.divider, sample.alphaBarStart, sample.alphaBarMiddle];
      }, { timeout: 30_000 }).toEqual([[1920, 462], line, danger, line]);
      await expect(page.getByRole('alert')).toHaveCount(0);
    });
    const labels = ['Gauges Built-in In use', 'Bars Built-in'];
    await test.step('手順2', async () => {
      const gaugesBefore = await preview(page, 'Gauges preview');
      const barsBefore = await preview(page, 'Bars preview');
      hub.send('stats', stats(60));
      await expect.poll(async () => (await colours(page, await preview(page, 'Bars preview'))).alphaBarMiddle).toEqual(normal);
      expect(await preview(page, 'Gauges preview')).not.toBe(gaugesBefore);
      expect(await preview(page, 'Bars preview')).not.toBe(barsBefore);
      await expect.poll(() => cards(page)).toEqual(labels);
      const redrawn = await colours(page, await preview(page, 'Bars preview'));
      expect(redrawn.divider).toEqual(line);
      expect(redrawn.alphaBarStart).toEqual(normal);
      await expect.poll(async () => official ? (await colours(page, official)).divider : []).not.toEqual(line);
    });
    await test.step('手順3', async () => {
      // The list is the manifests compiled into the app. Gauges and Bars stay built-in.
      const expected = definedStyles().map(style => `${style.name}${style.builtin ? ' Built-in' : ''}${style.name === 'Gauges' ? ' In use' : ''}`);
      await page.getByRole('link', { name: 'Display' }).click();
      await page.getByRole('link', { name: 'Styles' }).click();
      await expect.poll(() => cards(page)).toEqual(expected);
      await expect(page.getByRole('article', { name: 'Gauges' }).getByText('Built-in', { exact: true })).toBeVisible();
      await expect(page.getByRole('article', { name: 'Bars' }).getByText('Built-in', { exact: true })).toBeVisible();
    });
    await test.step('受け入れ条件', async () => {
      const defined = definedStyles();
      expect(await cards(page)).toEqual(defined.map(style => `${style.name}${style.builtin ? ' Built-in' : ''}${style.name === 'Gauges' ? ' In use' : ''}`));
      for (const style of defined) {
        const article = page.getByRole('article', { name: style.name });
        await expect(article.getByRole('button')).toHaveCount(0);
        await expect(article.getByRole('textbox')).toHaveCount(0);
        await expect(article.getByRole('checkbox')).toHaveCount(0);
        if (style.builtin) await expect(article.getByText('Built-in', { exact: true })).toBeVisible();
        else await expect(article.getByText('Built-in')).toHaveCount(0);
      }
      const gauges = await colours(page, await preview(page, 'Gauges preview'));
      const bars = await colours(page, await preview(page, 'Bars preview'));
      expect(bars.divider).toEqual(line);
      expect(gauges.divider).not.toEqual(line);
      expect((await colours(page, official)).divider).toEqual(gauges.divider);
      expect((await colours(page, official)).size).toEqual([1920, 462]);
      const update = page.getByRole('heading', { name: 'Update' });
      if (await update.count()) {
        const updateBox = await update.boundingBox();
        const stylesBox = await page.getByRole('heading', { name: 'Styles' }).boundingBox();
        expect(updateBox!.y).toBeLessThan(stylesBox!.y);
        expect(updateBox!.height).toBeGreaterThan(20);
      }
      expect(file()).toEqual(saved);
      await expect(page.getByText(token)).toHaveCount(0);
      await page.getByRole('link', { name: 'Display' }).click();
      await expect(page.getByRole('textbox', { name: 'Display style' })).toHaveValue('Gauges');
      await expect.poll(async () => (await colours(page, await preview(page, 'Display preview'))).divider).not.toEqual(line);
      await expect.poll(async () => (await colours(page, await preview(page, 'Display preview'))).alphaBarStart).not.toEqual(danger);
      expect(file()).toEqual(saved);
    });
  } finally {
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});
