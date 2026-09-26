import { spawn } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const data = mkdtempSync(join(tmpdir(), 'wails-e2e-'));
const child = spawn(resolve(root, 'bin', 'wails-template-server' + (process.platform === 'win32' ? '.exe' : '')), [], {
  cwd: root, stdio: 'inherit', env: { ...process.env, WAILS_DATA_DIR: data, WAILS_SERVER_PORT: '34115' },
});
child.on('error', error => { console.error(error); rmSync(data, { recursive: true, force: true }); process.exitCode = 1; });
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.on('exit', code => { rmSync(data, { recursive: true, force: true }); process.exitCode = code || 0; });
