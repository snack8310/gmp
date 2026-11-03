import test from 'node:test';
import assert from 'node:assert/strict';
import { composeFlowExperimentGraph, flowAnchorSignature, simulateFlowRuns } from './flow-experiment-graph.mjs';

const actions = [{ id: 'action-one', name: '动作一', receiptOutcomes: ['成功', '失败'] }, { id: 'action-two', name: '动作二', receiptOutcomes: [] }];
const refs = [{ type: 'group', groupId: 'group-one', groupName: '演示人群', available: true }];
const experiments = [{ id: 'exp-one', name: '演示实验', runtimeStatus: 'running', colored: true, split: { a: 50, b: 40, control: 10 } }];
const activity = { id: 'campaign-one', name: '演示活动', audience: { type: 'group', groupId: 'group-one' }, treatments: [
 { id: 'treatment-one', name: '投放一', actionId: 'action-one' }, { id: 'treatment-two', name: '投放二', actionId: 'action-two' }],
 activityFlow: { id: 'flow-one', entries: [{ groupId: 'group-one', kind: 'start', treatmentIds: ['treatment-one'] }],
 edges: [{ id: 'edge-success', fromTreatmentId: 'treatment-one', toTreatmentId: 'treatment-two', conditionLabel: '成功' }] } };
const sign = (campaign, catalog, prior, node, usedActions = actions) => ({ ...node, anchorSignature: flowAnchorSignature(composeFlowExperimentGraph(campaign, usedActions, catalog, experiments, prior).connections.find(item => item.id === node.anchorId)) });
const draft = sign(activity, refs, [], { id: 'node-one', experimentId: 'exp-one', anchorId: 'edge-success', routes: { a: 'treatment-two', b: 'flow-terminal', control: 'flow-terminal' } });

test('receipt insertion replaces the route in both graph and rehearsal', () => {
 const graph = composeFlowExperimentGraph(activity, actions, refs, experiments, [draft]);
 assert.equal(graph.canRehearse, true);
 assert.ok(graph.connections.some(item => item.id === 'experiment-in:node-one' && item.from === 'treatment-one' && item.to === 'flow-experiment:node-one'));
 assert.equal(graph.connections.some(item => item.id === 'edge-success'), false);
 const result = simulateFlowRuns(graph, [{ uid: 'SIM-0001' }, { uid: 'SIM-0002' }], () => 'a');
 assert.equal(result.visits.get('node-one').people.size, 1);
 assert.ok(result.runs.every(run => run.path.includes('flow-experiment:node-one') && run.path.includes('treatment-two') || !run.path.includes('flow-experiment:node-one')));
});

test('missing reference, broken anchor, unreachable treatment and cycles block rehearsal', () => {
 assert.match(composeFlowExperimentGraph(activity, actions, refs, [{ ...experiments[0], runtimeStatus: 'stopped' }], [draft]).readinessIssue, /停用/);
 assert.match(composeFlowExperimentGraph(activity, actions, refs, experiments, [{ ...draft, anchorId: 'removed' }]).readinessIssue, /重新连接/);
 assert.match(composeFlowExperimentGraph(activity, actions, refs, experiments, [{ ...draft, routes: { a: 'flow-terminal', b: 'flow-terminal', control: 'flow-terminal' } }]).readinessIssue, /不可达/);
 assert.match(composeFlowExperimentGraph(activity, actions, refs, experiments, [{ ...draft, routes: { a: 'treatment-one', b: 'treatment-two', control: 'flow-terminal' } }]).readinessIssue, /循环/);
});

