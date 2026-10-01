import { mkdirSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// The environment of a server that shortens its waits, so that a test does not wait for real time:
// local reading waits 0.2, 1 and 3 seconds instead of 2, 10 and 45, and the image is redrawn
// every second instead of every minute. The Go tests check the waits themselves.
export const shortIntervals: NodeJS.ProcessEnv = { ...process.env, WAILS_TEST_INTERVALS: 'short' };

// The interval of periodic reads with shortIntervals.
export const shortPoll = 3_000;

// Creates an empty home folder for tokscale, and the environment of a server whose tokscale reads
// only that folder instead of this PC's usage and Cursor account. tokscale also finds logs through
// variables such as CODEX_HOME or XDG_DATA_HOME, so those are left out.
export function localHome() {
  const home = mkdtempSync(join(tmpdir(), 'turzx-local-home-e2e-'));
  mkdirSync(join(home, 'AppData', 'Roaming'), { recursive: true });
  mkdirSync(join(home, 'AppData', 'Local'), { recursive: true });
  const env: NodeJS.ProcessEnv = {};
  for (const [name, value] of Object.entries(shortIntervals)) {
    if (!/(_HOME|_DIR|_PATH)$|^(XDG_|CODEX|CLAUDE|TOKSCALE)/i.test(name)) env[name] = value;
  }
  Object.assign(env, { HOME: home, USERPROFILE: home, APPDATA: join(home, 'AppData', 'Roaming'), LOCALAPPDATA: join(home, 'AppData', 'Local') });
  return { home, env };
}
