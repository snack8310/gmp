import test from 'node:test';
import assert from 'node:assert/strict';
import { buildActivityGraph, mountActivityFlowCanvas } from './activity-flow-canvas.mjs';
import { createFlowExperimentStore, FLOW_EXPERIMENT_KEY } from './flow-experiment-draft.mjs';
import { composeFlowExperimentGraph, flowAnchorSignature } from './flow-experiment-graph.mjs';
import { createAudienceStore } from './audience.mjs';
import { mountFlowExperiments } from './flow-experiment-ui.mjs';

const activity = { id: 'campaign-demo', name: '演示活动', audience: { type: 'group', groupId: 'group-demo' },
 treatments: [{ id: 'treatment-demo', name: '欢迎短信', actionId: 'action-demo' }],
 activityFlow: { id: 'flow-demo', entries: [{ groupId: 'group-demo', kind: 'start', treatmentIds: ['treatment-demo'] }], edges: [] } };
const actions = [{ id: 'action-demo', name: '短信', receiptOutcomes: ['成功'] }];
const references = [{ type: 'group', groupId: 'group-demo', groupName: '演示人群', available: true }];
const experiment = { id: 'exp-demo', name: '演示实验', runtimeStatus: 'running', colored: true, split: { a: 45, b: 45, control: 10 } };
const anchor = buildActivityGraph(activity, actions, references).connections.find(item => item.kind === 'entry');
const node = { id: 'node-demo', experimentId: 'exp-demo', anchorId: anchor.id, anchorSignature: flowAnchorSignature(anchor),
 routes: { a: 'treatment-demo', b: 'flow-terminal', control: 'flow-terminal' } };

function fakeRoot() {
 const elements = new Map();
 const element = selector => {
  if (!elements.has(selector)) elements.set(selector, {
   innerHTML: '', textContent: '', value: '', style: {}, dataset: {}, listeners: {}, classList: { add() {}, remove() {}, toggle() {} },
   addEventListener(event, listener) { this.listeners[event] = listener; },
   querySelector: element, querySelectorAll: () => [], setAttribute() {}, removeAttribute() {},
  });
  return elements.get(selector);
 };
 const root = { innerHTML: '', querySelector: element, querySelectorAll: () => [] };
 return { root, element };
}

test('node inspection is read only and offers routes, preview, edit and remove', () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const store = createFlowExperimentStore(storage); store.saveNode(activity.id, node);
 const before = values.get(FLOW_EXPERIMENT_KEY), { root, element } = fakeRoot(), actionsTaken = [];
 globalThis.matchMedia = () => ({ matches: true });
 mountActivityFlowCanvas({ root, activity, actions, references, api: {}, draftStore: store,
  experimentStore: { experiments: () => [experiment], bundles: () => [] },
  onExperimentAction: (...args) => actionsTaken.push(args) });
 element('[data-node-list]').listeners.click({ target: { closest: () => ({ dataset: { selectNode: 'flow-experiment:node-demo' } }) } });
 const detail = element('[data-edge-detail]').innerHTML;
 assert.match(detail, /A 组 → 欢迎短信/);
 assert.match(detail, /构造样本只读预测：SIM-0001/);
 assert.match(detail, /编辑此实验节点/);
 assert.match(detail, /移除此实验节点/);
 assert.equal(values.get(FLOW_EXPERIMENT_KEY), before);
 element('[data-edge-detail]').listeners.click({ target: { closest: selector => selector === '[data-edit-experiment]' ? { dataset: { editExperiment: 'node-demo' } } : null } });
 assert.deepEqual(actionsTaken, [['edit', 'node-demo']]);
});

test('list and node inspection show the inserted experiment as the actual first hop', () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const store = createFlowExperimentStore(storage);
 const secondExperiment = { ...experiment, id: 'exp-second', name: '第二实验' };
 const definitions = [experiment, secondExperiment];
 const firstGraph = composeFlowExperimentGraph(activity, actions, references, definitions, [node]);
 const branch = firstGraph.connections.find(item => item.id === 'experiment:node-demo:a');
 const second = { id: 'node-second', experimentId: secondExperiment.id, anchorId: branch.id, anchorSignature: flowAnchorSignature(branch),
  routes: { a: 'treatment-demo', b: 'flow-terminal', control: 'flow-terminal' } };
 store.saveNode(activity.id, node); store.saveNode(activity.id, second);
 const experiments = { experiments: () => definitions, bundles: () => [] };
 const listRoot = { innerHTML: '', addEventListener() {}, removeEventListener() {} };
 mountFlowExperiments({ root: listRoot, activity, actions, references, experimentStore: experiments, draftStore: store, onChange() {} });
 assert.match(listRoot.innerHTML, /A 组 → 第二实验/);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 mountActivityFlowCanvas({ root, activity, actions, references, api: {}, draftStore: store, experimentStore: experiments });
 element('[data-node-list]').listeners.click({ target: { closest: () => ({ dataset: { selectNode: 'flow-experiment:node-demo' } }) } });
 assert.match(element('[data-edge-detail]').innerHTML, /A 组 → 第二实验/);
});

