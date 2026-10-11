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
