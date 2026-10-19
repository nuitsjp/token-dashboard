import { test, expect, chromium, type Page, type Response } from '@playwright/test';
import { spawn, type ChildProcess } from 'node:child_process';
import { chmodSync, copyFileSync, cpSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { appProcesses, powershell, waitFor } from '../../support/desktop';
import { startHub } from '../../support/hub';
import { startServer } from '../../support/server';
import { shortIntervals } from '../../support/local';
import type { FrameRequest } from '../../../../bindings/token-monitor-turzx/internal/display/models';

// Opens the desktop folder dialog, so it runs only when STYLE_E2E=1. The development
// build in bin/ is required: the production build has no remote debugging port.
const root = resolve(import.meta.dirname, '../../../../..');
const exe = join(root, 'bin', process.platform === 'win32' ? 'token-monitor-turzx.exe' : 'token-monitor-turzx');
const debugPort = '9471';
const appID = 'io.github.nuitsjp.token-monitor-turzx';
// CDP contexts belong to the native app and disappear during the restart under test.
test.use({ trace: 'off' });

function quote(value: string) {
  return `'${value.replaceAll("'", "''")}'`;
}

function saveSettings(dataDir: string, url: string, small: boolean) {
  const plain = JSON.stringify({ url, token: 'e2e-hub-token' });
  const sealed = powershell(`Add-Type -AssemblyName System.Security
[Convert]::ToBase64String([Security.Cryptography.ProtectedData]::Protect([Text.Encoding]::UTF8.GetBytes(${quote(plain)}), [Text.Encoding]::UTF8.GetBytes('${appID}'), 'CurrentUser'))`);
  mkdirSync(dataDir, { recursive: true });
  // A distinct hardware suffix identifies the model but never opens the user's physical USB.
  writeFileSync(join(dataDir, 'settings.json'), JSON.stringify({ source: 'Hub', connection: sealed,
    displayID: small ? 'USB\\VID_1A86&PID_5722&E2E\\USB35INCHIPSV2' : 'USB\\VID_1CBE&PID_0092\\E2E92',
    displayName: small ? 'TURZX E2E 3.5-inch' : 'TURZX E2E 9.2-inch',
    // CDP input does not reset GetLastInputInfo. Keep the workstation's prior idle time
    // from turning off rendering during this foreground import/restart scenario.
    limitStyle: small ? 'gauges-35-landscape' : 'Gauges', serviceCycleSeconds: 60, cycleSeconds: 37, awayMinutes: 240,
    presentation: { reverse180: false, skipFull5h: false, services: {} },
  }));
}

function writeStyle(dir: string, id: string, name: string, css: string, dataLayout?: string) {
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, 'theme.json'), JSON.stringify({
    formatVersion: 1, id, name, width: 1920, height: 462, dataLayout, template: 'template.hbs', stylesheet: 'style.css',
  }));
  copyFileSync(join(root, 'themes', dataLayout === 'bars' ? 'bars' : 'gauges', 'template.hbs'), join(dir, 'template.hbs'));
  writeFileSync(join(dir, 'style.css'), css);
}

function startApp(dataDir: string) {
  spawn(exe, ['--show'], {
    env: { ...process.env, WAILS_DATA_DIR: dataDir, WAILS_WEBVIEW_DEBUG_PORT: debugPort },
    detached: true, stdio: 'ignore', windowsHide: true,
  }).unref();
}

function powershellAsync(script: string): ChildProcess {
  return spawn('powershell.exe', ['-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')], { stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true });
}

function finished(child: ChildProcess) {
  return new Promise<void>((resolve, reject) => {
    let err = '';
    child.stderr?.on('data', chunk => { err += chunk; });
    child.on('exit', code => code === 0 ? resolve() : reject(new Error(err || `exit ${code}`)));
  });
}

