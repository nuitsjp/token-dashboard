import { spawn } from 'node:child_process';
import { resolve } from 'node:path';
import { writeFileSync } from 'node:fs';

// Existing layout scenarios target the wide device explicitly now that Automatic may select 3.5-inch.
export function selectWideDisplay(dataDir: string) {
  writeFileSync(resolve(dataDir, 'settings.json'), JSON.stringify({ source: 'Local',
    displayID: 'USB\\VID_1CBE&PID_0092\\633A6E01A48A0706', displayName: 'TURZX 9.2-inch (633A6E01)' }));
}

const executable = resolve(import.meta.dirname, '../../../../bin', 'token-monitor-turzx-server' + (process.platform === 'win32' ? '.exe' : ''));

// Starts a dedicated server so that a test can stop and restart the app on the same data folder.
// env replaces the inherited environment of the server, and so of the tokscale it runs.
export async function startServer(dataDir: string, port: number, env: NodeJS.ProcessEnv = process.env, serverExecutable = executable) {
  const child = spawn(serverExecutable, [], { env: { ...env, WAILS_DATA_DIR: dataDir, WAILS_SERVER_PORT: String(port) }, stdio: 'inherit' });
  const url = `http://127.0.0.1:${port}`;
  const deadline = Date.now() + 30_000;
  for (;;) {
    if (child.exitCode !== null) throw new Error(`server exited with ${child.exitCode}`);
    try {
      if ((await fetch(`${url}/health`)).ok) break;
    } catch { /* not listening yet */ }
    if (Date.now() > deadline) { child.kill(); throw new Error('server did not start'); }
    await new Promise(done => setTimeout(done, 200));
  }
  return {
    url,
    pid: child.pid!,
    stop: () => new Promise<void>(done => {
      if (child.exitCode !== null || child.signalCode !== null) return done();
      child.once('exit', () => done());
      child.kill();
    }),
  };
}
