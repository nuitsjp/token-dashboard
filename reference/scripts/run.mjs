// The only public command entry. Build order lives in Taskfile.yml.
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, delimiter, resolve } from 'node:path';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const requiredNodeVersion = readFileSync(resolve(root, '.nvmrc'), 'utf8').trim();
if (process.versions.node !== requiredNodeVersion) {
  console.error(`Node.js ${requiredNodeVersion} を使用してください（実行中: ${process.versions.node}）。nvm install ${requiredNodeVersion} と nvm use ${requiredNodeVersion} を実行してください。`);
  process.exit(1);
}
process.chdir(root);
const windows = process.platform === 'win32';
const tools = resolve('.tools');
const cli = resolve(tools, windows ? 'wails3.exe' : 'wails3');
const env = { ...process.env, PATH: tools + delimiter + process.env.PATH };
function run(command, args, cwd = root, extra = {}) {
  // npm.cmd needs cmd.exe on Windows. All args here are fixed by this script.
  const options = { cwd, env: { ...env, ...extra }, stdio: 'inherit' };
  const result = windows && command === 'npm'
    ? spawnSync(process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', `npm ${args.join(' ')}`], options)
    : spawnSync(command, args, options);
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status || 1);
}
const [command = 'help', ...args] = process.argv.slice(2);
try {
  if (command === 'setup') {
    const match = readFileSync('go.mod', 'utf8').match(/github\.com\/wailsapp\/wails\/v3 (\S+)/);
    if (!match) throw new Error('Wails version is missing from go.mod');
    mkdirSync(tools, { recursive: true });
    // No silently substituted local CLI or hand-authored generated files.
    run('go', ['install', `github.com/wailsapp/wails/v3/cmd/wails3@${match[1]}`], root, { GOBIN: tools });
    run('go', ['mod', 'tidy']);
    run('npm', [existsSync('frontend/package-lock.json') ? 'ci' : 'install', '--no-audit', '--no-fund'], resolve('frontend'));
    run(cli, ['task', 'generate']);
  } else if (command === 'help') {
    console.log('node scripts/run.mjs setup | dev | dev:mock | build | package | server | verify | test:core | release <args>');
  } else {
    if (!existsSync(cli)) throw new Error('先に node scripts/run.mjs setup を実行してください。');
    if (command === 'dev' || command === 'dev:mock') {
      if (!windows) throw new Error('Desktop development is Windows-only. Use server for browser verification.');
      run(cli, ['dev'], root, { WAILS_FRONTEND_MODE: command === 'dev:mock' ? 'mock' : 'real' });
    } else if (command === 'release') {
      run('go', ['run', './cmd/release', ...args]);
    } else if (['build', 'package', 'server', 'verify', 'test:core', 'generate'].includes(command)) {
      run(cli, ['task', command]);
    } else throw new Error(`Unknown command: ${command}`);
  }
} catch (error) {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
}