test('a coloring write failure is shown immediately during playback', async () => {
 const values = new Map(); let fail = false;
 const storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => { if (fail) throw Error('quota'); values.set(key, value); } };
 const store = createFlowExperimentStore(storage); store.saveNode(activity.id, node);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 mountActivityFlowCanvas({ root, activity, actions, references, api: { get: async () => activity }, draftStore: store,
  experimentStore: { experiments: () => [experiment], bundles: () => [] } });
 fail = true;
 await element('[data-flow-play]').listeners.click();
 assert.match(element('[data-flow-note]').textContent, /浏览器写入失败/);
 assert.match(element('.flow-save-status').textContent, /刷新后可能丢失/);
});

test('playback refuses an activity flow changed in the management API', async () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const draftStore = createFlowExperimentStore(storage); draftStore.saveNode(activity.id, node);
 let serverActivity = activity, reads = 0;
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 mountActivityFlowCanvas({ root, activity, actions, references,
  api: { async get() { reads++; return serverActivity; } }, draftStore,
  experimentStore: { experiments: () => [experiment], bundles: () => [], catalog: () => references } });
 serverActivity = { ...activity, treatments: [], activityFlow: { ...activity.activityFlow, entries: [] } };
 await element('[data-flow-play]').listeners.click();
 assert.equal(reads, 1);
 assert.match(element('[data-flow-note]').textContent, /流程已变化|重新连接/);
 assert.equal(element('[data-play-state]').textContent, '已暂停');
});

test('playback refuses a draft changed before the canvas refreshes', async () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const first = createFlowExperimentStore(storage), second = createFlowExperimentStore(storage);
 first.saveNode(activity.id, node);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 mountActivityFlowCanvas({ root, activity, actions, references, api: { get: async () => activity }, draftStore: first,
  experimentStore: { experiments: () => [experiment], bundles: () => [], catalog: () => references } });
 second.saveNode(activity.id, { ...node, routes: { ...node.routes, a: 'flow-terminal' } });
 await element('[data-flow-play]').listeners.click();
 assert.match(element('[data-flow-note]').textContent, /草稿已变化|刷新页面/);
 assert.equal(element('[data-play-state]').textContent, '已暂停');
});

test('a failed API read after canvas unmount does not write to removed elements', async () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const draftStore = createFlowExperimentStore(storage); draftStore.saveNode(activity.id, node);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 let rejectRead;
 const pendingRead = new Promise((resolve, reject) => { rejectRead = reject; });
 const unmount = mountActivityFlowCanvas({ root, activity, actions, references, api: { get: () => pendingRead }, draftStore,
  experimentStore: { experiments: () => [experiment], bundles: () => [], catalog: () => references } });
 const playing = element('[data-flow-play]').listeners.click();
 unmount();
 root.querySelector = () => null;
 rejectRead(new Error('API unavailable'));
 await assert.doesNotReject(playing);
});

test('playback refuses a definition stopped in another tab', async () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const first = createAudienceStore(storage);
 first.saveExperiment(experiment);
 first.setExperimentStatus(experiment.id, 'running');
 const second = createAudienceStore(storage);
 const draftStore = createFlowExperimentStore(storage); draftStore.saveNode(activity.id, node);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 mountActivityFlowCanvas({ root, activity, actions, references, api: { get: async () => activity }, draftStore, experimentStore: first });
 second.setExperimentStatus(experiment.id, 'stopped');
 await element('[data-flow-play]').listeners.click();
 assert.match(element('[data-flow-note]').textContent, /停用|不可用/);
 assert.equal(element('[data-play-state]').textContent, '已暂停');
});

