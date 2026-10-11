import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';
import { measure, type Frame } from '../../support/shift';

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
      await expect.poll(() => frames.length, { timeout: 30_000 }).toBeGreaterThanOrEqual(14);
      const taken = frames.slice(0, 40);
      const measured = await measure(page, taken);
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