// Select the actual Shell tree item so that BFFM_SELCHANGED validates its PIDL. Typing
// an edit path bypasses that callback and would miss a disabled confirmation button.
// This runs beside the Add style click because the native dialog is modal.
function chooseFolder(pid: number, action: 'ok' | 'cancel', folder = '') {
  return powershellAsync(`
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public class StyleFolderDialog {
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowExW(IntPtr parent, IntPtr after, string cls, string title);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool IsWindowEnabled(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr SendMessageW(IntPtr h, uint msg, IntPtr w, IntPtr l);
  [DllImport("kernel32.dll", SetLastError=true)] static extern IntPtr OpenProcess(uint access, bool inherit, uint pid);
  [DllImport("kernel32.dll", SetLastError=true)] static extern IntPtr VirtualAllocEx(IntPtr process, IntPtr address, UIntPtr size, uint allocation, uint protection);
  [DllImport("kernel32.dll", SetLastError=true)] static extern bool WriteProcessMemory(IntPtr process, IntPtr address, byte[] data, UIntPtr size, out UIntPtr written);
  [DllImport("kernel32.dll")] static extern bool VirtualFreeEx(IntPtr process, IntPtr address, UIntPtr size, uint type);
  [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);

  public static void SelectFolder(IntPtr dialog, uint pid, string path) {
    // Messages above WM_USER do not marshal pointer arguments across processes.
    // Keep the UTF-16 path in the app's address space during BFFM_SETSELECTIONW.
    byte[] data = System.Text.Encoding.Unicode.GetBytes(path + "\\0");
    IntPtr process = OpenProcess(0x0028, false, pid); // VM_OPERATION | VM_WRITE
    if (process == IntPtr.Zero) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
    IntPtr remote = IntPtr.Zero;
    try {
      remote = VirtualAllocEx(process, IntPtr.Zero, (UIntPtr)data.Length, 0x3000, 0x04);
      if (remote == IntPtr.Zero) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
      UIntPtr written;
      if (!WriteProcessMemory(process, remote, data, (UIntPtr)data.Length, out written) || written.ToUInt64() != (ulong)data.Length)
        throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
      SendMessageW(dialog, 0x0467, (IntPtr)1, remote);
    } finally {
      if (remote != IntPtr.Zero) VirtualFreeEx(process, remote, UIntPtr.Zero, 0x8000);
      CloseHandle(process);
    }
  }
}
'@
$deadline = (Get-Date).AddSeconds(45)
$dlg = [IntPtr]::Zero
while ($dlg -eq [IntPtr]::Zero -and (Get-Date) -lt $deadline) {
  $candidate = [IntPtr]::Zero
  do {
    $candidate = [StyleFolderDialog]::FindWindowExW([IntPtr]::Zero, $candidate, '#32770', 'Select a folder')
    [uint32]$owner = 0
    if ($candidate -ne [IntPtr]::Zero) {
      [void][StyleFolderDialog]::GetWindowThreadProcessId($candidate, [ref]$owner)
      if ($owner -eq ${pid}) { $dlg = $candidate; break }
    }
  } while ($candidate -ne [IntPtr]::Zero)
  if ($dlg -eq [IntPtr]::Zero) { Start-Sleep -Milliseconds 200 }
}
if ($dlg -eq [IntPtr]::Zero) { throw 'folder dialog did not open' }
$cancel = [StyleFolderDialog]::GetDlgItem($dlg, 2)
if (${quote(action)} -eq 'cancel') { [void][StyleFolderDialog]::SendMessageW($cancel, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero); return }
try {
  # BFFM_SETSELECTIONW chooses the tree item; it does not populate the editable path.
  [StyleFolderDialog]::SelectFolder($dlg, ${pid}, ${quote(folder)})
  $ok = [StyleFolderDialog]::GetDlgItem($dlg, 1)
  $ready = (Get-Date).AddSeconds(5)
  while (-not [StyleFolderDialog]::IsWindowEnabled($ok) -and (Get-Date) -lt $ready) { Start-Sleep -Milliseconds 50 }
  if (-not [StyleFolderDialog]::IsWindowEnabled($ok)) { throw 'OK must be enabled after selecting a folder in the tree' }
  # BM_CLICK exercises the enabled button instead of bypassing it with WM_COMMAND.
  [void][StyleFolderDialog]::SendMessageW($ok, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero)
} catch {
  [void][StyleFolderDialog]::SendMessageW($cancel, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero)
  throw
}
`);
}

