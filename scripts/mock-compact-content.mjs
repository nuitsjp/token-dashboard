// Development fixture: only the external SSE boundary supplies fixed production-shaped data.
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const dataDir = resolve(root, '.test-data/compact-content-mock');
mkdirSync(dataDir, { recursive: true });
writeFileSync(join(dataDir, 'settings.json'), JSON.stringify({ source: 'Local', displayID: 'USB\\VID_1A86&PID_5722\\USB35INCHIPSV2', displayName: 'TURZX 3.5-inch (USB35INC)', rotationIntervalSeconds: 5 }));
const home = mkdtempSync(join(tmpdir(), 'turzx-compact-content-home-'));
const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !/(_HOME|_DIR|_PATH)$|^(XDG_|CODEX|CLAUDE|TOKSCALE)/i.test(name)));
Object.assign(env, { HOME: home, USERPROFILE: home, APPDATA: join(home, 'roaming'), LOCALAPPDATA: join(home, 'local'), WAILS_DATA_DIR: dataDir, WAILS_SERVER_PORT: '34127' });
const periods = {
  today: { totalTokens: 1234567, costUsd: 12.34, clients: { claude: 400000, codex: 500000, cursor: 334567 }, clientCosts: { claude: 4.50, codex: 5, cursor: 2.84 } },
  month: { totalTokens: 99000123, costUsd: 456.78, clients: { claude: 30000123, codex: 60000000, cursor: 9000000 }, clientCosts: { claude: 180, codex: 250, cursor: 26.78 } },
  allTime: { totalTokens: 1234567890, costUsd: 9123.45, clients: { claude: 500000000, codex: 700000000, cursor: 34567890 }, clientCosts: { claude: 4000, codex: 5000, cursor: 123.45 } },
};
const claudePeriods = {
  today: { totalTokens: 400000, costUsd: 4.50, clients: { claude: 400000 }, clientCosts: { claude: 4.50 } },
  month: { totalTokens: 30000123, costUsd: 180, clients: { claude: 30000123 }, clientCosts: { claude: 180 } },
  allTime: { totalTokens: 500000000, costUsd: 4000, clients: { claude: 500000000 }, clientCosts: { claude: 4000 } },
};
const largePeriods = {
  today: { totalTokens: 9000000000000000, costUsd: 9999999999.99, clients: { claude: 9000000000000000 }, clientCosts: { claude: 9999999999.99 } },
  month: { totalTokens: 1234567890123456, costUsd: 123456789.12, clients: { claude: 1234567890123456 }, clientCosts: { claude: 123456789.12 } },
  allTime: { totalTokens: 8765432109876543, costUsd: 9876543210.98, clients: { claude: 8765432109876543 }, clientCosts: { claude: 9876543210.98 } },
};
const independent = [
  { kind: 'session', label: 'Model A 5-hour', showMeter: true, remainingPercent: 24, resetsAt: '2027-01-01T00:00:00Z' },
  { kind: 'session', label: 'Model B 5-hour', showMeter: true, remainingPercent: 38, resetsAt: '2027-01-01T00:00:00Z' },
  { kind: 'session', label: 'Model C 5-hour', showMeter: true, remainingPercent: 82, resetsAt: '2027-01-01T00:00:00Z' },
];
const paired = [
  { kind: 'weekly', label: 'Model A weekly', showMeter: true, remainingPercent: 67, resetsAt: '2027-01-01T00:00:00Z' },
  { kind: 'session', label: 'Model A 5-hour', showMeter: true, remainingPercent: 24, resetsAt: '2027-01-01T00:00:00Z' },
  { kind: 'weekly', label: 'Model B weekly', showMeter: true, remainingPercent: 52, resetsAt: '2027-01-01T00:00:00Z' },
  { kind: 'session', label: 'Model B 5-hour', showMeter: true, remainingPercent: 38, resetsAt: '2027-01-01T00:00:00Z' },
  { kind: 'weekly', label: 'Model C weekly', showMeter: true },
  { kind: 'session', label: 'Model C 5-hour', showMeter: true, remainingPercent: 82, resetsAt: '2027-01-01T00:00:00Z' },
];
const providers = [
  { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: [
    { kind: 'session', label: '5-hour', showMeter: true, remainingPercent: 24, resetsAt: '2027-01-01T00:00:00Z' },
    { kind: 'weekly', label: 'weekly', showMeter: true, remainingPercent: 67, resetsAt: '2027-01-01T00:00:00Z' },
    { kind: 'session', label: 'Extra model 5-hour', showMeter: true, remainingPercent: 38, resetsAt: '2027-01-01T00:00:00Z' },
  ] },
  { provider: 'claude', accountLabel: 'personal', planLabel: 'Pro', windows: [
    { kind: 'session', label: '5-hour', showMeter: true, remainingPercent: 95, resetsAt: '2027-01-01T00:00:00Z' },
  ] },
  { provider: 'codex', accountLabel: 'plus', planLabel: 'Plus', windows: [
    { kind: 'session', label: '', showMeter: true, remainingPercent: 82, resetsAt: '2027-01-01T00:00:00Z' },
    { kind: 'weekly', label: 'weekly', showMeter: true },
  ] },
];
const fixtures = {
  services: { periods, limits: { providers: [
    { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: independent },
    { provider: 'codex', accountLabel: 'plus', planLabel: 'Plus', windows: independent },
    { provider: 'cursor', windows: [] },
    { provider: 'antigravity-cli', windows: [] },
    { provider: 'copilot', windows: [] },
    { provider: 'opencode', windows: [] },
    { provider: 'service-with-a-long-reported-id-that-must-wrap-without-clipping-or-horizontal-overflow', accountLabel: 'example', planLabel: 'Example', windows: independent },
  ] } },
  normal: { periods, limits: { providers } },
  three: { periods: claudePeriods, limits: { providers: [
    { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: independent },
  ] } },
  sameGroupThree: { periods: claudePeriods, limits: { providers: [
    { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: [
      { kind: 'weekly', label: 'weekly', showMeter: true, remainingPercent: 67, resetsAt: '2027-01-01T00:00:00Z' },
      { kind: 'daily', label: 'daily', showMeter: true, remainingPercent: 38, resetsAt: '2027-01-01T00:00:00Z' },
      { kind: 'session', label: '5-hour', showMeter: true, remainingPercent: 24, resetsAt: '2027-01-01T00:00:00Z' },
    ] },
  ] } },
  pairedThree: { periods: claudePeriods, limits: { providers: [
    { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: paired },
  ] } },
  largeThree: { periods: largePeriods, limits: { providers: [
    { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: independent },
  ] } },
  overflow: { periods: claudePeriods, limits: { providers: [
    { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: [...independent,
      { kind: 'session', label: 'Model D 5-hour', showMeter: true, remainingPercent: 50 },
    ] },
  ] } },
  unknown: { periods: {
    today: { totalTokens: 0, costUsd: 0, clients: { custom: 0 } },
    month: { totalTokens: 0, costUsd: 0, clientCosts: { custom: 0 } },
    allTime: { totalTokens: 0, costUsd: 0 },
  }, limits: { providers: [{ provider: 'custom', accountLabel: 'example', windows: [{ kind: 'weekly', label: 'weekly', showMeter: true }] }] } },
  noLimits: { periods: claudePeriods, limits: { providers: [{ provider: 'claude', windows: [] }] } },
  large: { periods: largePeriods, limits: { providers: [providers[0]] } },
  empty: { periods: {}, limits: { providers: [] } },
  full: { periods: claudePeriods, limits: { providers: [
    { provider: 'claude', accountLabel: 'team', planLabel: 'Team', windows: [
      { kind: 'session', label: '5-hour', showMeter: true, remainingPercent: 100 },
    ] },
  ] } },
};
let current = null;
const clients = new Set();
function send(response, type) {
  if (current) response.write(`event: ${type}\ndata: ${JSON.stringify({ type, stats: current })}\n\n`);
}
const hub = createServer((request, response) => {
  if (request.url === '/api/stats/stream') {
    if (request.headers.authorization !== 'Bearer compact-content-mock') { response.writeHead(401).end(); return; }
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
    response.flushHeaders(); clients.add(response); send(response, 'snapshot');
    request.on('close', () => clients.delete(response));
    return;
  }
  const fixture = request.url?.replace('/scenario/', '');
  if (Object.hasOwn(fixtures, fixture ?? '')) {
    current = fixtures[fixture];
    for (const client of clients) send(client, 'stats');
    response.writeHead(200, { 'Content-Type': 'text/plain' }).end(fixture);
    return;
  }
  response.writeHead(404).end();
});
hub.listen(34128, '127.0.0.1');
const heartbeat = setInterval(() => { for (const client of clients) client.write(': hb\n\n'); }, 30_000);
const app = spawn(resolve(root, 'bin/token-monitor-turzx-server.exe'), [], { env, cwd: root, stdio: 'inherit', windowsHide: true });
app.on('exit', () => {
  stop();
  if (dirname(resolve(home)) !== resolve(tmpdir())) throw new Error('Temporary home is outside the expected parent');
  rmSync(home, { recursive: true, force: true });
});
let stopping = false;
function stop() {
  if (stopping) return;
  stopping = true;
  clearInterval(heartbeat);
  app.kill(); for (const client of clients) client.end(); hub.close();
}
process.on('SIGINT', stop); process.on('SIGTERM', stop);
console.log('UI: http://127.0.0.1:34127/; Hub: http://127.0.0.1:34128/; token: compact-content-mock');
console.log(`Fixtures: /scenario/${Object.keys(fixtures).join(', /')}; initial state: waiting. Stop with Ctrl+C.`);
