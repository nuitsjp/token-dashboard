import { test, expect, type Page } from '@playwright/test';
import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { FrameRequest, ThemeData } from '../../../../bindings/token-monitor-turzx/internal/display/models';
import type { View } from '../../../../bindings/token-monitor-turzx/internal/settings/models';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { localHome } from '../../support/local';
import { checkPNGShift } from '../../support/shift';

// A distinct hardware suffix keeps this saved selection disconnected when the real device is plugged in.
const smallID = 'USB\\VID_1A86&PID_5722&E2E\\USB35INCHIPSV2';
const settingsGet = 'token-monitor-turzx/internal/settings.Service.Get';
const display = 'token-monitor-turzx/internal/display.Service.';
const skipName = 'Skip full 5h or missing limits';
test.use({ actionTimeout: 15_000 });
const styles = [
  ['Gauges 3.5" Landscape', 'gauges-35-landscape', 480, 320],
  ['Gauges 3.5" Portrait', 'gauges-35-portrait', 320, 480],
  ['Bars 3.5" Landscape', 'bars-35-landscape', 480, 320],
  ['Bars 3.5" Portrait', 'bars-35-portrait', 320, 480],
] as const;

async function rpc<T>(page: Page, method: string, args: unknown[] = []): Promise<T> {
  return page.evaluate(async ({ method, args }) => {
    const response = await fetch('/wails/runtime', {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'x-wails-client-id': 'service-display-e2e' },
      body: JSON.stringify({ object: 0, method: 0, args: { 'call-id': crypto.randomUUID(), methodName: method, args } }),
    });
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  }, { method, args });
}

function stats(remaining = 20) {
  const window = (label: string, kind: string, remainingPercent: number | null) => ({ label, kind, remainingPercent, showMeter: true });
  const period = { totalTokens: 987654, costUsd: 99, clients: { Claude: 0, Codex: 123456, Copilot: 10, 'antigravity-cli': 20 }, clientCosts: { Claude: 0, Codex: 1.25 } };
  return {
    periods: { today: period, month: period, allTime: period },
    limits: { providers: [
      { provider: 'Claude', accountLabel: 'one', planLabel: 'Pro', windows: [window('Alpha 5h', 'session', 100), window('Beta 5h', 'session', 100), window('Gamma Weekly', 'weekly', 80), window('Delta Weekly', 'weekly', null)] },
      { provider: 'Codex', accountLabel: 'two', planLabel: 'Low', windows: [window('5h', 'session', remaining), window('Weekly', 'weekly', 90)] },
      { provider: 'Codex', accountLabel: 'three', planLabel: 'High', windows: [window('5h', 'session', 80)] },
    ] },
  };
}

