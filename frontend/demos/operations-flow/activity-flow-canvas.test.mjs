import test from 'node:test';
import assert from 'node:assert/strict';
import { ACTIVITIES } from './campaign-activities.mjs';
import { buildActivityGraph, buildEntryRoutes } from './activity-flow-canvas.mjs';

const actions = [
 { id: 'action-rcs', name: 'RCS 消息', receiptOutcomes: ['成功', '失败'] },
 { id: 'action-email', name: '邮件', receiptOutcomes: ['成功', '失败'] },
 { id: 'action-sms', name: '短信', receiptOutcomes: ['成功', '失败'] },
];
const references = [
 { type: 'group', groupId: 'group-recall', name: '跨境召回人群组', groupName: '跨境召回人群组', available: true },
 { type: 'group', groupId: 'group-new', name: '新客人群组', groupName: '新客人群组', available: true },
 { type: 'arm', bundleId: 'bundle-warmup', groupId: 'group-arm-a', bundleName: '预热实验包', name: 'A 臂', armName: 'A 臂', part: 'a', available: true },
 { type: 'arm', bundleId: 'bundle-warmup', groupId: 'group-arm-b', bundleName: '预热实验包', name: 'B 臂', armName: 'B 臂', part: 'b', available: true },
 { type: 'arm', bundleId: 'bundle-warmup', groupId: 'group-arm-control', bundleName: '预热实验包', name: '对照臂', armName: '对照臂', part: 'control', available: true },
];
const fromApi = activity => ({
 ...activity,
 treatments: activity.treatments.map(({ actionRef, ...treatment }) => ({ ...treatment, actionId: actionRef.id })),
});

test('the graph shows one activity audience, its entry route and receipt edges', () => {
 const graph = buildActivityGraph(fromApi(ACTIVITIES[0]), actions, references);
 assert.deepEqual(graph.roots.map(item => item.id), ['treatment-rcs']);
 assert.equal(graph.nodes.filter(item => item.kind === 'audience').length, 1);
 assert.equal(graph.audienceNode.name, '跨境召回人群组');
 assert.deepEqual(graph.connections.filter(item => item.kind === 'entry').map(item => [item.from, item.to]), [['audience:activity', 'treatment-rcs']]);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'receipt').map(item => [item.id, item.from, item.to, item.label]), [
  ['edge-success', 'treatment-rcs', 'treatment-email', '成功'],
  ['edge-failed', 'treatment-rcs', 'treatment-sms', '失败'],
 ]);
 assert.ok(graph.nodes.some(item => item.id === 'treatment-email' && item.rules.some(([label, value]) => label === '入口配置' && value === '回执后续步骤')));
 assert.equal(graph.hasCycle, false);
});

test('one selected experiment package branches by arm before starting treatments', () => {
 const graph = buildActivityGraph(fromApi(ACTIVITIES.find(activity => activity.id === 'campaign-no-edges')), actions, references);
 assert.equal(graph.audienceNode.name, '预热实验包');
 assert.deepEqual(graph.roots.map(item => item.id), ['treatment-a', 'treatment-b']);
 assert.equal(graph.nodes.filter(item => item.kind === 'audience').length, 1);
 assert.equal(graph.nodes.filter(item => item.kind === 'entry').length, 3);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'entry').map(item => item.to), ['treatment-a', 'treatment-b']);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'split').map(item => item.to), ['entry:group-arm-a', 'entry:group-arm-b', 'entry:group-arm-control']);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'control').map(item => item.to), ['flow-terminal']);
 assert.equal(graph.canRehearse, true);
});

test('an experiment draft missing an arm route cannot start local rehearsal', () => {
 const activity = fromApi(ACTIVITIES.find(item => item.id === 'campaign-no-edges'));
 activity.activityFlow = { ...activity.activityFlow, entries: buildEntryRoutes(references.filter(item => item.type === 'arm' && item.bundleId === 'bundle-warmup'), new Map([
  ['group-arm-a', ['treatment-a']],
 ])) };
 assert.deepEqual(activity.activityFlow.entries, [
  { groupId: 'group-arm-a', kind: 'start', treatmentIds: ['treatment-a'] },
  { groupId: 'group-arm-b', kind: 'start', treatmentIds: [] },
  { groupId: 'group-arm-control', kind: 'control', treatmentIds: [] },
 ]);
 const graph = buildActivityGraph(activity, actions, references);
 assert.deepEqual(graph.roots.map(item => item.id), ['treatment-a']);
 assert.equal(graph.hasCycle, false);
 assert.equal(graph.canRehearse, false);
});

test('each selected group or experiment arm can route to multiple parallel start treatments', () => {
 const routes = buildEntryRoutes([
  { type: 'arm', groupId: 'group-arm-a', part: 'a' },
  { type: 'arm', groupId: 'group-arm-b', part: 'b' },
  { type: 'arm', groupId: 'group-arm-control', part: 'control' },
 ], new Map([
  ['group-arm-a', ['treatment-a1', 'treatment-a2']],
  ['group-arm-b', ['treatment-b1']],
 ]));
 assert.deepEqual(routes, [
  { groupId: 'group-arm-a', kind: 'start', treatmentIds: ['treatment-a1', 'treatment-a2'] },
  { groupId: 'group-arm-b', kind: 'start', treatmentIds: ['treatment-b1'] },
  { groupId: 'group-arm-control', kind: 'control', treatmentIds: [] },
 ]);
});

test('the graph flags a cycle and keeps empty activities free of invented nodes', () => {
 const cyclic = fromApi(ACTIVITIES[0]);
 cyclic.activityFlow.edges.push({ id: 'edge-cycle', fromTreatmentId: 'treatment-email', toTreatmentId: 'treatment-rcs', conditionLabel: '失败' });
 assert.equal(buildActivityGraph(cyclic, actions, references).hasCycle, true);
 const empty = buildActivityGraph(fromApi(ACTIVITIES.find(activity => activity.id === 'campaign-empty')), actions, references);
 assert.equal(empty.empty, true);
 assert.deepEqual(empty.nodes, []);
 assert.deepEqual(empty.connections, []);
});

test('an activity audience node can appear before treatments are assigned', () => {
 const empty = { ...fromApi(ACTIVITIES.find(activity => activity.id === 'campaign-empty')), audience: { type: 'group', groupId: 'group-recall' } };
 const graph = buildActivityGraph(empty, actions, references);
 assert.deepEqual(graph.nodes.map(item => [item.kind, item.id]), [['audience', 'audience:activity']]);
 assert.deepEqual(graph.connections, []);
 assert.equal(graph.empty, true);
});

test('missing action receipt contracts do not invent success or failure branches', () => {
 const activity = fromApi(ACTIVITIES.find(item => item.id === 'campaign-single'));
 const graph = buildActivityGraph(activity, [{ id: activity.treatments[0].actionId, name: '未声明回执动作' }], references);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'complete').map(item => item.label), ['动作结束']);
});
