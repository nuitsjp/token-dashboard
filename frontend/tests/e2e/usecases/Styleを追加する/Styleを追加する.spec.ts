import { test, expect, chromium, type Page } from '@playwright/test';
import { spawn, type ChildProcess } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { appProcesses, powershell, waitFor } from '../../support/desktop';
import { startHub } from '../../support/hub';

// Opens the desktop folder dialog, so it runs only when STYLE_E2E=1. The development
// build in bin/ is required: the production build has no remote debugging port.
const root = resolve(import.meta.dirname, '../../../../..');
const exe = join(root, 'bin', process.platform === 'win32' ? 'token-monitor-turzx.exe' : 'token-monitor-turzx');
const debugPort = '9471';
const appID = 'io.github.nuitsjp.token-monitor-turzx';

function quote(value: string) {
  return `'${value.replaceAll("'", "''")}'`;
}

function saveSettings(dataDir: string, url: string) {
  const plain = JSON.stringify({ url, token: 'e2e-hub-token' });
  const sealed = powershell(`Add-Type -AssemblyName System.Security
[Convert]::ToBase64String([Security.Cryptography.ProtectedData]::Protect([Text.Encoding]::UTF8.GetBytes(${quote(plain)}), [Text.Encoding]::UTF8.GetBytes('${appID}'), 'CurrentUser'))`);
  mkdirSync(dataDir, { recursive: true });
  writeFileSync(join(dataDir, 'settings.json'), JSON.stringify({ source: 'Hub', connection: sealed, displayID: '', displayName: '', limitStyle: 'Gauges' }));
}

function writeStyle(dir: string, id: string, name: string, css: string) {
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, 'theme.json'), JSON.stringify({
    formatVersion: 1, id, name, width: 1920, height: 462, template: 'template.hbs', stylesheet: 'style.css',
  }));
  copyFileSync(join(root, 'themes', 'bars', 'template.hbs'), join(dir, 'template.hbs'));
  writeFileSync(join(dir, 'style.css'), css);
}

function startApp(dataDir: string) {
  spawn(exe, ['--show'], {
    env: { ...process.env, WAILS_DATA_DIR: dataDir, WAILS_WEBVIEW_DEBUG_PORT: debugPort },
    detached: true, stdio: 'ignore',
  }).unref();
}

