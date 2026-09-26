import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
process.chdir(root);
const windows = process.platform === 'win32';
const cli = resolve('.tools', windows ? 'wails3.exe' : 'wails3');
const app = JSON.parse(readFileSync('build/app.json', 'utf8'));
const arch = process.env.GOARCH || (process.arch === 'arm64' ? 'arm64' : 'amd64');
const target = resolve('bin', app.executable);
const server = resolve('bin', 'wails-template-server' + (windows ? '.exe' : ''));
function run(cmd, args, extra = {}) {
  const result = spawnSync(cmd, args, { stdio: 'inherit', ...extra });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status || 1);
}
function safeNSIS(value) {
  if (typeof value !== 'string' || /[\r\n"$]/.test(value)) throw new Error('App metadata contains unsupported NSIS characters');
  return value;
}
function check() {
  if (!/^[a-zA-Z][a-zA-Z0-9_.-]+$/.test(app.id) || !/^[a-zA-Z0-9_.-]+\.exe$/.test(app.executable)) throw new Error('Invalid id/executable in build/app.json');
  if (!/^\d+\.\d+\.\d+$/.test(app.version) || app.version.split('.').some(x => Number(x) > 65535)) throw new Error('version must be major.minor.patch (0..65535)');
  if (!['amd64', 'arm64'].includes(arch)) throw new Error('Unsupported architecture');
}
try {
  check(); mkdirSync('bin', { recursive: true });
  const command = process.argv[2];
  if (command === 'desktop' || command === 'desktop-dev') {
    if (!windows) throw new Error('Windows desktop build must be run on Windows.');
    const production = command === 'desktop';
    // Identity and version come from app.json; the manifest file is a template.
    const manifest = readFileSync('build/windows/app.manifest', 'utf8').replaceAll('__APP_ID__', app.id).replaceAll('__APP_VERSION__', app.version);
    writeFileSync('bin/app.manifest', manifest);
    run(cli, ['generate', 'syso', '-manifest', 'bin/app.manifest', '-icon', 'build/windows/app.ico', '-arch', arch, '-out', `rsrc_windows_${arch}.syso`]);
    run('go', ['build', '-trimpath', ...(production ? ['-tags', 'production'] : []), '-ldflags', '-H windowsgui', '-o', target, '.'], { env: { ...process.env, GOOS: 'windows', GOARCH: arch, CGO_ENABLED: '0' } });
  } else if (command === 'server') {
    run('go', ['build', '-trimpath', '-tags', 'server,production', '-o', server, '.'], { env: { ...process.env, CGO_ENABLED: '0' } });
  } else if (command === 'run' || command === 'run-server') {
    run(command === 'run' ? target : server, []);
  } else if (command === 'package') {
    if (!windows) throw new Error('NSIS packaging must be run on Windows.');
    const include = [
      ['APP_ID', app.id], ['APP_NAME', app.name], ['APP_EXE', app.executable],
      ['APP_VERSION', app.version], ['APP_ARCH', arch],
      ['INSTALLER_NAME', `${app.executable.slice(0, -4)}-${app.version}-${arch}-setup.exe`],
    ].map(([key, value]) => `!define ${key} "${safeNSIS(value)}"`).join('\n') + '\n';
    writeFileSync('build/windows/nsis/app.nsh', '\ufeff' + include);
    const nsisCandidates = [process.env.NSIS_EXE,
      process.env['ProgramFiles(x86)'] && resolve(process.env['ProgramFiles(x86)'], 'NSIS/makensis.exe'),
      process.env.ProgramFiles && resolve(process.env.ProgramFiles, 'NSIS/makensis.exe')];
    const nsis = nsisCandidates.find(p => p && existsSync(p)) || 'makensis';
    run(nsis, ['/V3', 'build/windows/nsis/project.nsi']);
  } else throw new Error(`Unknown internal build action: ${command}`);
} catch (error) { console.error(error instanceof Error ? error.message : error); process.exitCode = 1; }
