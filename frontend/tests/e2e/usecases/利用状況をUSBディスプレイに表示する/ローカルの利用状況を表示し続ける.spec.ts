import { test, expect, type Page } from '@playwright/test';
import { appendFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startServer } from '../../support/server';
import { startHub } from '../../support/hub';
import { watchTokscale } from '../../support/processes';

// The server build has no task tray and no TURZX output. Opening the page stands in for opening
// the window, and stopping the process for Exit. tokscale reads a Claude Code log in a separate
// home folder instead of this PC's usage, and has no Cursor account there. The tokens are checked
// by where each number ends on the 1920x462 image: a longer number ends further right.
const background = [15, 17, 23];
const line = [42, 47, 58]; // the divider between Tokens and Usage Limits
const tokenRows = { today: [0, 75, 350, 110], month: [0, 215, 350, 250], all: [0, 355, 350, 390] } as const;

async function preview(page: Page) {
  const image = page.getByRole('img', { name: 'Display preview' });
  await expect(image).toBeVisible();
  return (await image.getAttribute('src')) ?? '';
}

// Returns the divider colour and the rightmost x of the drawn pixels in each token row.
async function inspect(page: Page) {
  return page.evaluate(async ({ src, rows }) => {
    const img = new Image();
    img.src = src;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext('2d')!;
    context.drawImage(img, 0, 0);
    const ends: Record<string, number> = {};
    for (const [name, [left, top, right, bottom]] of Object.entries(rows)) {
      const data = context.getImageData(left, top, right - left, bottom - top).data;
      let max = -1;
      for (let i = 0; i < data.length; i += 4) {
        if (data[i] === 15 && data[i + 1] === 17 && data[i + 2] === 23) continue;
        max = Math.max(max, left + ((i / 4) % (right - left)));
      }
      ends[name] = max;
    }
    return { divider: [...context.getImageData(356, 231, 1, 1).data.slice(0, 3)], ...ends } as { divider: number[]; today: number; month: number; all: number };
  }, { src: await preview(page), rows: tokenRows });
}

// One assistant message of a Claude Code log with the given input tokens.
let messages = 0;
function message(at: Date, input: number, output = 0, cacheRead = 0, cacheWrite = 0) {
  messages++;
  return JSON.stringify({
    type: 'assistant', timestamp: at.toISOString(), requestId: `request-${messages}`, sessionId: 'session',
    message: { id: `message-${messages}`, model: 'claude-sonnet-4-5', usage: { input_tokens: input, output_tokens: output, cache_read_input_tokens: cacheRead, cache_creation_input_tokens: cacheWrite } },
  }) + '\n';
}

