import { test, expect, type Page } from '@playwright/test';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { shortIntervals } from '../../support/local';
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
      expect(await preview()).toBe(`data:image/png;base64,${frames[frames.length - 1].png}`);
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
