import { test, expect, type Page } from '@playwright/test';
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { localHome } from '../../support/local';

const compactID = 'USB\\VID_1A86&PID_5722\\USB35INCHIPSV2';
const window = (label: string, remainingPercent?: number, kind = 'session') => ({ kind, label, showMeter: true, remainingPercent });
const high = { provider: 'alpha', accountLabel: 'high', planLabel: 'High', windows: [window('Weekly', 60, 'weekly')] };
const low = { provider: 'alpha', accountLabel: 'low', planLabel: 'Low', windows: [
  window('A 5-hour', 10), window('B 5-hour', 20), window('C 5-hour', 30), window('D 5-hour', 40), window('E 5-hour', 50),
] };
const last = { provider: 'beta', accountLabel: 'last', planLabel: 'Last', windows: [window('Weekly', 90, 'weekly')] };
const stats = (...providers: typeof high[]) => ({
  periods: { today: { totalTokens: 987654321, costUsd: 1234 }, month: {}, allTime: {} }, limits: { providers },
});
const all = () => stats(high, last, low);

async function image(page: Page) {
  const img = page.getByRole('img', { name: 'Display preview' });
  await expect(img).toBeVisible();
  return (await img.getAttribute('src'))!;
}

// Identify page content by pixels produced at the real renderer boundary. Header signatures
// are learned from isolated input contracts, rather than reproducing font rendering in the test.
async function inspect(page: Page) {
  const src = await image(page);
  return page.evaluate(async src => {
    const img = new Image(); img.src = src; await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth; canvas.height = img.naturalHeight;
    const ctx = canvas.getContext('2d')!; ctx.drawImage(img, 0, 0);
    function hash(x: number, y: number, w: number, h: number) {
      let n = 2166136261;
      for (const v of ctx.getImageData(x, y, w, h).data) n = Math.imul(n ^ v, 16777619);
      return (n >>> 0).toString(16);
    }
    const width = img.naturalWidth, height = img.naturalHeight;
    const data = ctx.getImageData(0, 80, width, height - 118).data;
    const colors = [[240, 97, 109], [250, 178, 25], [116, 102, 224]];
    const counts = colors.map(c => {
      let n = 0;
      for (let i = 0; i < data.length; i += 4) if (data[i] === c[0] && data[i + 1] === c[1] && data[i + 2] === c[2]) n++;
      return n;
    });
    const limitsX = width === 480 ? 218 : 8, limitsY = width === 480 ? 8 : 192;
    return { width, height, header: hash(limitsX + 1, limitsY + 1, 240, 66), footer: hash(width - 100, height - 32, 85, 20),
      panel: Array.from(ctx.getImageData(70, 70, 1, 1).data).slice(0, 3), counts };
  }, src);
}

function saveResponse(page: Page) {
  return page.waitForResponse(r => r.url().endsWith('/wails/runtime')
    && r.request().method() === 'POST'
    && r.request().postDataJSON()?.args?.methodName === 'token-monitor-turzx/internal/settings.Service.Save');
}

async function tokenValuePixels(page: Page) {
  const src = await image(page);
  return page.evaluate(async src => {
    const img = new Image(); img.src = src; await img.decode();
    const canvas = document.createElement('canvas'); canvas.width = img.width; canvas.height = img.height;
    const ctx = canvas.getContext('2d')!; ctx.drawImage(img, 0, 0);
    return [0, 91, 182].flatMap(offset => [
      [20, 56 + offset, 178, 29], [20, 88 + offset, 178, 24],
    ].map(([x, y, w, h]) => {
      let hash = 2166136261;
      for (const v of ctx.getImageData(x, y, w, h).data) hash = Math.imul(hash ^ v, 16777619);
      return hash >>> 0;
    }));
  }, src);
}

async function choose(page: Page, control: string, option: string, success = true) {
  const input = page.getByRole('textbox', { name: control, exact: true });
  const response = await input.inputValue() !== option ? saveResponse(page) : null;
  await input.click();
  await page.getByRole('option', { name: option, exact: true }).click();
  if (response) expect((await response).ok()).toBe(success);
  await expect(input).toBeEnabled();
}

