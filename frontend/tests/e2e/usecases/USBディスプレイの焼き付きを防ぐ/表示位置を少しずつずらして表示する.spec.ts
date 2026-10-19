import { test, expect, type Page } from '@playwright/test';
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { saveLargeDisplayFixture, startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { localHome, shortIntervals } from '../../support/local';
import { checkPNGShift, measure, type Frame, type PNGFrame } from '../../support/shift';
import { signal } from '../../support/presence';
import type { FrameRequest } from '../../../../bindings/token-monitor-turzx/internal/display/models';

// The server build has no TURZX output, so the test reads the JPEG that the resident renderer returns
// for the TURZX next to the PNG for the preview. The server shifts every second instead of every
// 3 minutes. The shift is the offset that best moves the PNG onto the JPEG turned back clockwise.
const token = 'e2e-hub-token';
const steps = [[0, 0], [4, 0], [4, 4], [0, 4], [-4, 4], [-4, 0], [-4, -4], [0, -4]];
const background = [15, 17, 23];

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

test('TURZXへ送る画像だけを一定時間ごとに少しずつずらす', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-shift-e2e-'));
  saveLargeDisplayFixture(dataDir);
  const hub = await startHub();
  let server = await startServer(dataDir, 34150, shortIntervals);
  let frames: Frame[] = [];
  const record = (target: Page) => target.on('request', request => {
    if (!request.url().endsWith('/wails/runtime') || request.method() !== 'POST') return;
    const body = request.postDataJSON();
    if (body?.args?.methodName !== 'token-monitor-turzx/internal/display.Service.CompleteFrame') return;
    const [, png, jpeg, error] = body.args.args as [number, string, string, string];
    if (!error && png && jpeg) frames.push({ png, jpeg });
  });
  const indexOf = (shift: number[]) => steps.findIndex(t => t.join() === shift.join());
  // The positions with every repeat removed.
  const walk = (shifts: number[][]) => shifts.filter((s, i) => i === 0 || s.join() !== shifts[i - 1].join());
  // Every position is the same as the one before it or the next of the eight, and the first is `from`.
  const follows = (shifts: number[][], from: number) => walk(shifts).every((s, i, all) =>
    i === 0 ? indexOf(s) === from : indexOf(s) === (indexOf(all[i - 1]) + 1) % steps.length);
  record(page);
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
      hub.send('snapshot', stats(40));
      await page.getByRole('link', { name: 'Display' }).click();
      await expect(page.getByRole('img', { name: 'Display preview' })).toBeVisible();
    });
    await test.step('手順1', async () => {
      // The positions go round the eight points in order, and back to the first.
      frames = [];
      const taken: Frame[] = [];
      const measured: Awaited<ReturnType<typeof measure>> = [];
      // Timed redraws can repeat a position, so a frame count does not guarantee a full cycle.
      await expect.poll(async () => {
        const next = frames.slice(taken.length);
        taken.push(...next);
        measured.push(...await measure(page, next));
        return walk(measured.map(m => m.shift)).length;
      }, { timeout: 30_000, intervals: [1_000] }).toBeGreaterThanOrEqual(steps.length + 1);
      const shifts = measured.map(m => m.shift);
      expect(follows(shifts, indexOf(shifts[0]))).toBe(true);
      expect(new Set(shifts.map(s => s.join())).size).toBe(8);
      // Where the shift leaves a gap it is filled with the background.
      const down = measured.find(m => m.shift.join() === '4,4');
      expect(down?.gap.every((c, i) => Math.abs(c - background[i]) <= 6)).toBe(true);
      // The PNG for the preview does not move: with no new usage it is the same at every position.
      expect(new Set(taken.map(f => f.png)).size).toBe(1);
      const preview = (await page.getByRole('img', { name: 'Display preview' }).getAttribute('src')) ?? '';
      expect(preview).toBe(`data:image/png;base64,${taken[0].png}`);
    });
    await test.step('手順2', async () => {
      // A new usage is drawn at the position the walk is at, and the walk goes on in order.
      frames = [];
      await expect.poll(() => frames.length).toBeGreaterThanOrEqual(2);
      hub.send('stats', stats(10));
      await expect.poll(() => frames.length, { timeout: 30_000 }).toBeGreaterThanOrEqual(12);
      const taken = frames.slice(0, 30);
      expect(new Set(taken.map(f => f.png)).size).toBeGreaterThan(1);
      const shifts = (await measure(page, taken)).map(m => m.shift);
      expect(follows(shifts, indexOf(shifts[0]))).toBe(true);
    });
    await test.step('手順3', async () => {
      // After a restart the walk starts again from the first position; nothing is saved for it.
      await page.close();
      await server.stop();
      await expect.poll(() => hub.streams()).toBe(0);
      server = await startServer(dataDir, 34150, shortIntervals);
      await expect.poll(() => hub.streams()).toBe(1);
      hub.send('snapshot', stats(40));
      frames = [];
      page = await context.newPage();
      record(page);
      await page.goto(server.url);
      await expect.poll(() => frames.length, { timeout: 30_000 }).toBeGreaterThanOrEqual(10);
      const shifts = (await measure(page, frames.slice(0, 14))).map(m => m.shift);
      expect(shifts[0]).toEqual([0, 0]);
      expect(follows(shifts, 0)).toBe(true);
      expect(new Set(shifts.map(s => s.join())).size).toBeGreaterThan(2);
    });
    await test.step('受け入れ条件', async () => {
      // The shift saves nothing: the saved values stay as the Connection screen left them.
      await page.getByRole('link', { name: 'Connection' }).click();
      await expect(page.getByRole('textbox', { name: 'Data source' })).toHaveValue('Hub');
    });
  } finally {
    await page.close().catch(() => undefined);
    await server.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
  }
});

