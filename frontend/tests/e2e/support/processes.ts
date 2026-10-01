import { spawn } from 'node:child_process';

// One tokscale process as sampled: its arguments and the first and last times it was seen.
export interface Run { pid: number; args: string; start: number; end: number }

// Samples the tokscale processes started by parent every quarter second. A process shorter
// than that can be missed, so only runs of a second or more are safe to count.
export function watchTokscale(parent: number) {
  const runs = new Map<number, Run>();
  const script = `while ($true) {
  $at = [DateTimeOffset]::Now.ToUnixTimeMilliseconds()
  $list = @(Get-CimInstance Win32_Process -Filter "Name='tokscale.exe' AND ParentProcessId=${parent}" | ForEach-Object { @{ pid = $_.ProcessId; args = ($_.CommandLine -replace '^.*tokscale\\.exe"?\\s*', '') } })
  ConvertTo-Json -Compress -Depth 3 @{ at = $at; list = $list }
  Start-Sleep -Milliseconds 250
}`;
  const child = spawn('powershell', ['-NoProfile', '-Command', script]);
  let pending = '';
  let sampled = 0;
  child.stdout.setEncoding('utf8').on('data', (chunk: string) => {
    pending += chunk;
    const lines = pending.split('\n');
    pending = lines.pop() ?? '';
    for (const line of lines) {
      if (!line.trim()) continue;
      const sample = JSON.parse(line) as { at: number; list: { pid: number; args: string }[] };
      for (const { pid, args } of sample.list) {
        const run = runs.get(pid);
        if (run) run.end = sample.at;
        else runs.set(pid, { pid, args, start: sample.at, end: sample.at });
      }
      sampled = sample.at;
    }
  });
  return {
    runs: (args: string) => [...runs.values()].filter(run => run.args === args).sort((a, b) => a.start - b.start),
    // Whether any tokscale of parent is running now, from a sample taken after this call.
    async running() {
      const after = Date.now();
      while (sampled < after) await new Promise(done => setTimeout(done, 100));
      return [...runs.values()].some(run => run.end === sampled);
    },
    stop: () => { child.kill(); },
  };
}
