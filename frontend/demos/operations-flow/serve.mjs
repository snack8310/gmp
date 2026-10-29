import { createServer, request } from 'node:http';
import { readFile } from 'node:fs/promises';

const listenPort = Number(process.argv[2] || 8081);
const apiPort = Number(process.argv[3] || 8082);
if (!Number.isInteger(listenPort) || !Number.isInteger(apiPort) || listenPort < 1 || apiPort < 1 || listenPort === apiPort) throw Error('Pass distinct listen and API ports');
const files = new Set(['index.html', 'style.css', 'app.mjs', 'audience.mjs', 'audience-ui.mjs', 'experiment-ui.mjs', 'model.mjs', 'campaign-activities.mjs', 'campaign-ui.mjs', 'campaign-api-client.mjs', 'campaign-controller.mjs']);

const server = createServer(async (incoming, outgoing) => {
 const started = Date.now();
 let status = 500;
 try {
  const pathname = new URL(incoming.url, 'http://localhost').pathname;
  if (pathname.startsWith('/api/v1/')) {
   await new Promise(resolve => {
    const upstream = request({ hostname: '127.0.0.1', port: apiPort, path: incoming.url, method: incoming.method, headers: { 'content-type': incoming.headers['content-type'] || 'application/json' } }, response => {
     status = response.statusCode;
     outgoing.writeHead(status, { 'content-type': response.headers['content-type'] || 'application/json' });
     response.pipe(outgoing); response.on('end', resolve);
    });
    upstream.on('error', () => { status = 502; outgoing.writeHead(status, { 'content-type': 'application/json' }); outgoing.end(JSON.stringify({ error: { code: 'api_unavailable', message: '活动管理 API 不可用' } })); resolve(); });
    incoming.pipe(upstream);
   });
  } else {
   const name = pathname === '/' ? 'index.html' : decodeURIComponent(pathname.slice(1));
   if (incoming.method !== 'GET' || !files.has(name)) { status = 404; outgoing.writeHead(status); outgoing.end(); }
   else {
    const content = await readFile(new URL(name, import.meta.url));
    status = 200;
    outgoing.writeHead(status, { 'content-type': name.endsWith('.mjs') ? 'text/javascript; charset=utf-8' : name.endsWith('.css') ? 'text/css; charset=utf-8' : 'text/html; charset=utf-8' });
    outgoing.end(content);
   }
  }
 } catch { status = 500; outgoing.writeHead(status); outgoing.end(); }
 console.info(JSON.stringify({ component: 'campaign-demo-server', method: incoming.method, status, duration_ms: Date.now() - started }));
});
server.on('error', error => { console.error(JSON.stringify({ component: 'campaign-demo-server', event: 'listen_failed', code: error.code })); process.exitCode = 1; });
server.listen(listenPort, '127.0.0.1', () => console.info(JSON.stringify({ component: 'campaign-demo-server', event: 'listening', port: listenPort, api_port: apiPort })));
