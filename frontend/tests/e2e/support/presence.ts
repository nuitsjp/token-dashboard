import { spawnSync } from 'node:child_process';

// What Windows posts to a window that asked for the power and session notifications.
const messages = {
  lock: { message: 0x02b1, wParam: 7 }, // WM_WTSSESSION_CHANGE, WTS_SESSION_LOCK
  unlock: { message: 0x02b1, wParam: 8 }, // WTS_SESSION_UNLOCK
  suspend: { message: 0x0218, wParam: 4 }, // WM_POWERBROADCAST, PBT_APMSUSPEND
  resume: { message: 0x0218, wParam: 0x12 }, // PBT_APMRESUMEAUTOMATIC
} as const;

// Posts the message to the hidden presence window of the server with the given process ID, as Windows
// would, and so reaches the real watcher.
export function signal(pid: number, what: keyof typeof messages) {
  const { message, wParam } = messages[what];
  const script = `
Add-Type -Namespace W -Name U -MemberDefinition '
[DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowW(string c, string t);
[DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr h, uint m, IntPtr w, IntPtr l);'
$h = [W.U]::FindWindowW('TokenMonitorTurzxPresence', 'TokenMonitorTurzxPresence-${pid}')
if ($h -eq [IntPtr]::Zero) { exit 2 }
if (-not [W.U]::PostMessageW($h, ${message}, [IntPtr]${wParam}, [IntPtr]::Zero)) { exit 3 }`;
  const result = spawnSync('powershell', ['-NoProfile', '-NonInteractive', '-Command', script], { encoding: 'utf8' });
  if (result.status !== 0) throw new Error(`Cannot post ${what} to the presence window of ${pid}: ${result.status} ${result.stderr}`);
}

// The Windows test host may have its physical monitor off when the suite starts. Send the
// display-state-on notification only to the server's hidden presence window; this does not change
// the OS monitor state and does not reach any other process.
export function clearMonitorOff(pid: number) {
  if (process.platform !== 'win32') return;
  const script = `
Add-Type -TypeDefinition @"
using System; using System.Runtime.InteropServices; using System.Threading;
public class TokenMonitorPower {
[DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern IntPtr FindWindowW(string c, string t);
[DllImport("user32.dll")] static extern IntPtr SendMessageW(IntPtr h, uint m, IntPtr w, IntPtr l);
public static void On(int pid) {
var h = IntPtr.Zero;
for (var attempt = 0; attempt < 50 && h == IntPtr.Zero; attempt++) {
 h = FindWindowW("TokenMonitorTurzxPresence", "TokenMonitorTurzxPresence-" + pid);
 if (h == IntPtr.Zero) Thread.Sleep(100);
}
if (h == IntPtr.Zero) throw new Exception("No presence window");
var bytes = new byte[24];
Array.Copy(new Guid("6fe69556-704a-47a0-8f24-c28d936fda47").ToByteArray(), bytes, 16);
bytes[16] = 4; bytes[20] = 1;
var p = Marshal.AllocHGlobal(24);
try {
 Marshal.Copy(bytes, 0, p, 24);
 if (SendMessageW(h, 0x218, (IntPtr)0x8013, p) != (IntPtr)1) throw new Exception("Monitor-on notification failed");
}
finally { Marshal.FreeHGlobal(p); }
}
}
"@
[TokenMonitorPower]::On(${pid})`;
  const result = spawnSync('powershell', ['-NoProfile', '-NonInteractive', '-Command', script], { encoding: 'utf8', windowsHide: true });
  if (result.status !== 0) throw new Error(`Cannot clear monitor-off state for ${pid}: ${result.status} ${result.stderr}`);
}
