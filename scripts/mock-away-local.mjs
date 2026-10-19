// Supplies fixed command responses only at the external tokscale boundary.
import { spawn, spawnSync } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
const root = resolve(import.meta.dirname, '..');
const runDir = mkdtempSync(join(tmpdir(), 'turzx-away-local-'));
const dataDir = join(runDir, 'data');
mkdirSync(join(dataDir, 'logs'), { recursive: true });
const write = (name, value) => writeFileSync(join(dataDir, name), JSON.stringify(value));
writeFileSync(join(dataDir, 'fake-tokscale'), '');
// The existing short-interval switch scales Cycle time by 0.1, so 50 means 5 seconds here.
write('settings.json', { source: 'Local', displayID: 'USB\\VID_1A86&PID_5722\\USB35INCHIPSV2', displayName: 'TURZX 3.5-inch', limitStyle: 'gauges-35-landscape', cycle: true, cycleSeconds: 50 });
const now = new Date();
const date = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
write('graph.json', { summary: { clients: ['claude', 'codex'] }, contributions: [{ date, totals: { cost: 3 }, tokenBreakdown: { input: 3000 }, clients: [
  { client: 'claude', tokens: { input: 1000 }, cost: 1 }, { client: 'codex', tokens: { input: 2000 }, cost: 2 },
] }] });
write('usage.json', [
  { provider: 'Claude', plan: 'Pro', metrics: [{ label: 'Session', remaining_percent: 40 }] },
  { provider: 'Codex', plan: 'Plus', metrics: [{ label: 'Session', remaining_percent: 70 }] },
]);
write('clients.json', { clients: [{ client: 'claude', sessionsPath: join(dataDir, 'logs') }] });
write('cursor.json', { synced: false, rows: 0, error: 'Not authenticated' });
const result = spawnSync('go', ['test', '-c', '-o', join(runDir, 'tokscale.exe'), './internal/localusage'], { cwd: root, stdio: 'inherit', windowsHide: true });
if (result.error || result.status !== 0) { rmSync(runDir, { recursive: true, force: true }); throw result.error ?? new Error('Cannot build the external CLI fixture'); }
copyFileSync(join(root, 'bin/token-monitor-turzx-server.exe'), join(runDir, 'server.exe'));
const home = join(runDir, 'home');
mkdirSync(home);
const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !/(_HOME|_DIR|_PATH)$|^(XDG_|CODEX|CLAUDE|TOKSCALE)/i.test(name)));
Object.assign(env, { HOME: home, USERPROFILE: home, APPDATA: join(home, 'roaming'), LOCALAPPDATA: join(home, 'local'), WAILS_DATA_DIR: dataDir, WAILS_SERVER_PORT: '34127', WAILS_TEST_INTERVALS: 'short' });
const app = spawn(join(runDir, 'server.exe'), [], { cwd: root, env, stdio: 'inherit', windowsHide: true });
const cleanup = () => rmSync(runDir, { recursive: true, force: true });
app.on('error', error => { console.error(error); cleanup(); process.exitCode = 1; });
app.on('exit', code => { cleanup(); process.exitCode = code || 0; });
process.on('SIGINT', () => app.kill());
process.on('SIGTERM', () => app.kill());
console.log(`UI: http://127.0.0.1:34127/; PID: ${app.pid}; data: ${dataDir}`);
console.log('Local CLI fixture only; no USB output. Stop with Ctrl+C.');
