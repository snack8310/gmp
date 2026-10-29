import test from 'node:test';
import assert from 'node:assert/strict';
import { CampaignApi, CAMPAIGN_API_BASE, browserCampaignApiBase } from './campaign-api-client.mjs';

test('browser API base follows the page origin, respects overrides, and rejects offline requests', async () => {
 const http = { protocol: 'http:', origin: 'http://127.0.0.1:9191' };
 const https = { protocol: 'https:', origin: 'https://example.test:9443' };
 const file = { protocol: 'file:', origin: 'null' };
 assert.equal(browserCampaignApiBase(http), http.origin);
 assert.equal(browserCampaignApiBase(https), https.origin);
 assert.equal(browserCampaignApiBase(http, 'http://127.0.0.1:9292'), 'http://127.0.0.1:9292');
 assert.equal(browserCampaignApiBase(file), '');
 assert.equal(browserCampaignApiBase(file, 'http://127.0.0.1:9292'), 'http://127.0.0.1:9292');
 let requested = false;
 const api = new CampaignApi(browserCampaignApiBase(file), async () => { requested = true; });
 await assert.rejects(api.list(), /活动管理 API 不可用/);
 assert.equal(requested, false);
});

test('campaign client uses the documented routes and JSON bodies', async () => {
 assert.equal(new CampaignApi().base, CAMPAIGN_API_BASE);
 const calls = [];
 const api = new CampaignApi('http://127.0.0.1:8081', async (url, options = {}) => {
  calls.push({ path: new URL(url).pathname + new URL(url).search, method: options.method || 'GET', body: options.body && JSON.parse(options.body) });
  return { ok: true, status: options.method === 'DELETE' ? 204 : 200, json: async () => ({ campaigns: [], actions: [], audienceReferences: [], usages: [] }) };
 });
 const ref = { type: 'arm', bundleId: 'bundle-a', groupId: 'group-b' };
 await api.list(); await api.create({ name: '新活动' }); await api.get('a/b'); await api.update('a/b', { id: 'a/b', name: '新名称', treatments: [] }); await api.delete('a/b');
 await api.createTreatment('a/b', { name: '短信', actionId: 'action-sms', audienceRef: ref });
 await api.updateTreatment('a/b', 't/1', { id: 't/1', name: '短信', actionId: 'action-sms', audienceRef: ref });
 await api.deleteTreatment('a/b', 't/1'); await api.actions(); await api.audienceReferences(); await api.usages(ref);
 assert.deepEqual(calls.map(({ path, method }) => `${method} ${path}`), [
  'GET /api/v1/campaigns', 'POST /api/v1/campaigns', 'GET /api/v1/campaigns/a%2Fb', 'PUT /api/v1/campaigns/a%2Fb', 'DELETE /api/v1/campaigns/a%2Fb',
  'POST /api/v1/campaigns/a%2Fb/treatments', 'PUT /api/v1/campaigns/a%2Fb/treatments/t%2F1', 'DELETE /api/v1/campaigns/a%2Fb/treatments/t%2F1',
  'GET /api/v1/actions', 'GET /api/v1/audience-references', 'GET /api/v1/audience-reference-usages?type=arm&bundleId=bundle-a&groupId=group-b',
 ]);
 assert.deepEqual(calls[1].body, { name: '新活动' });
 assert.deepEqual(calls[5].body.audienceRef, ref);
});

test('campaign client calls the default fetch with the global receiver', async () => {
 const originalFetch = globalThis.fetch;
 globalThis.fetch = async function (url) {
  if (this !== globalThis) throw new TypeError('Illegal invocation');
  assert.equal(new URL(url).pathname, '/api/v1/campaigns');
  return { ok: true, status: 200, json: async () => ({ campaigns: [] }) };
 };
 try {
  assert.deepEqual(await new CampaignApi().list(), { campaigns: [] });
 } finally {
  globalThis.fetch = originalFetch;
 }
});

test('campaign client propagates HTTP and network failures', async () => {
 const bad = new CampaignApi('http://127.0.0.1:8081', async () => ({ ok: false, status: 422, json: async () => ({ error: { code: 'unknown_audience', message: '人群不存在' } }) }));
 await assert.rejects(bad.createTreatment('x', {}), error => error.status === 422 && error.code === 'unknown_audience' && /人群不存在/.test(error.message));
 const offline = new CampaignApi('http://127.0.0.1:8081', async () => { throw Error('connection refused'); });
 await assert.rejects(offline.list(), /无法连接活动管理 API/);
});
