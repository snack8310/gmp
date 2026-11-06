import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { createServer } from 'node:http';
import { createServer as createNetServer } from 'node:net';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const servePath = fileURLToPath(new URL('./serve.mjs', import.meta.url));

async function availablePort(host) {
 const server = createNetServer();
 server.listen(0, host);
 await once(server, 'listening');
 const { port } = server.address();
 server.close();
 await once(server, 'close');
 return port;
}

async function upstream(host) {
 const server = createServer((request, response) => {
  assert.equal(request.url, '/api/v1/campaigns');
  response.writeHead(200, { 'content-type': 'application/json' });
  response.end(JSON.stringify({ campaigns: [{ id: 'configured-upstream' }] }));
 });
 server.listen(0, host);
 await once(server, 'listening');
 return server;
}

async function startFrontend(port, apiPort, overrides = {}) {
 const env = { ...process.env };
 delete env.GMP_DEMO_LISTEN_HOST;
 delete env.GMP_DEMO_API_HOST;
 const child = spawn(process.execPath, [servePath, String(port), String(apiPort)], {
  env: { ...env, ...overrides }, stdio: ['ignore', 'pipe', 'pipe'],
 });
 let output = '';
 try {
  await new Promise((resolve, reject) => {
   const timeout = setTimeout(() => reject(Error(`frontend did not listen: ${output}`)), 4000);
   child.stdout.on('data', chunk => {
    output += chunk;
    if (output.includes('"event":"listening"')) { clearTimeout(timeout); resolve(); }
   });
   child.once('exit', code => { clearTimeout(timeout); reject(Error(`frontend exited ${code}: ${output}`)); });
  });
  return child;
 } catch (error) {
  await stop(child);
  throw error;
 }
}

async function stop(child) {
 if (child.exitCode !== null) return;
 child.kill();
 await once(child, 'exit');
}

test('explicit container hosts serve the page and proxy the API', async () => {
 const api = await upstream('::1');
 const port = await availablePort('::1');
 let child;
 try {
  child = await startFrontend(port, api.address().port, {
   GMP_DEMO_LISTEN_HOST: '::1', GMP_DEMO_API_HOST: '::1',
  });
  const page = await fetch(`http://[::1]:${port}/`);
  assert.equal(page.status, 200);
  assert.match(await page.text(), /<html/);
  const response = await fetch(`http://[::1]:${port}/api/v1/campaigns`);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { campaigns: [{ id: 'configured-upstream' }] });
 } finally {
  if (child) await stop(child);
  api.close();
  await once(api, 'close');
 }
});

test('empty API host fails before the frontend begins listening', async () => {
 const port = await availablePort('127.0.0.1');
 const child = spawn(process.execPath, [servePath, String(port), '8082'], {
  env: { ...process.env, GMP_DEMO_API_HOST: '' }, stdio: 'ignore',
 });
 try {
  const outcome = await Promise.race([
   once(child, 'exit').then(([code]) => code),
   new Promise(resolve => setTimeout(() => resolve('still running'), 1000)),
  ]);
  assert.equal(outcome, 1);
 } finally {
  await stop(child);
 }
});

test('default local hosts still proxy the API over loopback', async () => {
 const api = await upstream('127.0.0.1');
 const port = await availablePort('127.0.0.1');
 let child;
 try {
  child = await startFrontend(port, api.address().port);
  const response = await fetch(`http://127.0.0.1:${port}/api/v1/campaigns`);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { campaigns: [{ id: 'configured-upstream' }] });
 } finally {
  if (child) await stop(child);
  api.close();
  await once(api, 'close');
 }
});