test('3.5インチでサービス別の利用枠を巡回し、4方向と設定を保存する', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-compact-e2e-'));
  const isolated = localHome();
  const settingsFile = join(dataDir, 'settings.json');
  writeFileSync(settingsFile, JSON.stringify({ source: 'Local', displayID: compactID, displayName: 'TURZX 3.5-inch (USB35INC)' }));
  const saved = () => JSON.parse(readFileSync(settingsFile, 'utf8'));
  const hub = await startHub();
  let server = await startServer(dataDir, 34126, isolated.env);
  const checkbox = (name: string) => page.getByRole('checkbox', { name, exact: true });
  const interval = async (seconds: number) => {
    await page.getByRole('textbox', { name: 'Rotation interval (seconds)', exact: true }).fill(String(seconds));
    const response = saveResponse(page);
    await page.getByRole('heading', { name: '3.5-inch display', exact: true }).click();
    expect((await response).ok()).toBe(true);
    // Node read handles deny replacement on Windows. Inspect the file after the save completes.
    await expect.poll(() => saved().rotationIntervalSeconds).toBe(seconds);
  };
  const skip = async (value: boolean) => {
    const sw = page.getByRole('switch', { name: 'Skip services with full 5h limits', exact: true });
    if (await sw.isChecked() !== value) {
      const response = saveResponse(page);
      await page.getByText('Skip services with full 5h limits', { exact: true }).click();
      expect((await response).ok()).toBe(true);
    }
    await expect(sw).toBeChecked({ checked: value });
    await expect.poll(() => saved().skipFull5hServices).toBe(value);
  };
  let lowHeader = '', highHeader = '', lastHeader = '';
  try {
    await test.step('分岐条件', async () => {
      // A server has no tray or USB output; process restart substitutes for Exit. The controlled
      // SSE peer supplies data only; settings, pagination, rendering and preview are production code.
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('textbox', { name: 'Data source', exact: true }).click();
      await page.getByRole('option', { name: 'Hub', exact: true }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel('Access token').fill('compact-e2e-token');
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect.poll(() => hub.streams()).toBe(1);
      await page.getByRole('link', { name: 'Display' }).click();
      await expect(page.getByRole('textbox', { name: 'Output device' })).toHaveValue(/TURZX 3.5-inch/);
    });
    await test.step('手順1', async () => {
      await expect(page.getByRole('textbox', { name: 'Orientation', exact: true })).toHaveValue('Landscape');
      await expect(page.getByRole('textbox', { name: 'Rotation interval (seconds)' })).toHaveValue('10');
      await expect(page.getByRole('switch')).not.toBeChecked();
      await expect(page.getByRole('heading', { name: 'Usage Limits', exact: true }).locator('..')
        .getByText('Waiting for usage.', { exact: true })).toBeVisible();
      await expect.poll(async () => (await inspect(page)).panel).toEqual([15, 17, 23]);
      expect((await inspect(page)).width).toBe(480);
      expect((await inspect(page)).height).toBe(320);
    });
    await test.step('手順2', async () => {
      // Isolated inputs establish identities for Low, High and Last, then combined input must start
      // with Low despite High being reported first and beta being interleaved in the source.
      hub.send('snapshot', stats(last));
      await expect(page.getByText('Last', { exact: true })).toBeVisible();
      await expect.poll(async () => (await inspect(page)).panel).toEqual([21, 24, 33]);
      lastHeader = (await inspect(page)).header;
      hub.send('stats', stats(high));
      await expect(page.getByText('High', { exact: true })).toBeVisible();
      await expect.poll(async () => (await inspect(page)).header).not.toBe(lastHeader);
      highHeader = (await inspect(page)).header;
      hub.send('stats', stats(low));
      await expect(page.getByText('Low', { exact: true })).toBeVisible();
      await expect.poll(async () => (await inspect(page)).counts[0]).toBeGreaterThan(30);
      lowHeader = (await inspect(page)).header;
      hub.send('stats', all());
      await expect(page.getByRole('checkbox')).toHaveCount(7);
      await expect.poll(async () => (await inspect(page)).header).toBe(lowHeader);
    });
    await test.step('手順3', async () => {
      await interval(5);
      const samples = [await inspect(page)];
      for (let i = 0; i < 4; i++) {
        await expect.poll(async () => (await inspect(page)).footer, { timeout: 7000 }).not.toBe(samples[i].footer);
        samples.push(await inspect(page));
      }
      expect(samples.map(s => s.header)).toEqual([lowHeader, lowHeader, highHeader, lastHeader, lowHeader]);
      expect(new Set(samples.slice(0, 4).map(s => s.footer)).size).toBe(4);
      expect(samples[0].counts[0]).toBeGreaterThan(30);
      expect(samples[1].counts[1]).toBeGreaterThan(30);
      expect(samples[2].counts[2]).toBeGreaterThan(30);
    });
    await test.step('手順4', async () => {
      await interval(300);
      for (const style of ['Gauges', 'Bars']) {
        await choose(page, 'Display style', style);
        await expect.poll(() => saved().limitStyle).toBe(style);
        for (const [normal, reversed, width, height] of [
          ['Landscape', 'Landscape (180°)', 480, 320], ['Portrait', 'Portrait (180°)', 320, 480],
        ] as const) {
          await choose(page, 'Orientation', normal);
          await expect.poll(() => saved().orientation).toBe(normal);
          await expect.poll(async () => (await inspect(page)).width).toBe(width);
          await expect.poll(async () => (await inspect(page)).height).toBe(height);
          const upright = await image(page);
          await choose(page, 'Orientation', reversed);
          await expect.poll(() => saved().orientation).toBe(normal === 'Landscape' ? 'ReverseLandscape' : 'ReversePortrait');
          await expect.poll(() => image(page)).toBe(upright);
        }
      }
      await skip(true);
      // Connection saves preserve compact controls too.
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      expect(saved()).toMatchObject({ orientation: 'ReversePortrait', rotationIntervalSeconds: 300, skipFull5hServices: true });
      await page.getByRole('link', { name: 'Display' }).click();
      const previous = await image(page);
      await page.getByRole('textbox', { name: 'Rotation interval (seconds)' }).fill('4');
      await page.getByRole('heading', { name: '3.5-inch display', exact: true }).click();
      await expect(page.getByText('Enter a whole number from 5 to 300.')).toBeVisible();
      expect(saved().rotationIntervalSeconds).toBe(300);
      expect(await image(page)).toBe(previous);
      // An unreadable settings file rejects a real save and keeps the previous control/image.
      const good = readFileSync(settingsFile, 'utf8');
      writeFileSync(settingsFile, '{broken');
      const failed = page.waitForResponse(r => r.url() === `${server.url}/wails/runtime`
        && r.request().postDataJSON()?.args?.methodName === 'token-monitor-turzx/internal/settings.Service.Save');
      await choose(page, 'Orientation', 'Landscape', false);
      expect((await failed).ok()).toBe(false);
      await expect(page.getByText(/saved settings cannot be read/).first()).toBeVisible();
      await expect(page.getByRole('textbox', { name: 'Orientation', exact: true })).toHaveValue('Portrait (180°)');
      expect(await image(page)).toBe(previous);
      writeFileSync(settingsFile, good);
      await page.reload();
      await expect(page.getByRole('textbox', { name: 'Orientation', exact: true })).toHaveValue('Portrait (180°)');
    });
    await test.step('手順5', async () => {
      await choose(page, 'Display style', 'Gauges');
      await skip(false);
      await interval(5);
      const first = (await inspect(page)).footer;
      let remaining = 15;
      const updates = setInterval(() => hub.send('stats', stats(high, last, { ...low,
        windows: [window('A 5-hour', remaining++ % 2 ? 15 : 16), ...low.windows.slice(1)] })), 500);
      try {
        await expect.poll(async () => (await inspect(page)).footer, { timeout: 7000 }).not.toBe(first);
      } finally { clearInterval(updates); }
      // Removing all pages of the current contract chooses its surviving successor.
      for (const label of ['A', 'B', 'C', 'D', 'E']) await checkbox(`alpha Low ${label} 5-hour`).click();
      await expect(checkbox('alpha Low A 5-hour')).toHaveAttribute('aria-checked', 'false');
      await expect.poll(async () => (await inspect(page)).header).toBe(highHeader);
      expect(saved().hiddenLimits).toHaveLength(5);
    });
    await test.step('手順6', async () => {
      const before = (await inspect(page)).footer;
      await page.close();
      await new Promise(done => setTimeout(done, 5200));
      page = await context.newPage();
      await page.goto(server.url);
      await expect.poll(async () => (await inspect(page)).footer).not.toBe(before);
      await expect(checkbox('alpha Low A 5-hour')).toHaveAttribute('aria-checked', 'false');
    });
    await test.step('手順7', async () => {
      await skip(true);
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34126, isolated.env);
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', all());
      await page.goto(server.url);
      await expect(page.getByRole('textbox', { name: 'Orientation', exact: true })).toHaveValue('Portrait (180°)');
      await expect(page.getByRole('textbox', { name: 'Rotation interval (seconds)' })).toHaveValue('5');
      await expect(page.getByRole('switch')).toBeChecked();
      await expect(checkbox('alpha Low A 5-hour')).toHaveAttribute('aria-checked', 'false');
      expect(saved().displayID).toBe(compactID);
      await expect.poll(async () => (await inspect(page)).header).toBe(highHeader);
    });
    await test.step('受け入れ条件', async () => {
      // Empty, fully skipped and recovered unknown/non-five-hour services have distinct outputs.
      hub.send('stats', stats());
      await expect(page.getByRole('checkbox')).toHaveCount(0);
      await expect.poll(async () => (await inspect(page)).panel).toEqual([15, 17, 23]);
      const empty = await image(page);
      hub.send('stats', stats({ provider: 'full', accountLabel: 'full', planLabel: 'Full', windows: [window('5h', 100)] }));
      await expect(page.getByText('Full', { exact: true })).toBeVisible();
      await expect.poll(() => image(page)).not.toBe(empty);
      expect((await inspect(page)).panel).toEqual([15, 17, 23]);
      hub.send('stats', stats(
        { provider: 'unknown', accountLabel: 'unknown', planLabel: 'Unknown', windows: [window('5h')] },
        { provider: 'rounded', accountLabel: 'rounded', planLabel: 'Rounded', windows: [window('5h', 99.9)] },
      ));
      await expect(checkbox('unknown Unknown 5h')).toContainText('—');
      await expect(checkbox('rounded Rounded 5h')).toContainText('100%');
      await expect.poll(async () => (await inspect(page)).panel).toEqual([21, 24, 33]);
      const resumed = (await inspect(page)).footer;
      await expect.poll(async () => (await inspect(page)).footer, { timeout: 7000 }).not.toBe(resumed);
      hub.send('stats', stats({ provider: 'weekly', accountLabel: 'weekly', planLabel: 'Weekly', windows: [window('weekly', 100, 'weekly')] }));
      await expect(page.getByText('Weekly', { exact: true })).toBeVisible();
      await expect.poll(async () => (await inspect(page)).panel).toEqual([21, 24, 33]);
      expect(saved().skipFull5hServices).toBe(true);
      await expect(page.getByText('compact-e2e-token', { exact: true })).toHaveCount(0);
    });
  } finally {
    await server.stop(); await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
    rmSync(isolated.home, { recursive: true, force: true });
  }
});