function powershellAsync(script: string): ChildProcess {
  return spawn('powershell.exe', ['-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], { stdio: ['ignore', 'pipe', 'pipe'] });
}

function finished(child: ChildProcess) {
  return new Promise<void>((resolve, reject) => {
    let err = '';
    child.stderr?.on('data', chunk => { err += chunk; });
    child.on('exit', code => code === 0 ? resolve() : reject(new Error(err || `exit ${code}`)));
  });
}

// The dialog reports the selected item's display name through WM_GETTEXT. The typed path
// is the window text, so the app reads that when the edit changes. This runs beside the
// button click: the dialog is modal, so the click does not finish until the dialog closes.
function chooseFolder(action: 'ok' | 'cancel', folder = '') {
  return powershellAsync(`
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public class StyleFolderDialog {
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowW(string cls, string title);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern bool SetWindowTextW(IntPtr h, string text);
  [DllImport("user32.dll")] public static extern IntPtr SendMessageW(IntPtr h, uint msg, IntPtr w, IntPtr l);
}
'@
$deadline = (Get-Date).AddSeconds(45)
$dlg = [IntPtr]::Zero
do { Start-Sleep -Milliseconds 200; $dlg = [StyleFolderDialog]::FindWindowW('#32770', 'Select a folder') } while ($dlg -eq [IntPtr]::Zero -and (Get-Date) -lt $deadline)
if ($dlg -eq [IntPtr]::Zero) { throw 'folder dialog did not open' }
if (${quote(action)} -eq 'cancel') { [void][StyleFolderDialog]::SendMessageW($dlg, 0x0111, [IntPtr]2, [IntPtr]::Zero); return }
$edit = [StyleFolderDialog]::GetDlgItem($dlg, 14148)
[void][StyleFolderDialog]::SetWindowTextW($edit, ${quote(folder)})
[void][StyleFolderDialog]::SendMessageW($dlg, 0x0111, [IntPtr]((0x0300 -shl 16) -bor 14148), $edit)
Start-Sleep -Milliseconds 200
[void][StyleFolderDialog]::SendMessageW($dlg, 0x0111, [IntPtr]1, [IntPtr]::Zero)
`);
}

async function addStyle(page: Page, action: 'ok' | 'cancel', folder = '') {
  const dialog = finished(chooseFolder(action, folder));
  const clicked = page.getByRole('button', { name: 'Add style' }).click({ timeout: 90_000 });
  await Promise.all([clicked, dialog]);
}

async function openStyles(page: Page) {
  await page.goto('http://wails.localhost/#/styles');
  await expect(page.getByRole('heading', { name: 'Styles' })).toBeVisible();
}

async function attach(): Promise<Page> {
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${debugPort}`);
  const page = browser.contexts()[0]?.pages()[0];
  if (!page) throw new Error('desktop page did not open');
  return page;
}

test('組み込み以外の表示スタイルを1つアプリの定義に加える', async () => {
  test.skip(process.env.STYLE_E2E !== '1', 'opens the desktop folder dialog; set STYLE_E2E=1');
  test.setTimeout(180_000);
  expect(powershell('@(Get-Process token-monitor-turzx -ErrorAction SilentlyContinue).Count'), 'stop every running Token Dashboard first').toBe('0');
  const work = mkdtempSync(join(tmpdir(), 'turzx-add-style-e2e-'));
  const dataDir = join(work, 'data');
  const styleDir = join(work, 'sample-style');
  const invalidDir = join(work, 'invalid');
  const imageDir = join(work, 'bad-image');
  writeStyle(styleDir, 'night', 'Night', 'body { color: black; }');
  writeFileSync(join(work, 'outside.txt'), 'outside');
  mkdirSync(invalidDir);
  writeFileSync(join(invalidDir, 'note.txt'), 'not a style');
  writeStyle(imageDir, 'broken', 'Broken', 'body { background: url(https://example.com/a.png); }');
  const hub = await startHub();
  saveSettings(dataDir, hub.url);
  let pid = 0;
  let cleanupError: unknown;
  try {
    startApp(dataDir);
    pid = (await waitFor('the app to start', () => appProcesses(exe)))[0];
    await waitFor('the hub stream', () => hub.streams() > 0);
    hub.send('snapshot', { periods: { today: { totalTokens: 1, costUsd: 0 }, month: { totalTokens: 1, costUsd: 0 }, allTime: { totalTokens: 1, costUsd: 0 } }, limits: { providers: [] } });
    const page = await attach();
    await test.step('開始条件', async () => {
      await openStyles(page);
      await expect(page.getByRole('article', { name: 'Gauges' }).getByText('Built-in', { exact: true })).toBeVisible();
      await expect(page.getByRole('article', { name: 'Bars' }).getByText('Built-in', { exact: true })).toBeVisible();
      await expect(page.getByRole('article', { name: 'Gauges' }).getByText('In use', { exact: true })).toBeVisible();
      await expect(page.getByRole('article', { name: 'Night' })).toHaveCount(0);
    });
    await test.step('手順1', async () => {
      await addStyle(page, 'ok', styleDir);
      const night = page.getByRole('article', { name: 'Night' });
      await expect(night).toBeVisible();
      await expect(night.getByText('Built-in')).toHaveCount(0);
      await expect(night.getByRole('img', { name: 'Night preview' })).toBeVisible();
      await expect(page.getByRole('article', { name: 'Gauges' }).getByText('In use', { exact: true })).toBeVisible();
      const stored = JSON.parse(readFileSync(join(dataDir, 'styles', 'night', 'theme.json'), 'utf8')) as { id: string; name: string };
      expect(stored).toMatchObject({ id: 'night', name: 'Night' });
      expect(readdirSync(join(dataDir, 'styles', 'night')).sort()).toEqual(['style.css', 'template.hbs', 'theme.json']);
      writeFileSync(join(styleDir, 'theme.json'), JSON.stringify({ formatVersion: 1, id: 'night', name: 'Changed', width: 1920, height: 462, template: 'template.hbs', stylesheet: 'style.css' }));
      expect(JSON.parse(readFileSync(join(dataDir, 'styles', 'night', 'theme.json'), 'utf8')).name).toBe('Night');
    });
    await test.step('受け入れ条件', async () => {
      const before = readdirSync(join(dataDir, 'styles'));
      await addStyle(page, 'ok', styleDir);
      await expect(page.getByRole('alert', { name: 'DUPLICATE' })).toContainText('This style id is already defined.');
      expect(readdirSync(join(dataDir, 'styles'))).toEqual(before);
      await addStyle(page, 'cancel');
      await expect(page.getByRole('alert')).toHaveCount(0);
      expect(readdirSync(join(dataDir, 'styles'))).toEqual(before);
      await addStyle(page, 'ok', invalidDir);
      await expect(page.getByRole('alert', { name: 'INVALID' })).toContainText('This folder is not a valid style.');
      expect(readdirSync(join(dataDir, 'styles'))).toEqual(before);
      await addStyle(page, 'ok', imageDir);
      await expect(page.getByRole('alert', { name: 'INVALID' })).toContainText('This folder is not a valid style.');
      expect(readdirSync(join(dataDir, 'styles', 'night')).sort()).toEqual(['style.css', 'template.hbs', 'theme.json']);
      expect(readdirSync(join(dataDir, 'styles')).filter(name => name !== '.incoming')).toEqual(['night']);
      process.kill(pid);
      await waitFor('the app to exit', () => appProcesses(exe).length === 0);
      startApp(dataDir);
      pid = (await waitFor('the app to start again', () => appProcesses(exe)))[0];
      const restarted = await attach();
      await openStyles(restarted);
      await expect(restarted.getByRole('article', { name: 'Night' })).toBeVisible();
      await expect(restarted.getByRole('article', { name: 'Gauges' })).toBeVisible();
      await expect(restarted.getByRole('article', { name: 'Bars' })).toBeVisible();
      await expect(restarted.getByRole('article', { name: 'Gauges' }).getByText('In use', { exact: true })).toBeVisible();
      expect(JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8')).limitStyle).toBe('Gauges');
    });
  } finally {
    for (const running of appProcesses(exe)) {
      try { process.kill(running); } catch { /* already exited */ }
    }
    const deadline = Date.now() + 15_000;
    while (appProcesses(exe).length > 0 && Date.now() < deadline) {
      await new Promise(done => setTimeout(done, 200));
    }
    await hub.close();
    for (let attempt = 0; attempt < 20; attempt++) {
      try {
        rmSync(work, { recursive: true, force: true });
        cleanupError = undefined;
        break;
      } catch (error) {
        cleanupError = error;
        if (attempt < 19) await new Promise(done => setTimeout(done, 300));
      }
    }
  }
  if (cleanupError) throw cleanupError;
});
