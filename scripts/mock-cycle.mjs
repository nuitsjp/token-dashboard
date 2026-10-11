// Mock for the "表示しきれない利用枠を順番に切り替えて表示する" scenario (stage 2 only; removed with the mock).
// Starts a fake Hub with 12 contracts of two windows each and the server build on http://127.0.0.1:34130.
// Gauges: 7 contracts per page, Bars: 8 per page, so both styles have two pages.
// Connection: Data source Hub, URL http://127.0.0.1:34131, any token.
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const hours = h => new Date(Date.now() + h * 3_600_000).toISOString();
const meter = (kind, label, remaining, h) => ({ kind, label, showMeter: true, remainingPercent: remaining, usedPercent: 100 - remaining, resetsAt: hours(h) });
const names = ['claude', 'codex', 'cursor', 'gemini', 'copilot', 'kiro', 'amp', 'droid', 'kimi', 'qwen', 'zed', 'opencode'];
const stats = () => ({
  periods: { today: { totalTokens: 1234567, costUsd: 1.23 }, month: { totalTokens: 23456789, costUsd: 23.45 }, allTime: { totalTokens: 345678901, costUsd: 345.67 } },
  limits: { providers: names.map((n, i) => ({ provider: n, accountLabel: `${n}@example.com`, planLabel: `Plan ${i + 1}`,
    windows: [meter('session', '5-hour', 10 + i * 6, 3), meter('weekly', 'Weekly', 20 + i * 6, 100)] })) },
});

const streams = new Set();
const hub = createServer((req, res) => {
  if (req.url !== '/api/stats/stream') { res.writeHead(404).end(); return; }
  res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
  res.write(': connected\n\n');
  const data = JSON.stringify({ type: 'snapshot', reason: 'snapshot', stats: stats(), at: new Date().toISOString() });
  res.write(`event: snapshot\ndata: ${data}\n\n`);
  streams.add(res);
  req.on('close', () => streams.delete(res));
}).listen(34131, '127.0.0.1');

const exe = join('bin', process.platform === 'win32' ? 'token-monitor-turzx-server.exe' : 'token-monitor-turzx-server');
const app = spawn(exe, [], { env: { ...process.env, WAILS_DATA_DIR: mkdtempSync(join(tmpdir(), 'turzx-cycle-mock-')), WAILS_SERVER_PORT: '34130' }, stdio: 'inherit' });
const stop = () => { app.kill(); hub.close(); process.exit(0); };
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
app.on('exit', () => process.exit(0));
