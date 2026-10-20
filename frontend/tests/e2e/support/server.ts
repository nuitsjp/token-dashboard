import { spawn } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { clearMonitorOff } from './presence';

const executable = resolve(import.meta.dirname, '../../../../bin', 'token-monitor-turzx-server' + (process.platform === 'win32' ? '.exe' : ''));

// Large-style scenarios need a saved large output even when Automatic resolves to a connected 3.5-inch device.
// Only the output fixture is set; the source and its defaults remain under test.
export function saveLargeDisplayFixture(dataDir: string) {
  const file = join(dataDir, 'settings.json');
  const saved = existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : {};
  const displayID = 'USB\\VID_1CBE&PID_0092\\E2E92';
  writeFileSync(file, JSON.stringify({ ...saved, displayID, displayName: 'TURZX E2E 9.2-inch' }));
  return displayID;
}

// Starts a dedicated server so that a test can stop and restart the app on the same data folder.
// env replaces the inherited environment of the server, and so of the tokscale it runs.
export async function startServer(dataDir: string, port: number, env: NodeJS.ProcessEnv = process.env, binary = executable) {
  const child = spawn(binary, [], { env: { ...env, WAILS_DATA_DIR: dataDir, WAILS_SERVER_PORT: String(port) }, stdio: 'inherit', windowsHide: true });
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
  try {
    clearMonitorOff(child.pid!);
  } catch (error) {
    await new Promise<void>(done => {
      if (child.exitCode !== null || child.signalCode !== null) return done();
      child.once('exit', () => done());
      if (!child.kill()) done();
    });
    throw error;
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