test('3.5インチのスタイル、サービス別巡回、スキップと保存を維持する', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-services-e2e-'));
  const settingsFile = join(dataDir, 'settings.json');
  const { home, env } = localHome();
  const hub = await startHub();
  // A saved explicit selection remains selectable while unplugged. No physical USB is opened.
  writeFileSync(settingsFile, JSON.stringify({ source: 'Local', displayID: smallID, displayName: 'TURZX 3.5"', limitStyle: 'Bars', cycle: false, cycleSeconds: 37 }));
  let server = await startServer(dataDir, 34137, env);
  let frame: FrameRequest | undefined;
  let completed: { request: FrameRequest; png: string; device: string } | undefined;
  const requests = new Map<number, FrameRequest>();
  const observe = (target: Page) => target.on('response', async response => {
    if (!response.url().endsWith('/wails/runtime')) return;
    try {
      const call = response.request().postDataJSON()?.args;
      if (call?.methodName === display + 'RenderRequest' && response.ok()) {
        const request = await response.json() as FrameRequest | null;
        if (request) { frame = request; requests.set(request.id, request); }
      }
      if (call?.methodName === display + 'CompleteFrame' && response.ok() && call.args[1] && call.args[2] && !call.args[3]) {
        const request = requests.get(call.args[0]);
        if (request) completed = { request, png: call.args[1], device: call.args[2] };
      }
    } catch { /* Ignore unrelated runtime responses. */ }
  });
  observe(page);
  const file = () => JSON.parse(readFileSync(settingsFile, 'utf8'));
  const preview = () => page.getByRole('img', { name: 'Display preview', exact: true });
  const interval = () => page.getByRole('textbox', { name: 'Page interval', exact: true });
  const setPageInterval = async (value: number, blur = false) => {
    await interval().focus(); await interval().press('Control+a'); await interval().pressSequentially(String(value));
    await interval().press(blur ? 'Tab' : 'Enter');
    await expect.poll(() => file().serviceCycleSeconds).toBe(value);
    await expect(interval()).toBeEnabled();
  };
  const selectStyle = async (label: string, id: string, width: number, height: number) => {
    await page.getByRole('textbox', { name: 'Display style' }).click();
    await page.getByRole('option', { name: label, exact: true }).click();
    await expect.poll(() => file().limitStyle).toBe(id);
    // Reopening the server page can leave one old Canvas request pending until its 10s deadline.
    await expect.poll(() => completed?.request.theme, { timeout: 20_000 }).toBe(id);
    await expect.poll(() => preview().evaluate((img: HTMLImageElement) => [img.naturalWidth, img.naturalHeight])).toEqual([width, height]);
    await expect(interval()).toHaveValue(`${file().serviceCycleSeconds ?? 10} s`);
  };
  const openServices = async () => {
    const summary = page.locator('summary').filter({ hasText: 'Service content' });
    if (!(await summary.locator('..').evaluate(el => el.hasAttribute('open')))) await summary.click();
  };
  const content = (name: string) => page.getByRole('article', { name: `${name} content`, exact: true });
  try {
    await test.step('分岐条件', async () => {
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Connection', exact: true }).click();
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Hub', exact: true }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel(/Access token/).fill('service-e2e-token');
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect(page.getByText('Saved.', { exact: true })).toBeVisible();
      await expect.poll(() => hub.streams()).toBe(1);
      await page.getByRole('link', { name: 'Display', exact: true }).click();
    });
    await test.step('手順1', async () => {
      await expect(page.getByRole('textbox', { name: 'Output device' })).toHaveValue(/3.5.*Disconnected/);
      await page.getByRole('textbox', { name: 'Display style' }).click();
      await expect(page.getByRole('option')).toHaveCount(4);
      await expect(page.getByRole('option', { name: 'Gauges', exact: true })).toHaveCount(0);
      await page.keyboard.press('Escape');
      await expect(page.getByText(/Selected style size is not supported/)).toHaveCount(0);
      await expect(page.getByRole('textbox', { name: 'Display style' })).toHaveValue('Gauges 3.5" Landscape');
      expect(file().limitStyle).toBe('gauges-35-landscape');
      const select = page.getByRole('textbox', { name: 'Display style' });
      await expect(select).toBeEnabled();
      const appearance = await select.evaluate(input => ({ color: getComputedStyle(input).color, text: getComputedStyle(document.body).color, cursor: getComputedStyle(input).cursor }));
      expect(appearance.color).toBe(appearance.text);
      expect(appearance.cursor).toBe('pointer');
      await select.focus(); await select.press('ArrowDown');
      await expect(page.getByRole('option')).toHaveCount(4);
      expect(await select.evaluate(input => getComputedStyle(input).outlineStyle)).toBe('solid');
      await page.keyboard.press('Escape');
      const saved = readFileSync(settingsFile);
      const generation = (await rpc<View>(page, settingsGet)).generation;
      for (let i = 0; i < 3; i++) expect((await rpc<View>(page, settingsGet)).generation).toBe(generation);
      expect(readFileSync(settingsFile).equals(saved)).toBe(true);
    });
    await test.step('手順2', async () => {
      for (const [label, id, width, height] of styles) {
        await selectStyle(label, id, width, height);
        expect(completed!.request.outputFormat).toBe('png');
        expect(completed!.request.data.available).toBe(false);
      }
      await selectStyle(...styles[0]);
      // The dedicated default is available before any usage, independent of the old 37-second setting.
      await expect(interval()).toHaveValue('10 s');
      await expect(page.getByText('Pages switch automatically when there is more than one page.', { exact: true })).toBeVisible();
      const selectBox = (await page.getByRole('textbox', { name: 'Display style' }).boundingBox())!;
      const intervalBox = (await interval().boundingBox())!;
      const previewBox = (await preview().boundingBox())!;
      expect(intervalBox.y).toBeGreaterThan(selectBox.y + selectBox.height);
      expect(intervalBox.y + intervalBox.height).toBeLessThan(previewBox.y);
      await page.getByRole('switch', { name: 'Rotate 180°', exact: true }).click();
      await expect.poll(() => file().presentation.reverse180).toBe(true);
      await expect.poll(() => completed?.request.reverse180).toBe(true);
      // The PNG stays upright and carries only the burn-in offset; the device applies the rotation.
      await checkPNGShift(page, [completed!]);
    });
    await test.step('手順3', async () => {
      hub.send('snapshot', stats());
      await expect.poll(() => frame?.data.service?.provider).toBe('Claude');
      await expect.poll(() => frame?.data.service?.count).toBe(6);
      expect(frame!.data.service!.tokens![0].tokensText).toBe('0');
      expect(frame!.data.service!.tokens![0].costText).toBe('$0.00');
      expect(frame!.data.service!.circles!.length).toBe(3);
      expect(file().cycle ?? false).toBe(false);
      const firstPage = frame!.data.service!.key;
      await expect.poll(() => frame?.data.service?.key, { timeout: 5_000 }).not.toBe(firstPage);
    });
    await test.step('手順4', async () => {
      const summary = page.locator('summary').filter({ hasText: 'Service content' });
      await expect(summary.locator('..')).not.toHaveAttribute('open');
      await summary.focus(); await page.keyboard.press('Enter');
      await expect(content('Claude')).toBeVisible();
      await content('Claude').getByText('Tokens', { exact: true }).click();
      await expect.poll(() => file().presentation?.services?.Hub?.Claude?.content).toBe('Tokens');
      const showClaude = page.getByRole('switch', { name: 'Show Claude', exact: true });
      await showClaude.click();
      // Saving also disables the radios; wait for the saved service state before toggling again.
      await expect.poll(() => file().presentation?.services?.Hub?.Claude?.enabled).toBe(false);
      await expect(showClaude).toHaveAttribute('aria-checked', 'false');
      await expect(content('Claude').getByRole('radio', { name: 'Tokens', exact: true })).toBeDisabled();
      await expect(content('Claude').getByRole('radio', { name: 'Tokens', exact: true })).toBeChecked();
      await showClaude.click();
      await expect(showClaude).toHaveAttribute('aria-checked', 'true');
      await expect(content('Claude').getByRole('radio', { name: 'Both', exact: true })).toBeEnabled();
      await content('Claude').getByText('Both', { exact: true }).click();
      await expect.poll(() => file().presentation?.services?.Hub?.Claude?.content).toBe('Both');
      await expect.poll(() => frame?.data.service?.count).toBe(6);

      const originalInterval = file().serviceCycleSeconds ?? 10;
      await setPageInterval(3600);
      for (const [label, id, width, height] of styles) {
        await selectStyle(label, id, width, height);
        await expect.poll(() => frame?.data.service?.provider).toBe('Claude');
        await expect.poll(() => frame?.data.service?.number).toBe(1);

        for (const selected of ['Limits', 'Both'] as const) {
          const previousID = completed!.request.id;
          await content('Claude').getByText(selected, { exact: true }).click();
          await expect.poll(() => file().presentation?.services?.Hub?.Claude?.content).toBe(selected);
          await expect.poll(() => {
            const request = completed?.request;
            const service = request?.data.service;
            return request !== undefined && request.id > previousID && request.theme === id && request.data.available
              && service?.provider === 'Claude' && service.showTokens === (selected === 'Both') && service.showLimits;
          }).toBe(true);

          const service = completed!.request.data.service!;
          expect(completed!.request.data.available).toBe(true);
          expect(service).toMatchObject({
            provider: 'Claude', plan: 'Pro', number: 1, count: 6,
            showTokens: selected === 'Both', showLimits: true, both: selected === 'Both',
          });
          const expectedLabels = ['Alpha 5h', 'Beta 5h', 'Gamma Weekly'];
          if (id.startsWith('gauges')) {
            expect(service.circles).toHaveLength(3);
            expect(service.circles!.map(circle => circle.name)).toEqual(['Alpha', 'Beta', 'Gamma']);
            expect(service.circles!.flatMap(circle => circle.windows ?? []).map(window => window.label)).toEqual(expectedLabels);
          } else {
            expect(service.windows).toHaveLength(3);
            expect(service.windows!.map(window => window.label)).toEqual(expectedLabels);
          }
        }
      }
      await selectStyle(...styles[0]);
      await expect.poll(() => frame?.data.service?.provider).toBe('Claude');
      await expect.poll(() => file().presentation?.services?.Hub?.Claude?.content).toBe('Both');
      await setPageInterval(originalInterval);
    });
    await test.step('手順5', async () => {
      await setPageInterval(5, true);
      for (const invalid of ['4', '3601', '', '1.5']) {
        const before = readFileSync(settingsFile);
        await interval().fill(invalid); await interval().press('Enter');
        await expect(interval()).toHaveValue('5 s');
        expect(readFileSync(settingsFile).equals(before)).toBe(true);
      }
      await setPageInterval(3600);
      const held = frame!;
      await setPageInterval(50);
      await expect.poll(() => frame?.id).toBeGreaterThan(held.id);
      expect(frame!.data.service!.key).toBe(held.data.service!.key);
      expect(file().cycleSeconds).toBe(37);

      const before = readFileSync(settingsFile), view = await rpc<View>(page, settingsGet);
      chmodSync(settingsFile, 0o444);
      try {
        await interval().fill('60'); await interval().press('Enter');
        const error = page.getByRole('alert');
        await expect(error).toBeVisible(); await expect(interval()).toHaveValue('50 s');
        expect(readFileSync(settingsFile).equals(before)).toBe(true);
        expect((await rpc<View>(page, settingsGet)).generation).toBe(view.generation);
        const fieldBox = (await interval().boundingBox())!, errorBox = (await error.boundingBox())!, imageBox = (await preview().boundingBox())!;
        expect(errorBox.y).toBeGreaterThan(fieldBox.y);
        expect(errorBox.y).toBeLessThan(imageBox.y);
      } finally { chmodSync(settingsFile, 0o600); }
      // A retry succeeds and changes the active interval while preserving the current page.
      await setPageInterval(51); await setPageInterval(50, true);
      await expect(page.getByRole('alert')).toHaveCount(0);
      await expect(page.getByText('Saved.', { exact: true })).toHaveCount(0);
      const current = frame!.data.service!.key;
      await page.getByRole('link', { name: 'Settings', exact: true }).click();
      await page.getByRole('textbox', { name: 'Cycle time' }).fill('70');
      await page.getByRole('textbox', { name: 'Cycle time' }).press('Enter');
      await expect.poll(() => file().cycleSeconds).toBe(70);
      await page.getByRole('switch', { name: 'Cycle', exact: true }).click();
      await expect.poll(() => file().cycle).toBe(true);
      await page.getByRole('switch', { name: 'Cycle', exact: true }).click();
      await expect.poll(() => file().cycle ?? false).toBe(false);
      expect(file().serviceCycleSeconds).toBe(50);
      await page.getByRole('link', { name: 'Display', exact: true }).click();
      await expect(interval()).toHaveValue('50 s');
      await expect.poll(() => frame?.data.service?.key, { timeout: 8_000 }).not.toBe(current);
      // One enabled service also cycles when one contract exceeds the page capacity.
      await openServices();
      for (const id of ['Codex', 'Copilot', 'antigravity-cli']) {
        await page.getByRole('switch', { name: `Show ${id}`, exact: true }).click();
        await expect(page.getByRole('switch', { name: `Show ${id}`, exact: true })).toHaveAttribute('aria-checked', 'false');
      }
      await expect.poll(() => frame?.data.service?.count).toBe(2);
      expect(frame!.data.service!.provider).toBe('Claude');
      await setPageInterval(5);
      const overflowPage = frame!.data.service!.key;
      await expect.poll(() => frame?.data.service?.key, { timeout: 4_000 }).not.toBe(overflowPage);
      expect(frame!.data.service!.provider).toBe('Claude');
      for (const id of ['Codex', 'Copilot', 'antigravity-cli']) {
        await page.getByRole('switch', { name: `Show ${id}`, exact: true }).click();
        await expect(page.getByRole('switch', { name: `Show ${id}`, exact: true })).toHaveAttribute('aria-checked', 'true');
      }
      await expect.poll(() => frame?.data.service?.count).toBe(6);
      await setPageInterval(50);
    });
    await test.step('手順6', async () => {
      const current = frame!;
      hub.send('stats', stats(21));
      await expect.poll(() => frame?.id).toBeGreaterThan(current.id);
      expect(frame!.data.service!.key).toBe(current.data.service!.key);
      await expect(page.getByRole('checkbox', { name: 'Codex Low 5h', exact: true })).toContainText('21%');
    });
    await test.step('手順7', async () => {
      const services = JSON.stringify(file().presentation.services);
      await page.getByRole('checkbox', { name: 'Claude Pro Alpha 5h', exact: true }).click();
      await expect(page.getByRole('checkbox', { name: 'Claude Pro Alpha 5h', exact: true })).toHaveAttribute('aria-checked', 'false');
      expect(JSON.stringify(file().presentation.services)).toBe(services);
      expect(file().hiddenLimits).toHaveLength(1);
      await expect(page.getByRole('checkbox', { name: 'Claude Pro', exact: true })).toHaveCount(0);
    });
    await test.step('手順8', async () => {
      // Unplug/replug and COM resolution are exercised on hardware. This server checks continued preview/cycling while the explicit device is absent.
      const view = await rpc<View>(page, settingsGet);
      expect(view.displayID).toBe(smallID);
      expect(view.displays!.find(d => d.deviceID === smallID)).toMatchObject({ connected: false });
      expect(view.compatibleThemeIDs).toHaveLength(4);
      const current = frame!.data.service!.key;
      await expect.poll(() => frame?.data.service?.key, { timeout: 10_000 }).not.toBe(current);
      await expect(preview()).toBeVisible();
      expect(hub.streams()).toBe(1);
      // A stale selector request is corrected against the output at the actual save boundary.
      const saved = file();
      const corrected = await rpc<View>(page, 'token-monitor-turzx/internal/settings.Service.Save', [{ source: view.source, url: view.url, displayID: smallID, limitStyle: 'Bars' }]);
      expect(corrected.limitStyle).toBe('gauges-35-landscape');
      expect(corrected.outputWarning).toBe('');
      expect(file()).toEqual({ ...saved, limitStyle: 'gauges-35-landscape', connection: expect.any(String) });
      await expect.poll(() => completed?.request.theme).toBe('gauges-35-landscape');
      expect(hub.streams()).toBe(1);
    });
    await test.step('手順9', async () => {
      // Native window hiding keeps the renderer resident; a server page stands in for reopening the window.
      await page.close();
      expect((await context.request.get(`${server.url}/health`)).ok()).toBe(true);
      expect(hub.streams()).toBe(1);
      page = await context.newPage(); observe(page);
      await page.goto(server.url);
      await expect(preview()).toBeVisible();
    });
    await test.step('手順10', async () => {
      await selectStyle(...styles[3]);
      const savedBeforeRestart = readFileSync(settingsFile);
      await page.close(); await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      frame = undefined; completed = undefined; requests.clear();
      server = await startServer(dataDir, 34137, env);
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats(21));
      page = await context.newPage(); observe(page); await page.goto(server.url);
      await expect.poll(() => frame?.data.service?.number).toBe(1);
      await expect(page.getByRole('textbox', { name: 'Display style' })).toHaveValue(styles[3][0]);
      await expect(page.getByRole('switch', { name: 'Rotate 180°', exact: true })).toHaveAttribute('aria-checked', 'true');
      await expect(page.getByRole('checkbox', { name: 'Claude Pro Alpha 5h', exact: true })).toHaveAttribute('aria-checked', 'false');
      expect(file().serviceCycleSeconds).toBe(50);
      expect(file().cycleSeconds).toBe(70);
      expect(file().cycle ?? false).toBe(false);
      await expect(interval()).toHaveValue('50 s');
      expect(file().limitStyle).toBe(styles[3][1]);
      expect(readFileSync(settingsFile).equals(savedBeforeRestart)).toBe(true);
    });
    await test.step('受け入れ条件', async () => {
      await setPageInterval(5);
      await page.getByRole('switch', { name: skipName, exact: true }).click();
      await expect.poll(() => frame?.data.service?.provider).toBe('Codex');
      await expect.poll(() => frame?.data.service?.count).toBe(2);
      // Codex alone remains visible, but both contracts must still cycle with legacy Cycle OFF.
      const contractPage = frame!.data.service!.key;
      await expect.poll(() => frame?.data.service?.key, { timeout: 4_000 }).not.toBe(contractPage);
      expect(frame!.data.service!.provider).toBe('Codex');
      expect(file().cycle ?? false).toBe(false);
      await openServices();
      await expect(content('Copilot').getByRole('switch')).toHaveAttribute('aria-checked', 'true');
      await expect(content('antigravity-cli').getByRole('switch')).toHaveAttribute('aria-checked', 'true');
      await content('Codex').getByText('Tokens', { exact: true }).click();
      await expect.poll(() => frame?.data.service?.count).toBe(1);
      await expect.poll(() => frame?.data.service?.showLimits).toBe(false);
      expect(file().hiddenLimits).toHaveLength(1);
      const onlyPage = frame!.data.service!.key;
      await page.waitForTimeout(1_200);
      expect(frame!.data.service!.count).toBe(1);
      expect(frame!.data.service!.key).toBe(onlyPage);
      await expect(interval()).toBeVisible();
      await setPageInterval(3600);

      // A real replacement failure preserves the file, generation, page, and checked state.
      const before = readFileSync(settingsFile); const view = await rpc<View>(page, settingsGet); const key = frame!.data.service!.key;
      chmodSync(settingsFile, 0o444);
      try {
        await page.getByRole('switch', { name: skipName, exact: true }).click();
        await expect(page.getByRole('alert').first()).toBeVisible();
        expect(readFileSync(settingsFile).equals(before)).toBe(true);
        expect((await rpc<View>(page, settingsGet)).generation).toBe(view.generation);
        await expect(page.getByRole('switch', { name: skipName, exact: true })).toHaveAttribute('aria-checked', 'true');
        expect(frame!.data.service!.key).toBe(key);
      } finally { chmodSync(settingsFile, 0o600); }

      // Candidate refresh errors disable selection without saving; the next successful poll recovers it.
      const savedBeforeLookup = readFileSync(settingsFile);
      await page.route('**/wails/runtime', async route => {
        if (route.request().postDataJSON()?.args?.methodName === settingsGet) {
          await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ code: 'UNEXPECTED', message: 'candidate lookup failed' }) });
        } else await route.continue();
      });
      try {
        await expect(page.getByRole('textbox', { name: 'Display style' })).toBeDisabled({ timeout: 10_000 });
        await expect(page.getByRole('alert').first()).toBeVisible();
        expect(readFileSync(settingsFile).equals(savedBeforeLookup)).toBe(true);
      } finally { await page.unroute('**/wails/runtime'); }
      await expect(page.getByRole('textbox', { name: 'Display style' })).toBeEnabled({ timeout: 10_000 });

      await page.setViewportSize({ width: 1200, height: 850 });
      const wide = await content('Claude').boundingBox(); const wideNext = await content('Codex').boundingBox();
      expect(Math.abs(wide!.y - wideNext!.y)).toBeLessThan(1);
      await page.setViewportSize({ width: 640, height: 850 });
      const narrow = await content('Claude').boundingBox(); const narrowNext = await content('Codex').boundingBox();
      expect(narrowNext!.y).toBeGreaterThan(narrow!.y);
      expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
      await expect(interval()).toBeVisible();
      for (const [label, id, width, height] of styles) await selectStyle(label, id, width, height);

      // Local and Hub keep the same interval across connection saves.
      await page.getByRole('link', { name: 'Connection', exact: true }).click();
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Local', exact: true }).click();
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect.poll(() => file().source).toBe('Local');
      await expect.poll(() => hub.streams()).toBe(0);
      expect(file().serviceCycleSeconds).toBe(3600);
      await page.getByRole('textbox', { name: 'Data source' }).click();
      await page.getByRole('option', { name: 'Hub', exact: true }).click();
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect.poll(() => file().source).toBe('Hub');
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats(21));
      expect(file().serviceCycleSeconds).toBe(3600);
      await page.getByRole('link', { name: 'Display', exact: true }).click(); await openServices();
      await expect(interval()).toHaveValue('3600 s');

      // Styles compares all six definitions without changing the selected style or preferences.
      const saved = readFileSync(settingsFile);
      await page.getByRole('link', { name: 'Styles', exact: true }).click();
      await expect(page.getByRole('article')).toHaveCount(6);
      expect(readFileSync(settingsFile).equals(saved)).toBe(true);
      await page.getByRole('link', { name: 'Display', exact: true }).click(); await openServices();
      for (const id of ['Claude', 'Codex', 'Copilot', 'antigravity-cli']) {
        await page.getByRole('switch', { name: `Show ${id}`, exact: true }).click();
        await expect(page.getByRole('switch', { name: `Show ${id}`, exact: true })).toHaveAttribute('aria-checked', 'false');
        if (id === 'Codex') await expect.poll(() => frame?.data.serviceMessage).toBe('All services skipped');
      }
      await expect.poll(() => frame?.data.serviceMessage).toBe('No services selected');
      const prepared = await rpc<ThemeData>(page, display + 'ThemePreview', [styles[3][1]]);
      expect(prepared.serviceMessage).toBe('No services selected');
      expect(file().connection).not.toContain('service-e2e-token');
      await expect(page.getByText('service-e2e-token', { exact: true })).toHaveCount(0);
      expect(hub.requests.every(r => r.headers.authorization === 'Bearer service-e2e-token')).toBe(true);
    });
  } finally {
    chmodSync(settingsFile, 0o600);
    await server.stop(); await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
    rmSync(home, { recursive: true, force: true });
  }
});
