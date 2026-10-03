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
function git(...args) {
  const result = spawnSync('git', args, { cwd: root, encoding: 'utf8' });
  if (result.status !== 0) throw new Error(`git ${args.join(' ')}: ${result.stderr.trim()}`);
  return result.stdout.trim();
}
function parseVersion(text) {
  const parts = /^(\d+)\.(\d+)\.(\d+)$/.exec(text ?? '')?.slice(1).map(Number);
  return parts && parts.every(n => n <= 65535) ? parts : null;
}
const compareVersions = (a, b) => a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
// Pushes a vX.Y.Z tag; the release workflow builds, signs and publishes it.
function tagRelease(requested) {
  git('fetch', '--tags', 'origin');
  if (git('status', '--porcelain')) throw new Error('コミットしていない変更があります。');
  if (spawnSync('git', ['merge-base', '--is-ancestor', 'HEAD', 'origin/main'], { cwd: root }).status !== 0) {
    throw new Error('HEAD が origin/main に含まれていません。main にマージしてから実行してください。');
  }
  // Without a tag, the version in build/app.json counts as the latest.
  const latest = git('tag', '--list', 'v*').split('\n').map(tag => parseVersion(tag.slice(1))).filter(Boolean)
    .sort(compareVersions).at(-1) ?? parseVersion(JSON.parse(readFileSync(resolve(root, 'build/app.json'), 'utf8')).version);
  const next = requested === undefined ? [latest[0], latest[1], latest[2] + 1] : parseVersion(requested);
  if (!next) throw new Error('版は major.minor.patch（各 0〜65535）で指定してください。');
  if (compareVersions(next, latest) <= 0) throw new Error(`v${latest.join('.')} より新しい版を指定してください。`);
  const tag = `v${next.join('.')}`;
  git('tag', tag);
  git('push', 'origin', tag);
  console.log(`${tag} を push しました。リリースは GitHub Actions の Release で作成されます。`);
}
const [command = 'help', ...args] = process.argv.slice(2);
try {
  if (command === 'setup') {
    const match = readFileSync('go.mod', 'utf8').match(/github\.com\/wailsapp\/wails\/v3 (\S+)/);
    if (!match) throw new Error('Wails version is missing from go.mod');
    mkdirSync(tools, { recursive: true });
    // No silently substituted local CLI or hand-authored generated files.
    // A CLI of the same version, such as one that the CI restored from its cache, is kept.
    const installed = existsSync(cli) ? spawnSync(cli, ['version'], { encoding: 'utf8' }).stdout?.trim() : '';
    if (installed !== match[1]) run('go', ['install', `github.com/wailsapp/wails/v3/cmd/wails3@${match[1]}`], root, { GOBIN: tools });
    run('go', ['mod', 'tidy']);
    run('npm', [existsSync('frontend/package-lock.json') ? 'ci' : 'install', '--no-audit', '--no-fund'], resolve('frontend'));
    run(cli, ['task', 'generate']);
  } else if (command === 'tag') {
    tagRelease(args[0]);
  } else if (command === 'help') {
    console.log('node scripts/run.mjs setup | dev | build | package | server | verify | test:core | test:desktop | tag [version] | release <args>');
  } else {
    if (!existsSync(cli)) throw new Error('先に node scripts/run.mjs setup を実行してください。');
    if (command === 'dev') {
      if (!windows) throw new Error('Desktop development is Windows-only. Use server for browser verification.');
      // Not the Wails default 9245, which other Wails projects on this PC also use.
      run(cli, ['dev', '-port', '9345']);
    } else if (command === 'test:desktop') {
      // Installs, updates and uninstalls the desktop app; never part of verify.
      run(cli, ['task', 'build:server']);
      run('npm', ['--prefix', 'frontend', 'run', 'test:e2e', '--', '--grep', '@desktop'], root, { DESKTOP_E2E: '1' });
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
