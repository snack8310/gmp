import test from 'node:test';
import assert from 'node:assert/strict';
import { createFlowExperimentStore, FLOW_EXPERIMENT_KEY, parseFlowDraftJSON } from './flow-experiment-draft.mjs';
import { assignGroup, DEFAULT_SPLIT, MOCK_USERS } from './audience.mjs';

const node = { id: 'node-a', experimentId: 'exp-demo', anchorId: 'entry:group-one:treatment-one',
 routes: { a: 'treatment-one', b: 'flow-terminal', control: 'flow-terminal' } };
const experiment = split => [{ id: 'exp-demo', runtimeStatus: 'running', split, colored: true }];
const storage = () => {
 const values = new Map();
 return { values, getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
};

test('flow coloring is scoped to activity and node and preview never writes', () => {
 const memory = storage(), store = createFlowExperimentStore(memory);
 store.saveNode('campaign-one', node);
 const before = memory.values.get(FLOW_EXPERIMENT_KEY);
 const preview = store.group('campaign-one', node, 'SIM-0001', experiment(DEFAULT_SPLIT));
 assert.equal(memory.values.get(FLOW_EXPERIMENT_KEY), before);
 assert.equal(store.group('campaign-one', node, 'SIM-0001', experiment(DEFAULT_SPLIT), true), preview);
 assert.equal(store.group('campaign-one', node, 'SIM-0001', experiment({ a: 0, b: 0, control: 100 })), preview);
 const second = { ...node, id: 'node-b' };
 assert.equal(store.group('campaign-one', second, 'SIM-0001', experiment({ a: 0, b: 0, control: 100 })), 'control');
 assert.equal(store.group('campaign-two', node, 'SIM-0001', experiment({ a: 0, b: 0, control: 100 })), 'control');
 assert.deepEqual(store.coloring('campaign-one', 'node-b'), {});
 assert.deepEqual(store.coloring('campaign-two', 'node-a'), {});
});

test('damaged draft preserves original bytes and blocks writes', () => {
 const memory = storage(); memory.values.set(FLOW_EXPERIMENT_KEY, '{damaged');
 const store = createFlowExperimentStore(memory);
 assert.equal(store.canUse, false);
 assert.throws(() => store.saveNode('campaign-one', node), /损坏/);
 assert.throws(() => store.references('exp-demo'), /无法确认.*引用占用/);
 assert.equal(memory.values.get(FLOW_EXPERIMENT_KEY), '{damaged');
});

test('failed browser write warns that the draft is session only', () => {
 const memory = storage(); memory.setItem = () => { throw Error('quota'); };
 const store = createFlowExperimentStore(memory);
 store.saveNode('campaign-one', node);
 assert.match(store.warning, /当前会话/);
 assert.equal(store.nodes('campaign-one').length, 1);
});

test('changing a node definition clears its prior simulated coloring', () => {
 const memory = storage(), store = createFlowExperimentStore(memory);
 store.saveNode('campaign-one', node);
 store.group('campaign-one', node, 'SIM-0001', experiment(DEFAULT_SPLIT), true);
 assert.equal(Object.keys(store.coloring('campaign-one', node.id)).length, 1);
 store.saveNode('campaign-one', { ...node, experimentId: 'exp-other' });
 assert.deepEqual(store.coloring('campaign-one', node.id), {});
});

test('activity IDs preserve valid API characters without treating object prototype keys as data', () => {
 const store = createFlowExperimentStore(storage());
 store.saveNode('campaign / a+b', node);
 assert.equal(store.nodes('campaign / a+b').length, 1);
 assert.throws(() => store.saveNode('__proto__', node), /ID 无效/);
});

test('removing an activity releases its local experiment references', () => {
 const store = createFlowExperimentStore(storage());
 store.saveNode('campaign-one', node);
 assert.equal(store.references('exp-demo').length, 1);
 store.removeActivity('campaign-one');
 assert.deepEqual(store.references('exp-demo'), []);
});

test('scoped identities encode tuple boundaries instead of joining ambiguous IDs', () => {
 const left = MOCK_USERS.map(user => assignGroup(user.uid, 'a_b', DEFAULT_SPLIT, ['c', 'd']));
 const right = MOCK_USERS.map(user => assignGroup(user.uid, 'a', DEFAULT_SPLIT, ['b_c', 'd']));
 assert.notDeepEqual(left, right);
});

test('an older tab checks the latest persisted draft before allowing experiment deletion', () => {
 const memory = storage();
 const firstTab = createFlowExperimentStore(memory);
 const secondTab = createFlowExperimentStore(memory);
 secondTab.saveNode('campaign-one', node);
 assert.equal(firstTab.references('exp-demo').length, 1);
 assert.equal(firstTab.nodes('campaign-one').length, 1);
});

test('a failed draft removal cannot make a still-persisted reference look free', () => {
 const memory = storage(), store = createFlowExperimentStore(memory);
 store.saveNode('campaign-one', node);
 const durable = memory.values.get(FLOW_EXPERIMENT_KEY);
 memory.setItem = () => { throw Error('quota'); };
 store.removeNode('campaign-one', node.id);
 assert.equal(memory.values.get(FLOW_EXPERIMENT_KEY), durable);
 assert.throws(() => store.references('exp-demo'), /无法确认/);
});

test('duplicate JSON keys cannot hide an experiment reference during cache loading', () => {
 assert.throws(() => parseFlowDraftJSON('{"a":1,"\\u0061":2}'), /Duplicate JSON key/);
 const memory = storage();
 const record = JSON.stringify({ nodes: [node], coloring: {} });
 const raw = `{"version":1,"activities":{"campaign-one":${record},"campaign-one":{"nodes":[],"coloring":{}}}}`;
 memory.values.set(FLOW_EXPERIMENT_KEY, raw);
 const store = createFlowExperimentStore(memory);
 assert.equal(store.canUse, false);
 assert.throws(() => store.references('exp-demo'), /无法确认/);
 assert.equal(memory.values.get(FLOW_EXPERIMENT_KEY), raw);
});
