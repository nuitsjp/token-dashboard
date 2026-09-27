import { createServer, type IncomingHttpHeaders, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';

// A controllable stand-in for the Token Monitor Hub SSE endpoint.
export async function startHub() {
  const streams = new Set<ServerResponse>();
  const requests: { path: string; headers: IncomingHttpHeaders }[] = [];
  const server = createServer((req, res) => {
    requests.push({ path: req.url ?? '', headers: req.headers });
    if (req.url !== '/api/stats/stream') { res.writeHead(404).end(); return; }
    res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
    res.write(': connected\n\n');
    streams.add(res);
    req.on('close', () => streams.delete(res));
  });
  await new Promise<void>(done => server.listen(0, '127.0.0.1', done));
  return {
    url: `http://127.0.0.1:${(server.address() as AddressInfo).port}`,
    requests,
    streams: () => streams.size,
    send(type: 'snapshot' | 'stats', stats: unknown) {
      const data = JSON.stringify({ type, reason: type, stats, at: new Date().toISOString() });
      for (const res of streams) res.write(`event: ${type}\ndata: ${data}\n\n`);
    },
    close: () => new Promise<void>(done => { for (const res of streams) res.destroy(); server.close(() => done()); }),
  };
}