test('a draft branch cannot route into an audience bundle entry', () => {
 const bundle = { ...activity, audience: { type: 'bundle', bundleId: 'bundle-one' }, activityFlow: { ...activity.activityFlow,
  entries: [{ groupId: 'arm-a', kind: 'start', treatmentIds: ['treatment-one'] }, { groupId: 'arm-b', kind: 'start', treatmentIds: ['treatment-two'] }, { groupId: 'arm-control', kind: 'control', treatmentIds: [] }], edges: [] } };
 const bundleRefs = ['a', 'b', 'control'].map(part => ({ type: 'arm', bundleId: 'bundle-one', groupId: `arm-${part}`, part, armName: part, available: true }));
 const first = sign(bundle, bundleRefs, [], { ...draft, anchorId: 'entry:arm-a:treatment-one',
  routes: { a: 'treatment-one', b: 'entry:arm-b', control: 'flow-terminal' } });
 const graph = composeFlowExperimentGraph(bundle, actions, bundleRefs, experiments, [first]);
 assert.equal(graph.canRehearse, false);
 assert.match(graph.readinessIssue, /去向/);
});

test('an API edge retaining its ID cannot silently move a local experiment to a changed path', () => {
 const changed = { ...activity, activityFlow: { ...activity.activityFlow,
  edges: [{ ...activity.activityFlow.edges[0], conditionLabel: '失败' }] } };
 const graph = composeFlowExperimentGraph(changed, actions, refs, experiments, [draft]);
 assert.match(graph.readinessIssue, /插入路径已变化/);
 assert.equal(graph.canRehearse, false);
});

test('empty ordinary audience has an insertion point, bundle control finish does not', () => {
 const empty = { ...activity, treatments: [], activityFlow: { id: 'flow-one', entries: [], edges: [] } };
 assert.ok(composeFlowExperimentGraph(empty, actions, refs, experiments).anchors.some(item => item.id === 'empty:entry'));
 const first = sign(empty, refs, [], { ...draft, anchorId: 'empty:entry', routes: { a: 'flow-terminal', b: 'flow-terminal', control: 'flow-terminal' } });
 assert.equal(composeFlowExperimentGraph(empty, actions, refs, experiments, [first]).canRehearse, true);
 const bundle = { ...activity, audience: { type: 'bundle', bundleId: 'bundle-one' }, treatments: [], activityFlow: { id: 'flow-one', entries: [{ groupId: 'control-group', kind: 'control', treatmentIds: [] }], edges: [] } };
 const bundleRefs = [{ type: 'arm', bundleId: 'bundle-one', groupId: 'control-group', armName: '对照组', part: 'control', available: true }];
 assert.equal(composeFlowExperimentGraph(bundle, actions, bundleRefs, experiments).anchors.some(item => item.id === 'control:control-group'), false);
});

test('an empty entry cannot hide existing treatments behind all-terminal experiment branches', () => {
 const noEntry = { ...activity, activityFlow: { id: 'flow-one', entries: [], edges: [] } };
 const draftAtEntry = sign(noEntry, refs, [], { ...draft, anchorId: 'empty:entry', routes: { a: 'flow-terminal', b: 'flow-terminal', control: 'flow-terminal' } });
 const graph = composeFlowExperimentGraph(noEntry, actions, refs, experiments, [draftAtEntry]);
 assert.match(graph.readinessIssue, /不可达/);
 assert.equal(graph.canRehearse, false);
});

test('an experiment branch can host a second node with independent visit counts', () => {
 const first = { ...draft, routes: { a: 'treatment-two', b: 'treatment-two', control: 'treatment-two' } };
 const second = sign(activity, refs, [first], { id: 'node-two', experimentId: 'exp-one', anchorId: 'experiment:node-one:a',
  routes: { a: 'treatment-two', b: 'flow-terminal', control: 'flow-terminal' } });
 const graph = composeFlowExperimentGraph(activity, actions, refs, experiments, [first, second]);
 assert.equal(graph.canRehearse, true);
 assert.ok(graph.connections.some(item => item.id === 'experiment-in:node-two' && item.from === 'flow-experiment:node-one'));
 const result = simulateFlowRuns(graph, [{ uid: 'SIM-0001' }], node => node.id === 'node-one' ? 'a' : 'control');
 assert.equal(result.visits.get('node-one').people.size, 1);
 assert.equal(result.visits.get('node-two').people.size, 1);
 assert.equal(result.visits.get('node-two').arrivals, 1);
});