test('サービスカードで表示内容を選択し、サービス別Tokensと画面全体を操作する', async ({ page }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-content-e2e-'));
  const isolated = localHome();
  const settingsFile = join(dataDir, 'settings.json');
  writeFileSync(settingsFile, JSON.stringify({ source: 'Local', displayID: compactID,
    rotationIntervalSeconds: 300, compactServiceContent: { alpha: {}, absent: { showLimits: false } } }));
  const saved = () => JSON.parse(readFileSync(settingsFile, 'utf8'));
  const hub = await startHub();
  let server = await startServer(dataDir, 34129, isolated.env);
  const longID = 'z-service-with-a-very-long-reported-identifier-that-must-wrap-without-clipping-the-card-or-causing-horizontal-overflow';
  const names = ['alpha', 'beta', 'unknown', 'delta', 'only-tokens', longID, 'zero'];
  const payload = {
    periods: {
      today: { totalTokens: 999, costUsd: 999, clients: { alpha: 1234567890123, delta: 1, 'only-tokens': 12, zero: 0, [longID]: 8 }, clientCosts: { alpha: 12345678.12, zero: 0 } },
      month: { totalTokens: 999, costUsd: 999, clients: { alpha: 2345678901234 }, clientCosts: { alpha: 23456789.23 } },
      allTime: { totalTokens: 999, costUsd: 999, clients: { alpha: 3456789012345 }, clientCosts: { alpha: 34567890.34 } },
    }, limits: { providers: [
      { provider: 'alpha', accountLabel: 'low', planLabel: 'Low', windows: [window('A 5h', 10), window('B 5h', 30), window('C 5h', 80)] },
      last,
      { provider: 'alpha', accountLabel: 'high', planLabel: 'High', windows: [window('Weekly', 90, 'weekly')] },
      { provider: 'unknown', accountLabel: 'unknown', planLabel: 'Unknown', windows: [] },
    ] },
  };
  const group = (name: string) => page.getByRole('radiogroup', { name: `${name} display content`, exact: true });
  const requests: string[] = [];
  const contentMethod = 'token-monitor-turzx/internal/display.Service.SetServiceContent';
  page.on('request', request => {
    if (request.method() === 'POST' && request.url().endsWith('/wails/runtime')) {
      const method = request.postDataJSON()?.args?.methodName;
      if (method === contentMethod) requests.push(method);
    }
  });
  const serviceToggle = (name: string) => page.getByRole('switch', { name: `${name} display enabled`, exact: true });
  const select = async (name: string, choice: string) => {
    const apply = async (action: () => Promise<void>, enabled: boolean, content: string) => {
      const previous = await image(page), before = requests.length;
      const response = page.waitForResponse(r => r.url().endsWith('/wails/runtime') && r.request().postDataJSON()?.args?.methodName === contentMethod);
      await action();
      expect((await response).ok()).toBe(true);
      await expect(serviceToggle(name)).toBeChecked({ checked: enabled });
      await expect(serviceToggle(name)).toBeEnabled();
      await expect(group(name).getByRole('radio', { name: content, exact: true })).toBeChecked();
      for (const radio of await group(name).getByRole('radio').all()) {
        if (enabled) await expect(radio).toBeEnabled();
        else await expect(radio).toBeDisabled();
      }
      expect(requests).toHaveLength(before + 1);
      expect(saved().compactServiceContent[name]).toEqual({ enabled, showLimits: content === 'Both' || content === 'Limits', showTokens: content === 'Both' || content === 'Tokens' });
      await expect.poll(() => image(page)).not.toBe(previous);
    };
    const enabled = choice !== 'Off';
    if (await serviceToggle(name).isChecked() !== enabled) {
      const content = (await group(name).getByRole('radio', { checked: true }).getAttribute('value'))!;
      await apply(async () => { await serviceToggle(name).focus(); await serviceToggle(name).press(' '); }, enabled, content);
    }
    if (enabled && !await group(name).getByRole('radio', { name: choice, exact: true }).isChecked()) {
      await apply(async () => { await group(name).getByText(choice, { exact: true }).click(); }, true, choice);
    }
  };
  const toggle = () => page.getByRole('button', { name: 'Service content', exact: true });
  try {
    await test.step('分岐条件', async () => {
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('textbox', { name: 'Data source', exact: true }).click();
      await page.getByRole('option', { name: 'Hub', exact: true }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel('Access token').fill('content-e2e-token');
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect.poll(() => hub.streams()).toBe(1);
      await page.getByRole('link', { name: 'Display' }).click();
    });
    await test.step('手順1', async () => {
      await expect(toggle()).toHaveAttribute('aria-expanded', 'false');
      await expect(page.getByRole('radiogroup')).toHaveCount(0);
      await expect(page.getByRole('heading', { name: 'Usage Limits', exact: true }).locator('..').getByText('Waiting for usage.', { exact: true })).toBeVisible();
    });
    await test.step('手順2', async () => {
      hub.send('snapshot', payload);
      await expect(page.getByText('Low', { exact: true })).toBeVisible();
      await expect.poll(async () => (await inspect(page)).panel).toEqual([21, 24, 33]);
      const before = await image(page);
      await toggle().focus(); await toggle().press('Enter');
      await expect(page.getByRole('radiogroup')).toHaveCount(7);
      expect(await page.getByRole('radiogroup').evaluateAll(es => es.map(e => e.getAttribute('aria-label')))).toEqual(names.map(name => `${name} display content`));
      await expect(group('alpha').getByRole('radio', { name: 'Both', exact: true })).toBeChecked();
      await toggle().press(' ');
      await expect(page.getByRole('radiogroup')).toHaveCount(0);
      expect(await image(page)).toBe(before);
      expect(requests).toHaveLength(0);
      await toggle().click();
      // Six independently changing value regions prove attribution and Tokens-only redraws
      // through SSE -> production Stats -> current page -> generated PNG.
      for (const [i, period, field] of [
        [0, 'today', 'clients'], [1, 'today', 'clientCosts'], [2, 'month', 'clients'],
        [3, 'month', 'clientCosts'], [4, 'allTime', 'clients'], [5, 'allTime', 'clientCosts'],
      ] as const) {
        const beforeValues = await tokenValuePixels(page), footer = (await inspect(page)).footer;
        payload.periods[period][field].alpha++;
        hub.send('stats', payload);
        await expect.poll(async () => (await tokenValuePixels(page))[i]).not.toBe(beforeValues[i]);
        const after = await tokenValuePixels(page);
        for (let j = 0; j < 6; j++) if (j !== i) expect(after[j]).toBe(beforeValues[j]);
        expect((await inspect(page)).footer).toBe(footer);
      }
      const attributed = await image(page);
      payload.periods.today.totalTokens = 123; payload.periods.today.costUsd = 456;
      hub.send('stats', payload);
      await expect(page.getByText('Low', { exact: true })).toBeVisible();
      expect(await image(page)).toBe(attributed);
    });
    await test.step('手順3', async () => {
      for (const name of names.slice(1)) await select(name, 'Off');
      await select('alpha', 'Tokens');
      await page.getByRole('textbox', { name: 'Rotation interval (seconds)' }).fill('5');
      const response = saveResponse(page);
      await page.getByRole('heading', { name: '3.5-inch display', exact: true }).click();
      expect((await response).ok()).toBe(true);
      const only = await image(page);
      await page.waitForTimeout(5300);
      expect(await image(page)).toBe(only); // Two contracts produce one Tokens-only page.
      for (const name of names.slice(1)) await select(name, 'Both');
      await select('alpha', 'Both');
      const first = (await inspect(page)).footer;
      const updates = setInterval(() => {
        payload.periods.today.clients.alpha++;
        hub.send('stats', payload);
      }, 500);
      try {
        await expect.poll(async () => (await inspect(page)).footer, { timeout: 7000 }).not.toBe(first);
      } finally { clearInterval(updates); }
      await page.getByRole('textbox', { name: 'Rotation interval (seconds)' }).fill('300');
      const savedResponse = saveResponse(page);
      await page.getByRole('heading', { name: '3.5-inch display', exact: true }).click();
      expect((await savedResponse).ok()).toBe(true);
    });
    await test.step('手順4', async () => {
      // Delay one real save, and keep the old choice while all controls are disabled.
      let release!: () => void;
      const pending = new Promise<void>(done => { release = done; });
      await page.route('**/wails/runtime', async route => {
        if (route.request().postDataJSON()?.args?.methodName === contentMethod) await pending;
        await route.continue();
      });
      await group('alpha').getByRole('radio', { name: 'Both', exact: true }).focus();
      await group('alpha').getByRole('radio', { name: 'Both', exact: true }).press('ArrowRight');
      await expect(group('alpha').getByRole('radio', { name: 'Both', exact: true })).toBeChecked();
      for (const name of names) {
        await expect(group(name).getByRole('radio').first()).toBeDisabled();
        await expect(serviceToggle(name)).toBeDisabled();
      }
      release();
      await expect(group('alpha').getByRole('radio', { name: 'Limits', exact: true })).toBeChecked();
      await expect(group('alpha').getByRole('radio').first()).toBeEnabled();
      await expect(serviceToggle('alpha')).toBeEnabled();
      await page.unroute('**/wails/runtime');
      // OFF retains all three choices; enabling restores each without another content save.
      for (const choice of ['Both', 'Limits', 'Tokens']) {
        await select('alpha', choice);
        const on = await image(page);
        const name = group('alpha').locator('..').getByText('alpha', { exact: true });
        const activeColor = await name.evaluate(el => getComputedStyle(el).color);
        await select('alpha', 'Off');
        await expect(name).not.toHaveCSS('color', activeColor);
        await select('alpha', choice);
        await expect(name).toHaveCSS('color', activeColor);
        await expect.poll(() => image(page)).toBe(on);
      }
      await select('alpha', 'Limits');
      for (const style of ['Gauges', 'Bars']) {
        await choose(page, 'Display style', style);
        for (const orientation of ['Landscape', 'Landscape (180°)', 'Portrait', 'Portrait (180°)']) {
          await choose(page, 'Orientation', orientation);
          for (const choice of ['Tokens', 'Off', 'Both', 'Limits']) {
            const before = await image(page);
            await select('alpha', choice);
            await expect.poll(async () => await image(page) !== before).toBe(true);
            const dimensions = await inspect(page);
            expect([dimensions.width, dimensions.height]).toEqual(orientation.startsWith('Portrait') ? [320, 480] : [480, 320]);
          }
        }
      }
      await choose(page, 'Orientation', 'Landscape'); await choose(page, 'Display style', 'Gauges');
      await select('alpha', 'Both');
      const previous = await image(page), good = readFileSync(settingsFile, 'utf8');
      try {
        writeFileSync(settingsFile, '{broken');
        const failed = page.waitForResponse(r => r.url().endsWith('/wails/runtime') && r.request().postDataJSON()?.args?.methodName === contentMethod);
        await serviceToggle('alpha').focus(); await serviceToggle('alpha').press(' ');
        expect((await failed).ok()).toBe(false);
        await expect(serviceToggle('alpha')).toBeChecked();
        await expect(serviceToggle('alpha')).toBeEnabled();
        await expect(group('alpha').getByRole('radio', { name: 'Both', exact: true })).toBeChecked();
        await expect(group('alpha').getByRole('radio').first()).toBeEnabled();
        expect(await image(page)).toBe(previous);
        await toggle().click();
        await expect(page.getByText(/saved settings cannot be read/).first()).toBeVisible();
      } finally { writeFileSync(settingsFile, good); }
      await page.reload(); await toggle().click();
    });
    await test.step('手順5', async () => {
      const meter = page.getByRole('checkbox', { name: 'alpha Low A 5h', exact: true });
      await meter.click(); await expect(meter).toHaveAttribute('aria-checked', 'false');
      const hidden = saved().hiddenLimits;
      await select('alpha', 'Limits'); await select('alpha', 'Off');
      await meter.click(); await expect(meter).toHaveAttribute('aria-checked', 'true');
      await expect(serviceToggle('alpha')).not.toBeChecked();
      await expect(group('alpha').getByRole('radio', { name: 'Limits', exact: true })).toBeChecked();
      await meter.click(); await expect(meter).toHaveAttribute('aria-checked', 'false');
      await select('alpha', 'Tokens');
      expect(saved().hiddenLimits).toEqual(hidden);
      hub.send('stats', { ...payload, limits: { providers: [last] } });
      await expect(page.getByText('Low', { exact: true })).toHaveCount(0);
      // alpha remains listed by its token attribution; absent persisted IDs are kept too.
      await expect(group('alpha').getByRole('radio', { name: 'Tokens', exact: true })).toBeChecked();
      hub.send('stats', payload);
      await expect(meter).toHaveAttribute('aria-checked', 'false');
      expect(saved().compactServiceContent.absent).toMatchObject({ showLimits: false });
      expect(saved().compactServiceContent.absent.showTokens ?? true).toBe(true);
    });
    await test.step('手順6', async () => {
      for (const [width, height] of [[1330, 880], [1330, 500], [965, 600], [964, 600], [825, 600], [824, 600], [480, 640], [320, 568]]) {
        await page.setViewportSize({ width, height });
        await group(longID).evaluate(el => el.scrollIntoView({ block: 'center' })); await expect(group(longID)).toBeInViewport({ ratio: 1 });
        await group('zero').evaluate(el => el.scrollIntoView({ block: 'center' })); await expect(group('zero')).toBeInViewport({ ratio: 1 });
        await page.getByRole('checkbox').last().evaluate(el => el.scrollIntoView({ block: 'center' }));
        await expect(page.getByRole('checkbox').last()).toBeInViewport({ ratio: 1 });
        const outer = await page.getByRole('heading', { name: '3.5-inch display', exact: true }).locator('..').boundingBox();
        const cards = await page.getByRole('radiogroup').evaluateAll(es => es.map(e => {
          const r = e.parentElement!.getBoundingClientRect(); return { x: r.x, y: r.y, right: r.right, bottom: r.bottom };
        }));
        for (const [i, card] of cards.entries()) {
          expect(card.x).toBeGreaterThanOrEqual(outer!.x); expect(card.right).toBeLessThanOrEqual(outer!.x + outer!.width);
          expect(card.bottom).toBeLessThanOrEqual(outer!.y + outer!.height);
          for (const other of cards.slice(i + 1)) expect(card.right <= other.x || other.right <= card.x || card.bottom <= other.y || other.bottom <= card.y).toBe(true);
        }
        const layout = await group(longID).evaluate(el => {
          const grid = el.parentElement!.parentElement!;
          const last = el.getBoundingClientRect();
          return { horizontalOverflow: document.documentElement.scrollWidth > innerWidth, groupWidth: last.width, gridWidth: grid.getBoundingClientRect().width,
            columns: getComputedStyle(grid).gridTemplateColumns.split(' ').length };
        });
        expect(layout.horizontalOverflow).toBe(false);
        expect(layout.groupWidth).toBeGreaterThan(0);
        expect(layout.columns).toBe(layout.gridWidth >= 660 ? 2 : 1);
        const orientation = await page.getByRole('textbox', { name: 'Orientation', exact: true }).boundingBox();
        const interval = await page.getByRole('textbox', { name: 'Rotation interval (seconds)' }).boundingBox();
        expect(Math.abs(orientation!.x - interval!.x) > 1).toBe(layout.gridWidth >= 520);
      }
      await page.setViewportSize({ width: 1330, height: 880 });
    });
    await test.step('手順7', async () => {
      await select('alpha', 'Off');
      expect(saved().compactServiceContent.alpha).toEqual({ enabled: false, showLimits: false, showTokens: true });
      await server.stop(); await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34129, isolated.env);
      await expect.poll(() => hub.streams()).toBe(1); hub.send('snapshot', payload);
      await page.goto(server.url); await expect(toggle()).toHaveAttribute('aria-expanded', 'false');
      await toggle().click();
      await expect(serviceToggle('alpha')).not.toBeChecked();
      await expect(group('alpha').getByRole('radio', { name: 'Tokens', exact: true })).toBeChecked();
      await expect(group('alpha').getByRole('radio').first()).toBeDisabled();
      await expect(page.getByRole('checkbox', { name: 'alpha Low A 5h', exact: true })).toHaveAttribute('aria-checked', 'false');
      await select('alpha', 'Tokens');
    });
    await test.step('受け入れ条件', async () => {
      for (const name of names) await select(name, 'Off');
      const off = await image(page);
      await select('alpha', 'Both');
      hub.send('stats', { ...payload, limits: { providers: [] } });
      await expect(page.getByRole('checkbox')).toHaveCount(0);
      await expect.poll(() => image(page)).not.toBe(off);
      const values = await tokenValuePixels(page);
      payload.periods.today.clients.alpha++;
      hub.send('stats', { ...payload, limits: { providers: [] } });
      await expect.poll(async () => (await tokenValuePixels(page))[0]).not.toBe(values[0]);
      const populated = await image(page);
      // No matching ID: no fallback to whole totals or differently cased service IDs.
      hub.send('stats', { periods: { today: { totalTokens: 123, costUsd: 456, clients: { Alpha: 999 }, clientCosts: { Alpha: 999 } } }, limits: { providers: [{ provider: 'alpha', windows: [] }] } });
      await expect.poll(() => image(page)).not.toBe(populated);
      const unknown = await tokenValuePixels(page);
      hub.send('stats', { periods: { today: { clients: { alpha: 0 }, clientCosts: { alpha: 0 } } }, limits: { providers: [{ provider: 'alpha', windows: [] }] } });
      await expect.poll(async () => (await tokenValuePixels(page))[0]).not.toBe(unknown[0]);
      await expect.poll(async () => (await tokenValuePixels(page))[1]).not.toBe(unknown[1]);
      expect((await tokenValuePixels(page)).slice(2)).toEqual(unknown.slice(2));
    });
  } finally {
    await server.stop(); await hub.close();
    rmSync(dataDir, { recursive: true, force: true }); rmSync(isolated.home, { recursive: true, force: true });
  }
});