test('playback refuses a bundle removed in another tab', async () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const first = createAudienceStore(storage), second = createAudienceStore(storage);
 const bundleActivity = { ...activity, audience: { type: 'bundle', bundleId: 'bundle-warmup' },
  treatments: [...activity.treatments, { id: 'treatment-b', name: '另一短信', actionId: 'action-demo' }],
  activityFlow: { ...activity.activityFlow, entries: [
   { groupId: 'group-arm-a', kind: 'start', treatmentIds: ['treatment-demo'] },
   { groupId: 'group-arm-b', kind: 'start', treatmentIds: ['treatment-b'] },
   { groupId: 'group-arm-control', kind: 'control', treatmentIds: [] } ] } };
 const bundleRefs = first.catalog().filter(item => item.bundleId === 'bundle-warmup');
 const entry = buildActivityGraph(bundleActivity, actions, bundleRefs).connections.find(item => item.id === 'entry:group-arm-a:treatment-demo');
 const draft = { ...node, experimentId: 'exp-recall', anchorId: entry.id, anchorSignature: flowAnchorSignature(entry) };
 const draftStore = createFlowExperimentStore(storage); draftStore.saveNode(bundleActivity.id, draft);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 mountActivityFlowCanvas({ root, activity: bundleActivity, actions, references: bundleRefs, api: { get: async () => bundleActivity }, draftStore, experimentStore: first });
 second.deleteBundle('bundle-warmup');
 await element('[data-flow-play]').listeners.click();
 assert.match(element('[data-flow-note]').textContent, /人群包.*不可用|已删除/);
 assert.equal(element('[data-play-state]').textContent, '已暂停');
});

test('playback refuses a bundle invalidated by a later experiment generation', async () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const first = createAudienceStore(storage), second = createAudienceStore(storage);
 const bundleActivity = { ...activity, audience: { type: 'bundle', bundleId: 'bundle-warmup' },
  treatments: [...activity.treatments, { id: 'treatment-b', name: '另一短信', actionId: 'action-demo' }],
  activityFlow: { ...activity.activityFlow, entries: [
   { groupId: 'group-arm-a', kind: 'start', treatmentIds: ['treatment-demo'] },
   { groupId: 'group-arm-b', kind: 'start', treatmentIds: ['treatment-b'] },
   { groupId: 'group-arm-control', kind: 'control', treatmentIds: [] } ] } };
 const bundleRefs = first.catalog().filter(item => item.bundleId === 'bundle-warmup');
 const entry = buildActivityGraph(bundleActivity, actions, bundleRefs).connections.find(item => item.id === 'entry:group-arm-a:treatment-demo');
 const draft = { ...node, experimentId: 'exp-recall', anchorId: entry.id, anchorSignature: flowAnchorSignature(entry) };
 const draftStore = createFlowExperimentStore(storage); draftStore.saveNode(bundleActivity.id, draft);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 mountActivityFlowCanvas({ root, activity: bundleActivity, actions, references: bundleRefs, api: { get: async () => bundleActivity }, draftStore, experimentStore: first });
 second.setExperimentStatus('exp-recall', 'stopped');
 second.setExperimentStatus('exp-recall', 'running');
 await element('[data-flow-play]').listeners.click();
 assert.match(element('[data-flow-note]').textContent, /不可用|失效/);
 assert.equal(element('[data-play-state]').textContent, '已暂停');
});

test('playback refuses an ordinary audience removed in another tab', async () => {
 const values = new Map(), storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
 const first = createAudienceStore(storage), second = createAudienceStore(storage);
 const groupActivity = { ...activity, audience: { type: 'group', groupId: 'group-aud-app' },
  activityFlow: { ...activity.activityFlow, entries: [{ groupId: 'group-aud-app', kind: 'start', treatmentIds: ['treatment-demo'] }] } };
 const groupRefs = first.catalog().filter(item => item.groupId === 'group-aud-app');
 const entry = buildActivityGraph(groupActivity, actions, groupRefs).connections.find(item => item.kind === 'entry');
 const draft = { ...node, experimentId: 'exp-recall', anchorId: entry.id, anchorSignature: flowAnchorSignature(entry) };
 const draftStore = createFlowExperimentStore(storage); draftStore.saveNode(groupActivity.id, draft);
 const { root, element } = fakeRoot();
 globalThis.matchMedia = () => ({ matches: true });
 globalThis.requestAnimationFrame = () => 1;
 globalThis.cancelAnimationFrame = () => {};
 mountActivityFlowCanvas({ root, activity: groupActivity, actions, references: groupRefs, api: { get: async () => groupActivity }, draftStore, experimentStore: first });
 second.delete('aud-app');
 await element('[data-flow-play]').listeners.click();
 assert.match(element('[data-flow-note]').textContent, /受众不可用/);
 assert.equal(element('[data-play-state]').textContent, '已暂停');
});