test('ローカルの利用記録の変化に合わせて表示を更新し、取得元を変えると取得を止める', async ({ page }) => {
  test.setTimeout(300_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-local-usage-e2e-'));
  const home = mkdtempSync(join(tmpdir(), 'turzx-local-home-e2e-'));
  const sessions = join(home, '.claude', 'projects', 'e2e');
  const log = join(sessions, 'session.jsonl');
  mkdirSync(sessions, { recursive: true });
  mkdirSync(join(home, 'AppData', 'Roaming'), { recursive: true });
  mkdirSync(join(home, 'AppData', 'Local'), { recursive: true });
  const now = new Date();
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 12);
  const sameMonth = yesterday.getMonth() === now.getMonth();
  // Today 1,000, yesterday 100,000 and 60 days ago 10,000,000 tokens.
  appendFileSync(log, message(new Date(now.getFullYear(), now.getMonth(), now.getDate() - 60, 12), 10_000_000)
    + message(yesterday, 100_000) + message(now, 100, 200, 300, 400));
  // Only the variables Windows needs to run programs pass through, because tokscale finds logs
  // through many others, such as CODEX_HOME or XDG_DATA_HOME.
  const env: NodeJS.ProcessEnv = { HOME: home, USERPROFILE: home, APPDATA: join(home, 'AppData', 'Roaming'), LOCALAPPDATA: join(home, 'AppData', 'Local') };
  for (const [name, value] of Object.entries(process.env)) {
    if (/^(SystemRoot|windir|SystemDrive|ComSpec|Path|PATHEXT|TEMP|TMP|OS|NUMBER_OF_PROCESSORS|PROCESSOR_ARCHITECTURE)$/i.test(name)) env[name] = value;
  }
  const hub = await startHub();
  let server: Awaited<ReturnType<typeof startServer>> | undefined;
  let tokscale: ReturnType<typeof watchTokscale> | undefined;
  let shown = { today: 0, month: 0, all: 0 };
  let idleFrom = 0;
  try {
    await test.step('分岐条件', async () => {
      // No settings are saved, so the source is Local.
      expect(existsSync(join(dataDir, 'settings.json'))).toBe(false);
    });

    await test.step('手順1', async () => {
      server = await startServer(dataDir, 34121, env);
      tokscale = watchTokscale(server.pid);
      await page.goto(server.url);
      await expect(page.getByRole('textbox', { name: 'Data source' })).toHaveValue('Local');
      // Until both the tokens and the limits arrive, the image has no values and no divider.
      expect((await inspect(page)).divider).toEqual(background);
    });

    await test.step('手順2', async () => {
      // Reading the limits takes about 36 seconds on the GitHub runner, which has no AI tool.
      await expect.poll(async () => (await inspect(page)).divider, { timeout: 90_000 }).toEqual(line);
      const first = await inspect(page);
      // 1,000 < 101,000 < 10,101,000. On the first of a month, yesterday is in the last month.
      if (sameMonth) expect(first.today).toBeLessThan(first.month);
      else expect(first.month).toBe(first.today);
      expect(first.month).toBeLessThan(first.all);
      shown = first;
    });

    await test.step('手順3', async () => {
      // Today becomes 1,001,000 tokens.
      appendFileSync(log, message(new Date(), 1_000_000));
      await expect.poll(async () => (await inspect(page)).today, { timeout: 60_000 }).toBeGreaterThan(shown.today);
      const changed = await inspect(page);
      expect(changed.month).toBeGreaterThan(shown.month);
      shown = changed;
      idleFrom = Date.now();
    });

    await test.step('手順4', async () => {
      // Without log changes, only the limits are read again.
      await page.waitForTimeout(50_000);
      expect(tokscale!.runs('usage --json').filter(run => run.start > idleFrom).length).toBeGreaterThanOrEqual(1);
      expect(tokscale!.runs('graph --no-spinner').filter(run => run.start > idleFrom)).toEqual([]);
      const idle = await inspect(page);
      expect([idle.today, idle.month, idle.all]).toEqual([shown.today, shown.month, shown.all]);
    });

    await test.step('手順5', async () => {
      // The date can not be moved here. Today and Month split the log by this PC's local date,
      // which is what the count after midnight uses: yesterday's 100,000 tokens are not Today.
      const current = await inspect(page);
      if (sameMonth) expect(current.today).toBeLessThanOrEqual(current.month);
      expect(current.today).toBeLessThan(current.all);
    });

    await test.step('手順6', async () => {
      const source = page.getByRole('textbox', { name: 'Data source' });
      await source.click();
      await page.getByRole('option', { name: 'Hub' }).click();
      await page.getByLabel('Hub URL').fill(hub.url);
      await page.getByLabel(/Access token/).fill('e2e-hub-token');
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Saved.')).toBeVisible();
      await expect.poll(() => tokscale!.running(), { timeout: 5_000 }).toBe(false);
      await expect.poll(() => hub.streams(), { timeout: 10_000 }).toBeGreaterThan(0);
      const stoppedAt = Date.now();
      await page.waitForTimeout(15_000);
      expect([...tokscale!.runs('graph --no-spinner'), ...tokscale!.runs('usage --json')].filter(run => run.end >= stoppedAt)).toEqual([]);
      await server!.stop();
      expect(await tokscale!.running()).toBe(false);
    });

    await test.step('受け入れ条件', async () => {
      // Sampling can miss a short run, so the overlap and the 10 seconds between graphs are
      // checked by the Go tests of internal/localusage, which record every run.
      // Without a Cursor account, the start's sync turns syncing off once, without retries.
      const logs = readFileSync(join(dataDir, 'logs', 'app.jsonl'), 'utf8');
      expect(logs.match(/"cursor_sync_disabled"/g) ?? []).toHaveLength(1);
      expect(logs).not.toContain('cursor_sync_failed');
      // tokscale keeps its caches in the app's folder, not in the home folder.
      expect(readdirSync(join(dataDir, 'tokscale')).length).toBeGreaterThan(0);
      expect(existsSync(join(home, '.config', 'tokscale', 'cache'))).toBe(false);
    });
  } finally {
    tokscale?.stop();
    await server?.stop();
    await hub.close();
    rmSync(dataDir, { recursive: true, force: true });
    rmSync(home, { recursive: true, force: true });
  }
});