test('Localの既知IDを統合し、設定を引き継ぎ、Hubの完全一致と同じサービス別Tokensを表示する', async ({ page }) => {
  test.setTimeout(180_000);
  const root = resolve(import.meta.dirname, '../../../../..');
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-service-ids-e2e-'));
  const isolated = localHome();
  const execDir = join(dataDir, 'bin'); mkdirSync(execDir);
  const serverExecutable = join(execDir, 'server.exe');
  copyFileSync(join(root, 'bin/token-monitor-turzx-server.exe'), serverExecutable);
  execFileSync('go', ['test', '-c', '-o', join(execDir, 'tokscale.exe'), './internal/localusage'], { cwd: root, windowsHide: true });
  const write = (name: string, value: unknown) => writeFileSync(join(dataDir, name), JSON.stringify(value));
  writeFileSync(join(dataDir, 'fake-tokscale'), ''); mkdirSync(join(dataDir, 'logs'));
  // Literal CLI IDs and values are independent of the application's correspondence table.
  const pairs = [
    { provider: 'Claude', client: 'claude', tokens: 12345, cost: 1.25 },
    { provider: 'Codex', client: 'codex', tokens: 67890, cost: 6.78 },
    { provider: 'Amp', client: 'amp', tokens: 111, cost: 0.11 },
    { provider: 'Antigravity', client: 'antigravity', tokens: 222, cost: 0.22 },
    { provider: 'Copilot', client: 'copilot', tokens: 0, cost: 0 },
    { provider: 'Grok Build', client: 'grok', tokens: 444, cost: 0.44 },
    { provider: 'Kimi', client: 'kimi', tokens: 555 },
    { provider: 'Warp/Oz', client: 'warp', tokens: 666, cost: 0.66 },
  ];
  const providers = [
    { provider: 'Codex', email: 'one@example.test', plan: 'Plus', metrics: [{ label: 'Session', remaining_percent: 26 }, { label: 'Weekly', remaining_percent: 62 }] },
    { provider: 'Claude', email: 'claude@example.test', plan: 'Pro', metrics: [{ label: 'Session', remaining_percent: 31 }] },
    { provider: 'Codex', email: 'two@example.test', plan: 'Team', metrics: [{ label: 'Weekly', remaining_percent: 90 }] },
    ...['Antigravity', 'Copilot', 'Grok Build', 'Kimi', 'Warp/Oz', 'OpenCode Go', 'Foo'].map(provider => ({ provider, email: '', plan: '', metrics: [{ label: 'Session', remaining_percent: 40 }] })),
  ];
  const now = new Date(), date = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
  const extras = [{ client: 'antigravity-cli', tokens: 777, cost: 0.77 }, { client: 'opencode', tokens: 888, cost: 0.88 }, { client: 'foo', tokens: 999, cost: 0.99 }];
  const clients = [...pairs, ...extras];
  write('graph.json', { summary: { clients: clients.map(c => c.client) }, contributions: [{ date,
    tokenBreakdown: { input: 9999999 }, totals: { cost: 99.99 },
    clients: clients.map(c => ({ client: c.client, tokens: { input: c.tokens }, cost: c.cost })),
  }] });
  write('usage.json', providers);
  write('clients.json', { clients: [{ client: 'codex', sessionsPath: join(dataDir, 'logs') }] });
  write('cursor.json', { synced: false, rows: 0, error: 'Not authenticated' });
  const names = ['Codex', 'Claude', 'Antigravity', 'Copilot', 'Grok Build', 'Kimi', 'Warp/Oz', 'OpenCode Go', 'Foo', 'Amp', 'antigravity-cli', 'foo', 'opencode'];
  const choices = Object.fromEntries(names.filter(n => n !== 'Claude').map(n => [n, { enabled: false, showLimits: true, showTokens: true }]));
  Object.assign(choices, { Codex: { enabled: true, showLimits: false, showTokens: true }, codex: { enabled: false, showLimits: false, showTokens: true }, claude: { enabled: true, showLimits: true, showTokens: false } });
  write('settings.json', { source: 'Local', displayID: compactID, rotationIntervalSeconds: 5, compactServiceContent: choices });
  const settingsFile = join(dataDir, 'settings.json');
  const saved = () => JSON.parse(readFileSync(settingsFile, 'utf8'));
  const hub = await startHub();
  let server = await startServer(dataDir, 34130, isolated.env, serverExecutable);
  const group = (name: string) => page.getByRole('radiogroup', { name: `${name} display content`, exact: true });
  const toggle = (name: string) => page.getByRole('switch', { name: `${name} display enabled`, exact: true });
  const expand = async () => {
    const fold = page.getByRole('button', { name: 'Service content', exact: true });
    if (await fold.getAttribute('aria-expanded') === 'false') await fold.click();
  };
  const enable = async (name: string, enabled: boolean) => {
    if (await toggle(name).isChecked() === enabled) return;
    await toggle(name).focus(); await toggle(name).press(' ');
    await expect(toggle(name)).toBeChecked({ checked: enabled }); await expect(toggle(name)).toBeEnabled();
  };
  const content = async (name: string, choice: string) => {
    if (await group(name).getByRole('radio', { name: choice, exact: true }).isChecked()) return;
    await group(name).getByText(choice, { exact: true }).click();
    await expect(group(name).getByRole('radio', { name: choice, exact: true })).toBeChecked(); await expect(toggle(name)).toBeEnabled();
  };
  const offImage = async () => {
    // Switching ON then OFF starts from a known service page, so wait for the empty image.
    await enable('Codex', true); await content('Codex', 'Tokens');
    await expect.poll(() => image(page)).not.toBe(empty);
    const on = await image(page); await enable('Codex', false);
    await expect.poll(() => image(page)).not.toBe(on); return image(page);
  };
  const localImages = new Map<string, string>();
  let empty = '';
  try {
    await test.step('分岐条件', async () => { await page.goto(server.url); await expect(page.getByRole('textbox', { name: 'Output device' })).toHaveValue(/TURZX 3.5-inch/); });
    await test.step('手順1', async () => {
      await expect(page.getByText('Plus', { exact: true })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Service content', exact: true })).toHaveAttribute('aria-expanded', 'false');
    });
    await test.step('手順2', async () => {
      await expand(); await expect(page.getByRole('radiogroup')).toHaveCount(13);
      expect(await page.getByRole('radiogroup').evaluateAll(es => es.map(e => e.getAttribute('aria-label')))).toEqual(names.map(n => `${n} display content`));
      await expect(toggle('Codex')).toBeChecked(); await expect(group('Codex').getByRole('radio', { name: 'Tokens', exact: true })).toBeChecked();
      await expect(toggle('Claude')).toBeChecked(); await expect(group('Claude').getByRole('radio', { name: 'Limits', exact: true })).toBeChecked();
      expect(saved().compactServiceContent.Claude).toBeUndefined(); // Reading never migrates old keys.
    });
    await test.step('手順3', async () => {
      await enable('Claude', false); await expect.poll(() => inspect(page)).toMatchObject({ width: 480, height: 320 });
      await page.waitForTimeout(250); const only = await image(page);
      await page.waitForTimeout(5300); expect(await image(page)).toBe(only); // Two Codex contracts, one Tokens page.
      const on = await image(page); await enable('Codex', false); await expect.poll(() => image(page)).not.toBe(on); empty = await image(page);
      for (const pair of pairs) {
        await enable(pair.provider, true); await content(pair.provider, 'Tokens');
        await expect.poll(() => image(page)).not.toBe(empty); localImages.set(pair.provider, await image(page));
        await enable(pair.provider, false); await expect.poll(() => image(page)).toBe(empty);
      }
    });
    await test.step('手順4', async () => {
      await enable('Codex', true); await content('Codex', 'Both');
      await expect.poll(async () => (await inspect(page)).counts.some(n => n > 20)).toBe(true);
      await content('Codex', 'Tokens'); await expect.poll(() => image(page)).toBe(localImages.get('Codex'));
      const before = readFileSync(settingsFile), preview = await image(page);
      // Real atomic replacement failure, rather than a simulated success or HTTP stub.
      chmodSync(settingsFile, 0o444);
      try {
        const response = page.waitForResponse(r => r.url().endsWith('/wails/runtime') && r.request().postDataJSON()?.args?.methodName === 'token-monitor-turzx/internal/display.Service.SetServiceContent');
        await group('Codex').getByText('Limits', { exact: true }).click(); expect((await response).ok()).toBe(false);
        await expect(page.getByRole('alert')).toBeVisible(); await expect(group('Codex').getByRole('radio', { name: 'Tokens', exact: true })).toBeChecked();
        expect(readFileSync(settingsFile)).toEqual(before); expect(await image(page)).toBe(preview);
      } finally { chmodSync(settingsFile, 0o600); }
    });
    await test.step('手順5', async () => {
      const meter = page.getByRole('checkbox', { name: 'Codex Plus Session', exact: true });
      await meter.click(); await expect(meter).toHaveAttribute('aria-checked', 'false');
      expect(saved().hiddenLimits).toContain('Codex\u001fone@example.test\u001fsession\u001fSession');
      await expect(group('Codex').getByRole('radio', { name: 'Tokens', exact: true })).toBeChecked();
      expect(await image(page)).toBe(localImages.get('Codex'));
      await expect(group('codex')).toHaveCount(0);
    });
    await test.step('手順6', async () => { await page.reload(); await expand(); await expect(group('Codex')).toBeVisible(); });
    await test.step('手順7', async () => {
      await enable('Codex', false); const before = saved();
      await server.stop(); server = await startServer(dataDir, 34130, isolated.env, serverExecutable);
      await page.goto(server.url); await expand(); await expect(group('Codex')).toBeVisible();
      await expect(toggle('Codex')).not.toBeChecked(); await expect(group('Codex').getByRole('radio', { name: 'Tokens', exact: true })).toBeChecked();
      await expect(page.getByRole('checkbox', { name: 'Codex Plus Session', exact: true })).toHaveAttribute('aria-checked', 'false');
      expect(saved().compactServiceContent.codex).toEqual(before.compactServiceContent.codex);
      expect(saved().compactServiceContent.claude).toEqual(before.compactServiceContent.claude);
      // Whole-record precedence, missing/null defaults and old Off are read through main's real adapter.
      for (const tc of [
        { canonical: {}, alias: { enabled: false, showLimits: false, showTokens: true }, enabled: true, choice: 'Both' },
        { canonical: { showLimits: false }, alias: { enabled: false, showLimits: true, showTokens: false }, enabled: true, choice: 'Tokens' },
        { canonical: { showLimits: false, showTokens: false }, alias: { enabled: true, showTokens: true }, enabled: false, choice: 'Both' },
        { canonical: undefined, alias: { enabled: null, showLimits: false, showTokens: false }, enabled: false, choice: 'Both' },
        { canonical: { enabled: true, showLimits: false, showTokens: false }, alias: {}, enabled: true, choice: 'Both' },
        { canonical: undefined, alias: { enabled: null, showLimits: null, showTokens: null }, enabled: true, choice: 'Both' },
      ]) {
        await server.stop(); const next = saved(); next.compactServiceContent.Codex = tc.canonical; next.compactServiceContent.codex = tc.alias; write('settings.json', next);
        server = await startServer(dataDir, 34130, isolated.env, serverExecutable); await page.goto(server.url); await expand();
        await expect(toggle('Codex')).toBeChecked({ checked: tc.enabled }); await expect(group('Codex').getByRole('radio', { name: tc.choice, exact: true })).toBeChecked();
        expect(saved().compactServiceContent.Codex).toEqual(tc.canonical); expect(saved().compactServiceContent.codex).toEqual(tc.alias);
      }
      await enable('Codex', false); empty = await offImage();
    });
    await test.step('受け入れ条件', async () => {
      // The same raw case differences stay separate when supplied by Hub.
      await page.getByRole('link', { name: 'Connection' }).click();
      await page.getByRole('textbox', { name: 'Data source', exact: true }).click(); await page.getByRole('option', { name: 'Hub', exact: true }).click();
      await page.getByLabel('Hub URL').fill(hub.url); await page.getByLabel('Access token').fill('local-ids-e2e-token');
      await page.getByRole('button', { name: 'Save', exact: true }).click(); await expect.poll(() => hub.streams()).toBe(1);
      const rawPeriod = { totalTokens: 9999999, costUsd: 99.99, clients: Object.fromEntries(clients.map(c => [c.client, c.tokens])), clientCosts: Object.fromEntries(clients.filter(c => c.cost !== undefined).map(c => [c.client, c.cost])) };
      const limits = { providers: providers.map(p => ({ provider: p.provider, accountLabel: p.email, planLabel: p.plan, windows: [] })) };
      hub.send('snapshot', { periods: { today: rawPeriod, month: rawPeriod, allTime: rawPeriod }, limits });
      await page.getByRole('link', { name: 'Display' }).click(); await expand(); await expect(page.getByRole('radiogroup')).toHaveCount(20);
      for (const n of ['Codex', 'codex', 'Claude', 'claude', 'Antigravity', 'antigravity-cli', 'OpenCode Go', 'opencode', 'Foo', 'foo']) await expect(group(n)).toHaveCount(1);
      for (const n of await page.getByRole('radiogroup').evaluateAll(es => es.map(e => e.getAttribute('aria-label')!.replace(/ display content$/, '')))) await enable(n, false);
      await enable('Codex', true); await content('Codex', 'Tokens');
      await expect.poll(async () => (await inspect(page)).panel).toEqual([21, 24, 33]);
      await expect.poll(() => image(page)).not.toBe(localImages.get('Codex')); // Hub does not look up lower-case Tokens for Codex.
      await enable('Codex', false);
      // Canonical Hub IDs provide an independent exact-match reference for all six values,
      // including explicit 0, absent cost and tool-only Amp, for each of the eight Local pairs.
      const canonicalPeriod = { ...rawPeriod, clients: Object.fromEntries(pairs.map(c => [c.provider, c.tokens])), clientCosts: Object.fromEntries(pairs.filter(c => c.cost !== undefined).map(c => [c.provider, c.cost])) };
      hub.send('stats', { periods: { today: canonicalPeriod, month: canonicalPeriod, allTime: canonicalPeriod }, limits });
      await expect(group('Amp')).toBeVisible(); await expect(group('codex')).toHaveCount(0);
      empty = await offImage();
      for (const pair of pairs) {
        await enable(pair.provider, true); await content(pair.provider, 'Tokens');
        await expect.poll(() => image(page)).toBe(localImages.get(pair.provider));
        await enable(pair.provider, false); await expect.poll(() => image(page)).toBe(empty);
      }
      const calls = readFileSync(join(dataDir, 'calls.log'), 'utf8').split('\n').filter(line => line.startsWith('start|graph '));
      expect(calls.length).toBeGreaterThan(0); expect(calls.every(line => line.split('|')[1] === 'graph --no-spinner')).toBe(true);
    });
  } finally {
    await server.stop(); await hub.close();
    rmSync(dataDir, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); rmSync(isolated.home, { recursive: true, force: true });
  }
});