test('a new node can be inserted before an existing node on an occupied entry path', () => {
 const existing = sign(activity, refs, [], { ...draft, anchorId: 'entry:group-one:treatment-one',
  routes: { a: 'treatment-one', b: 'flow-terminal', control: 'flow-terminal' } });
 const before = composeFlowExperimentGraph(activity, actions, refs, experiments, [existing]);
 assert.ok(before.anchors.some(item => item.id === 'experiment-in:node-one'));
 const upstream = sign(activity, refs, [existing], { id: 'node-before', experimentId: 'exp-one', anchorId: 'experiment-in:node-one',
  routes: { a: 'flow-experiment:node-one', b: 'flow-terminal', control: 'flow-terminal' } });
 const graph = composeFlowExperimentGraph(activity, actions, refs, experiments, [existing, upstream]);
 assert.equal(graph.canRehearse, true);
 assert.ok(graph.connections.some(item => item.id === 'experiment-in:node-before' && item.to === 'flow-experiment:node-before'));
 const result = simulateFlowRuns(graph, [{ uid: 'SIM-0001' }], () => 'a');
 assert.deepEqual(result.runs[0].path.slice(0, 3), ['audience:activity', 'flow-experiment:node-before', 'flow-experiment:node-one']);
});

test('one UID entering by multiple paths is counted once per node but each arrival remains visible', () => {
 const parallel = { ...activity, treatments: [{ id: 'treatment-one', name: '投放一', actionId: 'action-one' }],
  activityFlow: { id: 'flow-one', entries: [{ groupId: 'group-one', kind: 'start', treatmentIds: ['treatment-one'] }], edges: [] } };
 const singleOutcomeActions = [{ ...actions[0], receiptOutcomes: ['成功'] }];
 const node = sign(parallel, refs, [], { ...draft, anchorId: 'complete:treatment-one:成功', routes: { a: 'flow-terminal', b: 'flow-terminal', control: 'flow-terminal' } }, singleOutcomeActions);
 const graph = composeFlowExperimentGraph(parallel, singleOutcomeActions, refs, experiments, [node]);
 assert.equal(graph.canRehearse, true);
 const result = simulateFlowRuns(graph, [{ uid: 'SIM-0001' }, { uid: 'SIM-0001' }], () => 'a');
 assert.equal(result.visits.get('node-one').people.size, 1);
 assert.equal(result.visits.get('node-one').arrivals, 2);
});

test('an audience bundle sends each constructed UID only through its selected entry before flow experiments', () => {
 const bundle = { ...activity, audience: { type: 'bundle', bundleId: 'bundle-one' }, treatments: [
  { id: 'treatment-one', name: '投放一', actionId: 'action-one' }, { id: 'treatment-two', name: '投放二', actionId: 'action-two' }],
  activityFlow: { id: 'flow-one', entries: [
   { groupId: 'arm-a', kind: 'start', treatmentIds: ['treatment-one'] },
   { groupId: 'arm-b', kind: 'start', treatmentIds: ['treatment-two'] },
   { groupId: 'arm-control', kind: 'control', treatmentIds: [] }], edges: [] } };
 const bundleRefs = ['a', 'b', 'control'].map(part => ({ type: 'arm', bundleId: 'bundle-one', groupId: `arm-${part}`, part, armName: part, available: true }));
 const node = sign(bundle, bundleRefs, [], { ...draft, anchorId: 'entry:arm-a:treatment-one', routes: { a: 'treatment-one', b: 'flow-terminal', control: 'flow-terminal' } });
 const graph = composeFlowExperimentGraph(bundle, actions, bundleRefs, experiments, [node]);
 const users = [{ uid: 'SIM-0001' }, { uid: 'SIM-0002' }, { uid: 'SIM-0003' }];
 const assignments = { 'SIM-0001': 'a', 'SIM-0002': 'b', 'SIM-0003': 'control' };
 const result = simulateFlowRuns(graph, users, () => 'a', (groupId, uid) => groupId === `arm-${assignments[uid]}`);
 assert.equal(result.visits.get('node-one').people.size, 1);
 assert.equal(result.runs.length, 3);
});
