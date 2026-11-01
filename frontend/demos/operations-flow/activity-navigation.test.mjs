import test from 'node:test';
import assert from 'node:assert/strict';
import { mountActivityNavigation, renderActivityNavigation } from './activity-navigation.mjs';

test('activity navigation uses stable encoded IDs and marks the selected activity', () => {
 const html = renderActivityNavigation([
  { id: 'campaign alpha/β', name: '<活动 A>' },
  { id: 'campaign-b', name: '活动 B' },
 ], 'campaign-b');
 assert.match(html, /href="#activities\/campaign%20alpha%2F%CE%B2\/audience"/);
 assert.match(html, /href="#activities\/campaign-b\/audience" aria-current="location"/);
 assert.doesNotMatch(html, /<活动 A>/);
});

test('activity navigation uses the management API and reports a missing activity clearly', async () => {
 const values = new Map();
 const item = id => {
  if (!values.has(id)) values.set(id, {
   innerHTML: '', textContent: '', listeners: {}, setAttribute(name, value) { this.attributes ??= {}; this.attributes[name] = value; },
   addEventListener(name, listener) { this.listeners[name] = listener; },
  });
  return values.get(id);
 };
 const previous = globalThis.document;
 globalThis.document = { getElementById: item };
 try {
  const api = { list: async () => ({ campaigns: [{ id: 'one', name: '活动一' }] }) };
  const nav = mountActivityNavigation({ api });
  const selected = await nav.refresh('one');
  assert.equal(selected.selected.name, '活动一');
  assert.match(item('activity-list').innerHTML, /aria-current="location"/);
  assert.equal(item('activity-context-name').textContent, '活动一');
  const missing = await nav.refresh('missing');
  assert.equal(missing.selected, null);
  assert.doesNotMatch(item('activity-list').innerHTML, /aria-current/);
 } finally { globalThis.document = previous; }
});

test('activity navigation surfaces API errors instead of inventing a campaign', async () => {
 const values = new Map();
 const item = id => { if (!values.has(id)) values.set(id, { innerHTML: '', textContent: '' }); return values.get(id); };
 const previous = globalThis.document;
 globalThis.document = { getElementById: item };
 try {
  const nav = mountActivityNavigation({ api: { list: async () => { throw Error('API unavailable'); } } });
  const result = await nav.refresh('one');
  assert.equal(result.selected, null);
  assert.equal(item('activity-nav-status').textContent, '无法读取活动管理 API：API unavailable');
  assert.match(item('activity-list').innerHTML, /无法显示活动列表/);
 } finally { globalThis.document = previous; }
});
