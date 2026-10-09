// Local development fixture: fixed tokscale command outputs at the external CLI boundary.
// Reuse the existing localusage test executable's command-output mode; no app mock branch.
import { spawn, spawnSync } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const runDir = mkdtempSync(join(tmpdir(), 'turzx-local-service-ids-'));
const cleanup = () => {
  if (dirname(resolve(runDir)) !== resolve(tmpdir())) throw new Error('Fixture executable directory is outside the temporary parent');
  rmSync(runDir, { recursive: true, force: true });
};
const dataDir = resolve(root, '.test-data/compact-local-ids-mock');
mkdirSync(dataDir, { recursive: true });
mkdirSync(join(dataDir, 'logs'), { recursive: true });
writeFileSync(join(dataDir, 'fake-tokscale'), '');
const write = (name, value) => writeFileSync(join(dataDir, name), JSON.stringify(value));
write('settings.json', {
  source: 'Local', displayID: 'USB\\VID_1A86&PID_5722\\USB35INCHIPSV2',
  displayName: 'TURZX 3.5-inch (USB35INC)', rotationIntervalSeconds: 10,
  compactServiceContent: {
    Codex: { enabled: true, showLimits: false, showTokens: true },
    codex: { enabled: false, showLimits: false, showTokens: true },
    claude: { enabled: true, showLimits: true, showTokens: false },
    Amp: { enabled: false, showLimits: true, showTokens: true },
    Antigravity: { enabled: false, showLimits: true, showTokens: true },
    Copilot: { enabled: false, showLimits: true, showTokens: true },
    'Grok Build': { enabled: false, showLimits: true, showTokens: true },
    Kimi: { enabled: false, showLimits: true, showTokens: true },
    'Warp/Oz': { enabled: false, showLimits: true, showTokens: true },
    'antigravity-cli': { enabled: false, showLimits: true, showTokens: true },
    'OpenCode Go': { enabled: false, showLimits: true, showTokens: true },
    opencode: { enabled: false, showLimits: true, showTokens: true },
    Foo: { enabled: false, showLimits: true, showTokens: true },
    foo: { enabled: false, showLimits: true, showTokens: true },
  },
});
const now = new Date();
const date = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
write('graph.json', {
  summary: { clients: ['codex', 'claude', 'amp', 'antigravity', 'copilot', 'grok', 'kimi', 'warp', 'antigravity-cli', 'opencode', 'foo'] },
  contributions: [{ date, totals: { cost: 99.99 }, tokenBreakdown: { input: 9999999 }, clients: [
    { client: 'codex', tokens: { input: 67890 }, cost: 6.78 },
    { client: 'claude', tokens: { input: 12345 }, cost: 1.25 },
    { client: 'amp', tokens: { input: 111 }, cost: 0.11 },
    { client: 'antigravity', tokens: { input: 222 }, cost: 0.22 },
    { client: 'copilot', tokens: { input: 0 }, cost: 0 },
    { client: 'grok', tokens: { input: 444 }, cost: 0.44 },
    { client: 'kimi', tokens: { input: 555 } },
    { client: 'warp', tokens: { input: 666 }, cost: 0.66 },
    { client: 'antigravity-cli', tokens: { input: 777 }, cost: 0.77 },
    { client: 'opencode', tokens: { input: 888 }, cost: 0.88 },
    { client: 'foo', tokens: { input: 999 }, cost: 0.99 },
  ] }],
});
write('usage.json', [
  { provider: 'Codex', plan: 'Plus', email: 'personal@example.test', metrics: [
    { label: 'Session', remaining_percent: 26 }, { label: 'Weekly', remaining_percent: 62 },
  ] },
  { provider: 'Claude', plan: 'Pro', email: 'claude@example.test', metrics: [
    { label: 'Session', remaining_percent: 31 }, { label: 'Weekly', remaining_percent: 74 },
    { label: 'Extra Session', remaining_percent: 55 },
  ] },
  { provider: 'Codex', plan: 'Team', email: 'team@example.test', metrics: [{ label: 'Weekly', remaining_percent: 90 }] },
  { provider: 'Antigravity', metrics: [{ label: 'Session', remaining_percent: 44 }] },
  { provider: 'Copilot', metrics: [{ label: 'Monthly', remaining_percent: 50 }] },
  { provider: 'Grok Build', metrics: [{ label: 'Session', remaining_percent: 60 }] },
  { provider: 'Kimi', metrics: [{ label: 'Weekly', remaining_percent: 70 }] },
  { provider: 'Warp/Oz', metrics: [{ label: 'Monthly', remaining_percent: 80 }] },
  { provider: 'OpenCode Go', metrics: [{ label: 'Session', remaining_percent: 40 }] },
  { provider: 'Foo', metrics: [{ label: 'Session', remaining_percent: 30 }] },
]);
write('clients.json', { clients: [{ client: 'codex', sessionsPath: join(dataDir, 'logs') }] });
write('cursor.json', { synced: false, rows: 0, error: 'Not authenticated' });

const testBinary = spawnSync('go', ['test', '-c', '-o', join(runDir, 'tokscale.exe'), './internal/localusage'], {
  cwd: root, stdio: 'inherit', windowsHide: true,
});
if (testBinary.error || testBinary.status !== 0) {
  cleanup();
  throw testBinary.error ?? new Error('Could not build the external tokscale fixture executable');
}
copyFileSync(join(root, 'bin/token-monitor-turzx-server.exe'), join(runDir, 'server.exe'));
const home = join(runDir, 'home');
mkdirSync(home, { recursive: true });
const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !/(_HOME|_DIR|_PATH)$|^(XDG_|CODEX|CLAUDE|TOKSCALE)/i.test(name)));
Object.assign(env, {
  HOME: home, USERPROFILE: home, APPDATA: join(home, 'roaming'), LOCALAPPDATA: join(home, 'local'),
  WAILS_DATA_DIR: dataDir, WAILS_SERVER_PORT: '34127', WAILS_TEST_INTERVALS: 'short',
});
const app = spawn(join(runDir, 'server.exe'), [], { env, cwd: root, stdio: 'inherit', windowsHide: true });
app.on('error', error => { console.error(error); cleanup(); process.exitCode = 1; });
app.on('exit', code => { cleanup(); process.exitCode = code || 0; });
process.on('SIGINT', () => app.kill()); process.on('SIGTERM', () => app.kill());
console.log('UI: http://127.0.0.1:34127/; source: Local; fixed data comes only from the tokscale CLI boundary.');
console.log('Codex and codex settings conflict; Claude inherits its lower-case choice. Stop with Ctrl+C.');