async function addStyle(page: Page, pid: number, action: 'ok' | 'cancel', folder = '') {
  const dialog = finished(chooseFolder(pid, action, folder));
  const clicked = page.getByRole('button', { name: 'Add style' }).click({ timeout: 90_000 });
  await Promise.all([clicked, dialog]);
}

async function openStyles(page: Page) {
  await page.goto('http://wails.localhost/#/styles');
  await expect(page.getByRole('heading', { name: 'Styles' })).toBeVisible();
}

async function attach(): Promise<Page> {
  await expect.poll(async () => {
    try { return (await fetch(`http://127.0.0.1:${debugPort}/json/version`)).ok; }
    catch { return false; }
  }, { timeout: 20_000 }).toBe(true);
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${debugPort}`);
  const page = browser.contexts()[0]?.pages()[0];
  if (!page) throw new Error('desktop page did not open');
  return page;
}

const settingsAPI = 'token-monitor-turzx/internal/settings.Service.';
const displayAPI = 'token-monitor-turzx/internal/display.Service.';
const samples = [
  { id: 'sample-midnight-35-landscape', name: 'Midnight Gauges 3.5" Landscape', width: 480, height: 320 },
  { id: 'sample-paper-35-portrait', name: 'Paper Bars 3.5" Portrait', width: 320, height: 480 },
  { id: 'sample-terminal-35-landscape', name: 'Terminal Bars 3.5" Landscape', width: 480, height: 320 },
  { id: 'sample-orbit-35-portrait', name: 'Orbit Gauges 3.5" Portrait', width: 320, height: 480 },
];
const largeStyles = [
  { id: 'legacy', name: 'Legacy Gauges', width: 1920, height: 462 },
  { id: 'external-bars', name: 'External Bars', width: 1920, height: 462 },
];
async function rpc<T>(page: Page, method: string, args: unknown[] = []): Promise<T> {
  return page.evaluate(async ({ method, args }) => {
    const response = await fetch('/wails/runtime', { method: 'POST', body: JSON.stringify({ object: 0, method: 0, args: { 'call-id': crypto.randomUUID(), methodName: method, args } }) });
    if (!response.ok) throw new Error(await response.text());
    const body = await response.text();
    return body ? JSON.parse(body) : undefined;
  }, { method, args });
}
function stats(remaining: number) {
  const period = { totalTokens: 987654, costUsd: 99, clients: { Alpha: 123456789012, Beta: 0 }, clientCosts: { Alpha: 12.34, Beta: 0 } };
  return { periods: { today: period, month: period, allTime: period }, limits: { providers: [
    { provider: 'Alpha', planLabel: 'Pro', windows: [{ kind: 'session', showMeter: true, remainingPercent: remaining, resetsAt: new Date(Date.now() + 3600000).toISOString() }] },
    { provider: 'Beta', planLabel: 'Free', windows: [{ kind: 'weekly', showMeter: true, remainingPercent: 80 }] },
  ] } };
}

for (const small of [true, false]) test(`外部スタイルを取り込み、適用と再起動で復元する (${small ? '3.5' : '9.2'}インチ) @style-import`, async () => {
  test.skip(process.env.STYLE_E2E !== '1' || process.platform !== 'win32', 'native folder dialog; node scripts/run.mjs test:styles');
  test.setTimeout(240_000);
  expect(powershell('@(Get-Process token-monitor-turzx -ErrorAction SilentlyContinue).Count'), 'stop every running Token Dashboard first').toBe('0');
  const work = mkdtempSync(join(tmpdir(), 'turzx-add-style-e2e-'));
  const dataDir = join(work, 'data');
  const selected = small ? samples : largeStyles;
  for (const style of samples) cpSync(join(root, 'examples/styles', style.id), join(work, style.id), { recursive: true });
  for (const style of largeStyles) writeStyle(join(work, style.id), style.id, style.name, readFileSync(join(root, 'themes', style.id === 'legacy' ? 'gauges' : 'bars', 'style.css'), 'utf8'), style.id === 'legacy' ? undefined : 'bars');
  writeStyle(join(work, 'invalid'), 'invalid', 'Invalid', 'body{}', 'bars');
  const invalidManifest = join(work, 'invalid', 'theme.json');
  writeFileSync(invalidManifest, JSON.stringify({ ...JSON.parse(readFileSync(invalidManifest, 'utf8')), width: 480, height: 320, dataLayout: 'bars' }));
  writeStyle(join(work, 'bad-image'), 'bad-image', 'Bad image', 'body { background: url(https://example.invalid/a.png); }');
  const hub = await startHub();
  saveSettings(dataDir, hub.url, small);
  const saved = () => JSON.parse(readFileSync(join(dataDir, 'settings.json'), 'utf8'));
  // DPAPI ciphertext is randomized on a Hub save; compare the other persisted values
  // and check the decrypted connection through Get rather than exposing encrypted blobs.
  const preferences = (value: Record<string, unknown>) => Object.fromEntries(Object.entries(value).filter(([key]) => key !== 'connection'));
  const stored = () => readdirSync(join(dataDir, 'styles')).filter(name => name !== '.incoming').sort();
  let pid = 0;
  let cleanupError: unknown;
  let page: Page;
  let completed: FrameRequest | undefined;
  let renderError = '';
  const observe = (target: Page) => {
    const requests = new Map<number, FrameRequest>();
    completed = undefined;
    target.on('response', async response => {
      if (!response.url().endsWith('/wails/runtime')) return;
      try {
        const call = response.request().postDataJSON()?.args;
        if (call?.methodName === displayAPI + 'RenderRequest' && response.ok()) {
          const frame = await response.json() as FrameRequest | null;
          if (frame) requests.set(frame.id, frame);
        }
        if (call?.methodName === displayAPI + 'CompleteFrame' && response.ok()) {
          if (call.args[3]) renderError = call.args[3];
          else if (call.args[1] && call.args[2]) { renderError = ''; completed = requests.get(call.args[0]); }
        }
      } catch { /* Unrelated runtime messages have no frame body. */ }
    });
  };
  const selectStyle = async (style: typeof samples[number]) => {
    await page.getByRole('textbox', { name: 'Display style', exact: true }).click();
    await page.getByRole('option', { name: style.name, exact: true }).click();
    await expect.poll(() => saved().limitStyle).toBe(style.id);
    await expect.poll(() => renderError || completed?.theme, { timeout: 20_000 }).toBe(style.id);
    await expect.poll(() => page.getByRole('img', { name: 'Display preview', exact: true }).evaluate((img: HTMLImageElement) => [img.naturalWidth, img.naturalHeight])).toEqual([style.width, style.height]);
  };
  const stopApp = async () => {
    await rpc(page, 'token-monitor-turzx/internal/desktop.Service.ConfirmQuit');
    await page.evaluate(() => { setTimeout(() => void fetch('/wails/runtime', { method: 'POST', body: JSON.stringify({ object: 2, method: 2 }) }), 100); });
    await waitFor('the app to exit', () => appProcesses(exe).length === 0);
  };
  try {
    startApp(dataDir);
    pid = (await waitFor('the app to start', () => appProcesses(exe)))[0];
    page = await attach(); observe(page);
    await expect(page.getByRole('textbox', { name: 'Display style', exact: true })).toBeVisible();
    await waitFor('the hub stream', () => hub.streams() > 0);
    hub.send('snapshot', stats(20));
    await test.step('開始条件', async () => {
      await openStyles(page);
      await expect(page.getByRole('article')).toHaveCount(6);
      for (const style of [...samples, ...largeStyles]) await expect(page.getByRole('article', { name: style.name, exact: true })).toHaveCount(0);
    });
    const initialSettings = saved();
    await test.step('手順1', async () => {
      for (const style of [...samples, ...largeStyles]) {
        await addStyle(page, pid, 'ok', join(work, style.id));
        await expect(page.getByRole('article', { name: style.name, exact: true })).toBeVisible();
        expect(saved()).toEqual(initialSettings);
      }
      expect(stored()).toEqual([...samples, ...largeStyles].map(style => style.id).sort());
      const badge = 'assets/badge.svg';
      expect(readFileSync(join(dataDir, 'styles', samples[1].id, badge))).toEqual(readFileSync(join(work, samples[1].id, badge)));
    });
    await test.step('手順2', async () => {
      for (const style of [...samples, ...largeStyles]) {
        const card = page.getByRole('article', { name: style.name, exact: true });
        await expect(card.getByText('Built-in', { exact: true })).toHaveCount(0);
        try {
          await expect.poll(() => card.getByRole('img').evaluate((img: HTMLImageElement) => [img.naturalWidth, img.naturalHeight]), { timeout: 20_000 }).toEqual([style.width, style.height]);
        } catch (error) { throw new Error(`${style.id}: ${await card.innerText()}`, { cause: error }); }
      }
      await expect(page.getByRole('article')).toHaveCount(12);
      await expect.poll(() => page.getByRole('article').evaluateAll(nodes => nodes.map(node => node.getAttribute('aria-label')))).toEqual([
        'Gauges', 'Bars', 'Bars 3.5" Landscape', 'Bars 3.5" Portrait', 'Gauges 3.5" Landscape', 'Gauges 3.5" Portrait',
        ...[...samples, ...largeStyles].map(style => style.name).sort(),
      ]);
    });
    await test.step('手順3', async () => {
      await page.getByRole('link', { name: 'Display', exact: true }).click();
      await page.getByRole('textbox', { name: 'Display style', exact: true }).click();
      await expect(page.getByRole('option')).toHaveCount(small ? 8 : 4);
      for (const style of small ? largeStyles : samples) await expect(page.getByRole('option', { name: style.name, exact: true })).toHaveCount(0);
      await page.keyboard.press('Escape');
      for (const style of selected) {
        await selectStyle(style);
        expect(preferences(saved())).toEqual({ ...preferences(initialSettings), limitStyle: style.id });
        expect(await rpc(page, settingsAPI + 'Get')).toMatchObject({ url: hub.url, tokenSet: true });
      }
      const view = await rpc<{ themes: { id: string; dataLayout: string }[] }>(page, settingsAPI + 'Get');
      expect(view.themes.find(theme => theme.id === 'legacy')?.dataLayout).toBe('gauges');
      expect(view.themes.find(theme => theme.id === 'external-bars')?.dataLayout).toBe('bars');
    });
    await test.step('手順4', async () => {
      await expect.poll(() => completed?.data.available).toBe(true);
      const previous = completed!;
      const oldImage = await page.getByRole('img', { name: 'Display preview', exact: true }).getAttribute('src');
      hub.send('stats', stats(50));
      await expect.poll(() => completed?.id).not.toBe(previous.id);
      await expect.poll(() => page.getByRole('img', { name: 'Display preview', exact: true }).getAttribute('src')).not.toBe(oldImage);
      expect(completed!.theme).toBe(selected.at(-1)!.id);
      if (small) {
        expect(completed!.data.service?.key).toBe(previous.data.service?.key);
        expect(completed!.data.service?.count).toBe(2);
        const interval = page.getByRole('textbox', { name: 'Page interval', exact: true });
        await interval.fill('5'); await interval.press('Enter');
        await expect.poll(() => saved().serviceCycleSeconds).toBe(5);
        const key = completed!.data.service?.key;
        await expect.poll(() => completed?.data.service?.key, { timeout: 10_000 }).not.toBe(key);
        await interval.fill('60'); await interval.press('Enter');
        await expect.poll(() => saved().serviceCycleSeconds).toBe(60);
      } else await expect(page.getByRole('textbox', { name: 'Page interval', exact: true })).toHaveCount(0);
    });
    await test.step('手順5', async () => {
      const before = saved();
      for (const style of [...samples, ...largeStyles]) rmSync(join(work, style.id), { recursive: true });
      await stopApp();
      startApp(dataDir);
      pid = (await waitFor('the app to restart', () => appProcesses(exe)))[0];
      page = await attach(); observe(page);
      await expect(page.getByRole('textbox', { name: 'Display style', exact: true })).toBeVisible();
      await waitFor('the hub to reconnect', () => hub.streams() > 0);
      hub.send('snapshot', stats(50));
      await expect(page.getByRole('textbox', { name: 'Display style', exact: true })).toHaveValue(selected.at(-1)!.name);
      await expect.poll(() => completed?.theme, { timeout: 20_000 }).toBe(selected.at(-1)!.id);
      await expect.poll(() => completed?.data.available).toBe(true);
      expect(saved()).toEqual(before);
    });
    await test.step('受け入れ条件', async () => {
      await page.getByRole('link', { name: 'Styles', exact: true }).click();
      const before = saved();
      const ids = stored();
      await addStyle(page, pid, 'cancel');
      expect(saved()).toEqual(before);
      expect(stored()).toEqual(ids);
      await addStyle(page, pid, 'ok', join(work, 'invalid'));
      await expect(page.getByRole('alert', { name: 'INVALID' })).toContainText('This folder is not a valid style.');
      await addStyle(page, pid, 'ok', join(work, 'bad-image'));
      await expect(page.getByRole('alert', { name: 'INVALID' })).toContainText('This folder is not a valid style.');
      await addStyle(page, pid, 'ok', join(dataDir, 'styles', selected[0].id));
      await expect(page.getByRole('alert', { name: 'DUPLICATE' })).toContainText('This style id is already defined.');
      expect(stored()).toEqual(ids);
      expect(saved()).toEqual(before);
      const selectedCard = page.getByRole('article', { name: selected.at(-1)!.name, exact: true });
      await expect(selectedCard.getByRole('button', { name: /^Delete / })).toBeDisabled();
      await expect(selectedCard).toContainText('Switch to another style before deleting.');
      const file = join(dataDir, 'settings.json');
      chmodSync(file, 0o444);
      try {
        await page.getByRole('link', { name: 'Display', exact: true }).click();
        await page.getByRole('textbox', { name: 'Display style', exact: true }).click();
        await page.getByRole('option', { name: selected[0].name, exact: true }).click();
        await expect(page.getByRole('alert')).toBeVisible();
        expect(saved()).toEqual(before);
        await expect(page.getByRole('textbox', { name: 'Display style', exact: true })).toHaveValue(selected.at(-1)!.name);
      } finally { chmodSync(file, 0o600); }
      expect(readdirSync(join(dataDir, 'styles', '.incoming'))).toEqual([]);
      await stopApp();
    });
  } finally {
    for (const running of appProcesses(exe)) { try { process.kill(running); } catch { /* already exited */ } }
    await waitFor('desktop cleanup', () => appProcesses(exe).length === 0);
    await hub.close();
    for (let attempt = 0; attempt < 20; attempt++) {
      try { rmSync(work, { recursive: true, force: true }); cleanupError = undefined; break; }
      catch (error) { cleanupError = error; await new Promise(done => setTimeout(done, 300)); }
    }
  }
  if (cleanupError) throw cleanupError;
});

for (const small of [true, false]) test(`未選択スタイルの画像欠損が正常な表示を止めない (${small ? '3.5' : '9.2'}インチ)`, async ({ page, context }) => {
  test.setTimeout(90_000);
  const work = mkdtempSync(join(tmpdir(), 'turzx-style-isolation-'));
  const dataDir = join(work, 'data');
  const hub = await startHub();
  saveSettings(dataDir, hub.url, small);
  const broken = samples[1];
  const healthy = small ? samples[0] : largeStyles[1];
  const store = join(dataDir, 'styles');
  cpSync(join(root, 'examples/styles', broken.id), join(store, broken.id), { recursive: true });
  const missingImage = join(store, broken.id, 'assets/badge.svg');
  rmSync(missingImage);
  if (small) cpSync(join(root, 'examples/styles', healthy.id), join(store, healthy.id), { recursive: true });
  else writeStyle(join(store, healthy.id), healthy.id, healthy.name, readFileSync(join(root, 'themes/bars/style.css'), 'utf8'), 'bars');
  const port = small ? 34143 : 34144;
  let server = await startServer(dataDir, port, shortIntervals);
  let completed: FrameRequest | undefined;
  let renderError = '';
  let stopObserving: (() => void) | undefined;
  const observe = (target: Page) => {
    const requests = new Map<number, FrameRequest>();
    const onResponse = async (response: Response) => {
      if (!response.url().endsWith('/wails/runtime') || !response.ok()) return;
      const call = response.request().postDataJSON()?.args;
      if (call?.methodName === displayAPI + 'RenderRequest') {
        let request: FrameRequest | null;
        try {
          request = await response.json() as FrameRequest | null;
        } catch (error) {
          if (target.isClosed()) return;
          throw error;
        }
        if (request) requests.set(request.id, request);
      }
      if (call?.methodName === displayAPI + 'CompleteFrame') {
        if (call.args[3]) renderError = call.args[3];
        else if (call.args[1] && call.args[2]) completed = requests.get(call.args[0]);
      }
    };
    target.on('response', onResponse);
    return () => target.off('response', onResponse);
  };
  const waitForFrame = async (id: string, width: number, height: number) => {
    await expect.poll(() => ({ theme: completed?.theme, available: completed?.data.available }), { timeout: 15_000 })
      .toEqual({ theme: id, available: true });
    await expect.poll(() => page.getByRole('img', { name: 'Display preview', exact: true })
      .evaluate((img: HTMLImageElement) => [img.naturalWidth, img.naturalHeight])).toEqual([width, height]);
  };
  try {
    stopObserving = observe(page);
    await expect.poll(() => hub.streams()).toBe(1);
    hub.send('snapshot', stats(20));
    await page.goto(server.url);
    await waitForFrame(small ? 'gauges-35-landscape' : 'gauges', small ? 480 : 1920, small ? 320 : 462);

    // The resident output must keep updating even though the unrelated Paper image is absent.
    const previousID = completed!.id;
    hub.send('stats', stats(50));
    await expect.poll(() => completed?.id).toBeGreaterThan(previousID);
    await expect.poll(() => small
      ? completed?.data.service?.circles?.[0]?.windows?.[0]?.remainingPercent
      : completed?.data.gaugePanels?.[0]?.circles?.[0]?.windows?.[0]?.remainingPercent).toBe(50);
    expect(renderError).toBe('');
    await page.getByRole('textbox', { name: 'Display style', exact: true }).click();
    await page.getByRole('option', { name: healthy.name, exact: true }).click();
    await waitForFrame(healthy.id, healthy.width, healthy.height);

    // A healthy external style is restored without opening the Styles list or reading Paper.
    stopObserving?.(); stopObserving = undefined;
    await page.close(); await server.stop();
    await expect.poll(() => hub.streams()).toBe(0);
    completed = undefined;
    server = await startServer(dataDir, port, shortIntervals);
    await expect.poll(() => hub.streams()).toBe(1);
    hub.send('snapshot', stats(50));
    page = await context.newPage(); stopObserving = observe(page); await page.goto(server.url);
    await waitForFrame(healthy.id, healthy.width, healthy.height);
    expect(renderError).toBe('');

    if (small) {
      await page.getByRole('textbox', { name: 'Display style', exact: true }).click();
      await page.getByRole('option', { name: broken.name, exact: true }).click();
      await expect.poll(() => renderError).toContain('badge.svg');
      expect(completed!.theme).toBe(healthy.id);
      const file = join(dataDir, 'settings.json');
      expect(JSON.parse(readFileSync(file, 'utf8')).limitStyle).toBe(broken.id);
      // Failed loads are retried: repairing the asset resumes the selected style, without a fallback.
      copyFileSync(join(root, 'examples/styles', broken.id, 'assets/badge.svg'), missingImage);
      hub.send('stats', stats(70));
      await waitForFrame(broken.id, broken.width, broken.height);
      expect(JSON.parse(readFileSync(file, 'utf8')).limitStyle).toBe(broken.id);
    }
  } finally {
    stopObserving?.();
    await server.stop(); await hub.close();
    rmSync(work, { recursive: true, force: true });
  }
});
