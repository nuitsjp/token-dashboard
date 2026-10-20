import { test, expect, type Page } from '@playwright/test';
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { localHome, shortIntervals } from '../../support/local';
import type { FrameRequest } from '../../../../bindings/token-monitor-turzx/internal/display/models';
import { signal } from '../../support/presence';
import { measure, type Frame } from '../../support/shift';

// The server build has no TURZX output, so the test watches the frames of the resident renderer: while the
// user is away nothing is drawn, not even for a new usage or the 1-second shift of the short intervals. What
// Windows posts for a lock, a sleep and the like is posted to the hidden window of the server, so it reaches
// the real watcher. The server never counts idle time as away, because the test cannot press keys; the idle
// time, the monitor power and the black image sent to the TURZX are checked by the Go tests of
// internal/presence and internal/display.
const token = 'e2e-hub-token';

// Half an hour past the hour, so the shown time until reset does not change while a test runs.
const hours = (h: number) => new Date(Date.now() + h * 3_600_000 + 1_800_000).toISOString();
function stats(remaining: number) {
  return {
    periods: { today: { totalTokens: 1234567, costUsd: 1.23 }, month: { totalTokens: 23456789, costUsd: 23.45 }, allTime: { totalTokens: 345678901, costUsd: 345.67 } },
    limits: { providers: [{ provider: 'alpha', planLabel: 'Pro', windows: [
      { kind: 'weekly', label: 'Weekly', showMeter: true, remainingPercent: remaining, usedPercent: 100 - remaining, resetsAt: hours(100) },
    ] }] },
  };
}

test('離席条件が成立している間は TURZX を消灯し、すべて解けたら最新の表示に戻す', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-away-e2e-'));
  const hub = await startHub();
  let server = await startServer(dataDir, 34151, shortIntervals);
  let frames: Frame[] = [];
  const record = (target: Page) => target.on('request', request => {
    if (!request.url().endsWith('/wails/runtime') || request.method() !== 'POST') return;
    const body = request.postDataJSON();
    if (body?.args?.methodName !== 'token-monitor-turzx/internal/display.Service.CompleteFrame') return;
    const [, png, jpeg, error] = body.args.args as [number, string, string, string];
    if (!error && png && jpeg) frames.push({ png, jpeg });
  });
  const away = () => page.getByRole('textbox', { name: 'Away time' });
  const file = () => JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
  // The preview is on the Display page, so it is read there and the Settings page is opened again.
  const preview = async () => {
    await page.getByRole('link', { name: 'Display' }).click();
    const src = (await page.getByRole('img', { name: 'Display preview' }).getAttribute('src')) ?? '';
    await page.getByRole('link', { name: 'Settings' }).click();
    return src;
  };
  const setAway = async (value: string) => { await away().fill(value); await away().press('Enter'); };
  // Nothing is drawn for the given time, though a new usage arrives meanwhile.
  const stays = async (ms = 3_000) => {
    await page.waitForTimeout(500);
    const count = frames.length;
    hub.send('stats', stats(20 + (count % 50)));
    await page.waitForTimeout(ms);
    expect(frames.length).toBe(count);
  };
  const drawing = async () => {
    const count = frames.length;
    await expect.poll(() => frames.length, { timeout: 15_000 }).toBeGreaterThan(count + 1);
  };
  record(page);
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
      hub.send('snapshot', stats(40));
      // The Away time is set on the Settings page; the preview is read on the Display page.
      await expect.poll(preview).toContain('data:image/png');
      await page.getByRole('link', { name: 'Settings' }).click();
      await drawing();
    });
    await test.step('手順1', async () => {
      // Away time is in minutes, 10 when nothing is saved, and its entry is right-aligned.
      await expect(away()).toHaveValue('10 min');
      expect(await away().evaluate(e => getComputedStyle(e).textAlign)).toBe('right');
    });
    await test.step('手順2', async () => {
      // A value in range is saved at once with no Save button; one out of range is not saved and the
      // saved value is shown again.
      await expect(page.getByRole('button', { name: 'Save' })).toHaveCount(0);
      await setAway('3');
      await expect.poll(() => file().awayMinutes).toBe(3);
      await expect(away()).toHaveValue('3 min');
      expect(file().source).toBe('Hub');
      expect(file().connection).toBeTruthy();
      await away().fill('241');
      await away().blur();
      await expect(away()).toHaveValue('3 min');
      await setAway('0');
      await expect(away()).toHaveValue('3 min');
      expect(file().awayMinutes).toBe(3);
    });
    await test.step('手順3', async () => {
      // Locked: nothing is drawn any more, though the usage changes, and the preview stays as it was.
      await drawing();
      signal(server.pid, 'lock');
      await page.waitForTimeout(1_500);
      const shown = await preview();
      await stays();
      expect(await preview()).toBe(shown);
    });
    await test.step('手順4', async () => {
      // Back: the latest usage is drawn at once, and the shift starts again from the first position.
      frames = [];
      signal(server.pid, 'unlock');
      await expect.poll(() => frames.length, { timeout: 15_000 }).toBeGreaterThan(2);
      expect((await measure(page, frames.slice(0, 1)))[0].shift).toEqual([0, 0]);
      // A new frame may arrive during preview navigation, so retry the complete comparison.
      await expect.poll(async () => (await preview()) === `data:image/png;base64,${frames[frames.length - 1].png}`).toBe(true);
    });
    await test.step('手順5', async () => {
      // A display plugged in while away is switched off by the black image that the beat and a new usage
      // send again (Go test of internal/display); there is no display here, so the part seen is that
      // nothing else is drawn, whatever arrives.
      signal(server.pid, 'lock');
      await page.waitForTimeout(1_500);
      await stays();
      hub.send('snapshot', stats(70));
      await stays(2_000);
      signal(server.pid, 'unlock');
      await drawing();
    });
    await test.step('受け入れ条件', async () => {
      // Away while any condition holds: the lock ends but the sleep goes on, so nothing is drawn until both end.
      signal(server.pid, 'lock');
      signal(server.pid, 'suspend');
      await page.waitForTimeout(1_500);
      await stays();
      signal(server.pid, 'unlock');
      await stays();
      signal(server.pid, 'resume');
      await drawing();
      // The saved Away time is shown again after a restart, and the watching goes on there.
      await page.close();
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34151, shortIntervals);
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats(40));
      page = await context.newPage();
      record(page);
      await page.goto(server.url);
      await page.getByRole('link', { name: 'Settings' }).click();
      await expect(away()).toHaveValue('3 min');
      await drawing();
      signal(server.pid, 'lock');
      await page.waitForTimeout(1_500);
      await stays();
      signal(server.pid, 'unlock');
      await drawing();
    });
  } finally {
    await page.close().catch(() => undefined);
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});

