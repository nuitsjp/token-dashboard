import { spawn } from 'node:child_process';
import { resolve } from 'node:path';

const executable = resolve(import.meta.dirname, '../../../../bin', 'token-monitor-turzx-server' + (process.platform === 'win32' ? '.exe' : ''));

// Starts a dedicated server so that a test can stop and restart the app on the same data folder.
export async function startServer(dataDir: string, port: number) {
  const child = spawn(executable, [], { env: { ...process.env, WAILS_DATA_DIR: dataDir, WAILS_SERVER_PORT: String(port) }, stdio: 'inherit' });
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
    stop: () => new Promise<void>(done => {
      if (child.exitCode !== null) return done();
      child.once('exit', () => done());
      child.kill();
    }),
  };
}