test('3.5インチの全Styleと反転で機器用PNGだけを8点移動し、Local更新・離席・再起動を扱う', async ({ page }) => {
  test.setTimeout(180_000);
  const root = resolve(import.meta.dirname, '../../../../..');
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-small-shift-e2e-'));
  const isolated = localHome();
  const execDir = join(dataDir, 'bin'), sessions = join(dataDir, 'sessions');
  mkdirSync(execDir); mkdirSync(sessions);
  const executable = join(execDir, 'server.exe');
  const settingsFile = join(dataDir, 'settings.json');
  const write = (name: string, value: unknown) => writeFileSync(join(dataDir, name), JSON.stringify(value));
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
  let frames: (PNGFrame & { at: number })[] = [];
  const requests = new Map<number, FrameRequest>();
  const completed = new Map<number, { png: string; device: string; at: number }>();
  const collect = (id: number) => {
    const request = requests.get(id), pixels = completed.get(id);
    if (request && pixels) {
      frames.push({ request, ...pixels }); frames.sort((a, b) => a.request.id - b.request.id);
      completed.delete(id);
    }
  };
  page.on('response', async response => {
    if (!response.url().endsWith('/wails/runtime')) return;
    try {
      const call = response.request().postDataJSON()?.args;
      if (call?.methodName?.endsWith('.RenderRequest') && response.ok()) {
        const request = await response.json() as FrameRequest | null;
        if (request) { requests.set(request.id, request); collect(request.id); }
      }
      if (call?.methodName?.endsWith('.CompleteFrame') && response.ok() && call.args[1] && call.args[2] && !call.args[3]) {
        // Windows notification helpers block Node briefly. The CompleteFrame response can be delivered
        // before the asynchronous RenderRequest body is read; join both instead of losing the first frame.
        completed.set(call.args[0], { png: call.args[1], device: call.args[2], at: Date.now() }); collect(call.args[0]);
      }
    } catch { /* Ignore unrelated runtime responses. */ }
  });
  const offset = (f: PNGFrame) => [f.request.shiftX, f.request.shiftY].join();
  const positions = () => frames.map(offset).filter((p, i, all) => i === 0 || p !== all[i - 1]);
  const ordered = () => {
    const walk = positions(), expected = steps.map(s => s.join());
    expect(walk.every((p, i) => i === 0 || expected.indexOf(p) === (expected.indexOf(walk[i - 1]) + 1) % 8)).toBe(true);
  };
  const preview = async () => await page.getByRole('img', { name: 'Display preview', exact: true }).getAttribute('src');
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
    write('settings.json', { source: 'Local', displayID: 'USB\\VID_1A86&PID_5722&E2E\\USB35INCHIPSV2', displayName: 'TURZX E2E 3.5-inch',
      limitStyle: 'gauges-35-landscape', cycle: false, cycleSeconds: 50, serviceCycleSeconds: 3600,
      presentation: { reverse180: false, skipFull5h: false, services: { Local: {
        Claude: { enabled: true, content: 'Both' }, Codex: { enabled: true, content: 'Both' },
      } } },
    });
    server = await startServer(dataDir, 34154, isolated.env, executable);
    await test.step('開始条件', async () => {
      await page.goto(server!.url);
      await expect(page.getByText('Pro', { exact: true })).toBeVisible();
      await expect(page.getByText('Plus', { exact: true })).toBeVisible();
      await expect.poll(() => frames.at(-1)?.request.data.service?.key).toBeTruthy();
    });
    await test.step('手順1', async () => {
      for (const [label, id, width, height] of [
        ['Gauges 3.5" Landscape', 'gauges-35-landscape', 480, 320],
        ['Gauges 3.5" Portrait', 'gauges-35-portrait', 320, 480],
        ['Bars 3.5" Landscape', 'bars-35-landscape', 480, 320],
        ['Bars 3.5" Portrait', 'bars-35-portrait', 320, 480],
      ] as const) for (const reverse of [false, true]) {
        const style = page.getByRole('textbox', { name: 'Display style' });
        if (await style.inputValue() !== label) {
          await style.click(); await page.getByRole('option', { name: label, exact: true }).click();
        }
        const rotate = page.getByRole('switch', { name: 'Rotate 180°', exact: true });
        await expect(rotate).toBeEnabled();
        if ((await rotate.getAttribute('aria-checked') === 'true') !== reverse) await rotate.click();
        await expect.poll(() => frames.at(-1)?.request.theme === id && frames.at(-1)?.request.reverse180 === reverse).toBe(true);
        frames = [];
        await expect.poll(() => positions().length, { timeout: 15_000, intervals: [200] }).toBeGreaterThanOrEqual(9);
        ordered(); expect(new Set(positions()).size).toBe(8);
        expect(frames.every(f => f.request.width === width && f.request.height === height && f.request.outputFormat === 'png')).toBe(true);
        await checkPNGShift(page, frames);
        expect(new Set(frames.map(f => f.png)).size).toBe(1);
        expect(await preview()).toBe(`data:image/png;base64,${frames[0].png}`);
      }
    });
    await test.step('手順2', async () => {
      frames = [];
      await expect.poll(() => frames.some(f => offset(f) === '4,4'), { timeout: 10_000, intervals: [100] }).toBe(true);
      write('graph.json', graph(true)); writeFileSync(join(sessions, 'session.jsonl'), 'changed');
      await expect.poll(() => frames.at(-1)?.request.data.service?.tokens?.[0]?.tokensText, { timeout: 10_000 }).toBe('44,444,444');
      expect(new Set(frames.map(f => f.png)).size).toBeGreaterThan(1);
      ordered(); await checkPNGShift(page, frames);
    });
    await test.step('手順3', async () => {
      const persisted = readFileSync(settingsFile);
      await server!.stop();
      server = await startServer(dataDir, 34154, isolated.env, executable);
      frames = []; requests.clear(); completed.clear();
      await page.reload();
      await expect.poll(() => frames.length, { timeout: 15_000 }).toBeGreaterThan(1);
      expect(offset(frames[0])).toBe('0,0'); ordered();
      expect(readFileSync(settingsFile)).toEqual(persisted);
      await checkPNGShift(page, frames);
    });
    await test.step('受け入れ条件', async () => {
      const interval = page.getByRole('textbox', { name: 'Page interval', exact: true });
      await interval.fill('50'); await interval.press('Enter');
      frames = [];
      await expect.poll(() => new Set(frames.map(f => f.request.data.service?.key)).size, { timeout: 7_000 }).toBeGreaterThan(1);
      expect(new Set(positions()).size).toBeGreaterThan(3); ordered();
      await checkPNGShift(page, frames);
      signal(server!.pid, 'lock'); await page.waitForTimeout(1_200);
      const held = await preview(), frozen = frames.length;
      await page.waitForTimeout(2_200);
      expect(frames.length).toBe(frozen); expect(await preview()).toBe(held);
      signal(server!.pid, 'unlock');
      await expect.poll(() => frames.length, { intervals: [50] }).toBeGreaterThan(frozen);
      const first = frames[frozen]; expect(offset(first)).toBe('0,0');
      await expect.poll(() => frames.slice(frozen).find(f => offset(f) !== '0,0'), { intervals: [50] }).toBeTruthy();
      const moved = frames.slice(frozen).find(f => offset(f) !== '0,0')!;
      expect(offset(moved)).toBe('4,0'); expect(moved.at - first.at).toBeGreaterThanOrEqual(850);
      await checkPNGShift(page, frames.slice(frozen));
      const saved = JSON.parse(readFileSync(settingsFile, 'utf8'));
      expect(saved.source).toBe('Local'); expect(saved.serviceCycleSeconds).toBe(50);
      expect(Object.keys(saved).some(key => /shift/i.test(key))).toBe(false);
    });
  } finally {
    await page.close().catch(() => undefined);
    await server?.stop();
    rmSync(dataDir, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    rmSync(isolated.home, { recursive: true, force: true });
  }
});