test('3.5インチStyleは離席中もLocalを取得し、最新値と元のページから巡回を再開する', async ({ page }) => {
  test.setTimeout(150_000);
  const root = resolve(import.meta.dirname, '../../../../..');
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-style-away-e2e-'));
  const isolated = localHome();
  const execDir = join(dataDir, 'bin');
  const sessions = join(dataDir, 'sessions');
  mkdirSync(execDir); mkdirSync(sessions);
  const executable = join(execDir, 'server.exe');
  const settingsFile = join(dataDir, 'settings.json');
  const write = (name: string, value: unknown) => writeFileSync(join(dataDir, name), JSON.stringify(value));
  const saved = () => JSON.parse(readFileSync(settingsFile, 'utf8'));
  const calls = (command: string) => readFileSync(join(dataDir, 'calls.log'), 'utf8').split('\n').filter(line => line.startsWith(`end|${command}|`)).length;
  const date = new Date();
  const today = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
  const graph = (updated = false) => ({ summary: { clients: ['claude', 'codex'] }, contributions: [{ date: today,
    totals: { cost: updated ? 99.99 : 8.03 }, tokenBreakdown: { input: updated ? 99999999 : 80235 },
    clients: [
      { client: 'claude', tokens: { input: updated ? 44444444 : 12345 }, cost: updated ? 44.44 : 1.25 },
      { client: 'codex', tokens: { input: updated ? 55555555 : 67890 }, cost: updated ? 55.55 : 6.78 },
    ],
  }] });
  let server: Awaited<ReturnType<typeof startServer>> | undefined;
  const frames: { request: FrameRequest; png: string }[] = [];
  const requests = new Map<number, FrameRequest>();
  page.on('response', async response => {
    if (!response.url().endsWith('/wails/runtime')) return;
    try {
      const call = response.request().postDataJSON()?.args;
      if (call?.methodName?.endsWith('.RenderRequest') && response.ok()) {
        const request = await response.json() as FrameRequest | null;
        if (request) requests.set(request.id, request);
      }
      if (call?.methodName?.endsWith('.CompleteFrame') && response.ok() && call.args[1] && !call.args[3]) {
        const request = requests.get(call.args[0]);
        if (request) frames.push({ request, png: call.args[1] });
      }
    } catch { /* Ignore unrelated runtime responses. */ }
  });
  const preview = async () => (await page.getByRole('img', { name: 'Display preview', exact: true }).getAttribute('src')) ?? '';
  const away = () => page.getByRole('textbox', { name: 'Away time', exact: true });
  const setAway = async (value: string) => { await away().fill(value); await away().press('Enter'); };
  const lock = async () => { signal(server!.pid, 'lock'); await page.waitForTimeout(1_200); };
  let held = '', heldKey = '', initialKey = '';
  let frozenCount = 0;
  try {
    copyFileSync(join(root, 'bin/token-monitor-turzx-server.exe'), executable);
    execFileSync('go', ['test', '-c', '-o', join(execDir, 'tokscale.exe'), './internal/localusage'], { cwd: root, windowsHide: true });
    writeFileSync(join(dataDir, 'fake-tokscale'), '');
    write('clients.json', { clients: [{ client: 'claude', sessionsPath: sessions }] });
    write('cursor.json', { synced: false, rows: 0, error: 'Not authenticated' });
    write('graph.json', graph());
    write('usage.json', [
      { provider: 'Claude', plan: 'Pro', metrics: [{ label: 'Session', remaining_percent: 31 }] },
      { provider: 'Codex', plan: 'Plus', metrics: [{ label: 'Session', remaining_percent: 62 }] },
    ]);
    write('settings.json', { source: 'Local', displayID: 'USB\\VID_1A86&PID_5722\\USB35INCHIPSV2', displayName: 'TURZX 3.5-inch',
      // shortIntervals scales Page interval by 0.1: 50 here exercises a full 5-second interval.
      limitStyle: 'bars-35-landscape', cycle: true, cycleSeconds: 50, serviceCycleSeconds: 50, hiddenLimits: ['unrelated'],
      presentation: { reverse180: false, skipFull5h: false, services: { Local: {
        Claude: { enabled: true, content: 'Both' }, Codex: { enabled: true, content: 'Both' },
      } } },
    });
    server = await startServer(dataDir, 34153, isolated.env, executable);
    await test.step('分岐条件', async () => {
      await page.goto(server!.url);
      await expect(page.getByText('Pro', { exact: true })).toBeVisible();
      await expect(page.getByText('Plus', { exact: true })).toBeVisible();
      await expect.poll(() => frames.find(frame => frame.request.data.service)?.request.data.service?.key).toBeTruthy();
      initialKey = frames.find(frame => frame.request.data.service)!.request.data.service!.key;
      await expect.poll(() => calls('graph --no-spinner')).toBeGreaterThan(0);
    });
    await test.step('手順1', async () => {
      await page.getByRole('link', { name: 'Settings', exact: true }).click();
      await expect(away()).toHaveValue('10 min');
      await expect(page.getByRole('switch', { name: 'Cycle', exact: true })).toBeVisible();
      await expect(page.getByRole('textbox', { name: 'Cycle time', exact: true })).toHaveValue('50 s');
      await expect(page.getByRole('button', { name: 'Save', exact: true })).toHaveCount(0);
    });
    await test.step('手順2', async () => {
      await setAway('3'); await expect.poll(() => saved().awayMinutes).toBe(3);
      await away().fill('241'); await away().blur(); await expect(away()).toHaveValue('3 min');
      await setAway('0'); await expect(away()).toHaveValue('3 min');
      const before = readFileSync(settingsFile);
      chmodSync(settingsFile, 0o444);
      try {
        await setAway('4'); await expect(page.getByRole('alert')).toBeVisible();
        await expect(away()).toHaveValue('3 min'); expect(readFileSync(settingsFile)).toEqual(before);
      } finally { chmodSync(settingsFile, 0o600); }
      expect(saved()).toMatchObject({ source: 'Local', limitStyle: 'bars-35-landscape', cycle: true, cycleSeconds: 50,
        hiddenLimits: ['unrelated'], presentation: { reverse180: false, skipFull5h: false, services: { Local: {
          Claude: { enabled: true, content: 'Both' }, Codex: { enabled: true, content: 'Both' },
        } } },
      });
      expect(calls('cursor sync --json')).toBe(1);
      await page.getByRole('link', { name: 'Display', exact: true }).click();
    });
    await test.step('手順3', async () => {
      // Lock on the second page so that a reset to the first page cannot accidentally pass.
      await expect.poll(async () => {
        const frame = frames.at(-1);
        return !!frame?.request.data.service && frame.request.data.service.key !== initialKey && await preview() === `data:image/png;base64,${frame.png}`;
      }, { timeout: 7_000 }).toBe(true);
      await lock(); held = await preview(); frozenCount = frames.length;
      heldKey = [...frames].reverse().find(frame => `data:image/png;base64,${frame.png}` === held)!.request.data.service!.key;
      const graphs = calls('graph --no-spinner'), limits = calls('usage --json');
      write('graph.json', graph(true)); writeFileSync(join(sessions, 'session.jsonl'), 'changed');
      await expect.poll(() => calls('graph --no-spinner')).toBeGreaterThan(graphs);
      await expect.poll(() => calls('usage --json')).toBeGreaterThan(limits);
      await page.waitForTimeout(5_500);
      expect(await preview()).toBe(held); expect(frames.length).toBe(frozenCount);
    });
    await test.step('手順4', async () => {
      signal(server!.pid, 'unlock');
      await expect.poll(() => frames.length).toBeGreaterThan(frozenCount);
      const first = frames[frozenCount].request.data.service!;
      expect(first.key).toBe(heldKey);
      expect(first.tokens![0].tokensText).toBe(first.provider === 'Claude' ? '44,444,444' : '55,555,555');
      await expect.poll(preview).not.toBe(held);
      await page.waitForTimeout(3_000);
      expect(frames.at(-1)!.request.data.service!.key).toBe(heldKey);
      await expect.poll(() => frames.at(-1)!.request.data.service!.key, { timeout: 4_000 }).not.toBe(heldKey);
    });
    await test.step('手順5', async () => {
      await lock(); held = await preview(); frozenCount = frames.length;
      // The server has no USB output; actual black bytes and four orientations are checked in Go and on the desktop.
      for (const [label, id] of [
        ['Gauges 3.5" Portrait', 'gauges-35-portrait'], ['Gauges 3.5" Landscape', 'gauges-35-landscape'],
        ['Bars 3.5" Portrait', 'bars-35-portrait'], ['Bars 3.5" Landscape', 'bars-35-landscape'],
      ]) {
        await page.getByRole('textbox', { name: 'Display style' }).click();
        await page.getByRole('option', { name: label, exact: true }).click();
        await expect.poll(() => saved().limitStyle).toBe(id);
        await page.getByRole('switch', { name: 'Rotate 180°', exact: true }).click();
        await expect.poll(() => page.getByRole('switch', { name: 'Rotate 180°', exact: true }).isEnabled()).toBe(true);
        expect(await preview()).toBe(held); expect(frames.length).toBe(frozenCount);
      }
      signal(server!.pid, 'unlock');
      await expect.poll(() => frames.length).toBeGreaterThan(frozenCount);
      await expect.poll(() => frames.at(-1)!.request.theme).toBe('bars-35-landscape');
    });
    await test.step('受け入れ条件', async () => {
      await lock(); held = await preview(); frozenCount = frames.length;
      signal(server!.pid, 'suspend'); signal(server!.pid, 'unlock');
      await page.waitForTimeout(5_500);
      expect(await preview()).toBe(held); expect(frames.length).toBe(frozenCount);
      signal(server!.pid, 'resume');
      await expect.poll(() => frames.length).toBeGreaterThan(frozenCount);
      // Restart while away: only settings persist, not the away condition or cursor.
      await lock();
      const persisted = readFileSync(settingsFile);
      await server!.stop();
      server = await startServer(dataDir, 34153, isolated.env, executable);
      frames.length = 0; requests.clear();
      await page.reload();
      await expect.poll(() => frames.find(frame => frame.request.data.service)?.request.data.service?.key, { timeout: 15_000 }).toBe(initialKey);
      await page.getByRole('link', { name: 'Settings', exact: true }).click();
      await expect(away()).toHaveValue('3 min'); expect(readFileSync(settingsFile)).toEqual(persisted);
      await expect.poll(() => frames.at(-1)?.request.data.service?.key, { timeout: 7_000 }).not.toBe(initialKey);
    });
  } finally {
    if (existsSync(settingsFile)) chmodSync(settingsFile, 0o600);
    await page.close().catch(() => undefined);
    await server?.stop();
    rmSync(dataDir, { recursive: true, force: true }); rmSync(isolated.home, { recursive: true, force: true });
  }
});
