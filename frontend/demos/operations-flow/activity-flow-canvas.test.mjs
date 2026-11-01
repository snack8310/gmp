import test from 'node:test';
import assert from 'node:assert/strict';
import { ACTIVITIES } from './campaign-activities.mjs';
import { buildActivityGraph } from './activity-flow-canvas.mjs';

const actions = [
 { id: 'action-rcs', name: 'RCS 消息', receiptOutcomes: ['成功', '失败'] },
 { id: 'action-email', name: '邮件', receiptOutcomes: ['成功', '失败'] },
 { id: 'action-sms', name: '短信', receiptOutcomes: ['成功', '失败'] },
];
const references = [
 { type: 'group', groupId: 'group-recall', name: '跨境召回人群组', available: true },
 { type: 'arm', bundleId: 'bundle-warmup', groupId: 'group-arm-a', bundleName: '预热实验包', name: 'A 臂', available: true },
 { type: 'arm', bundleId: 'bundle-warmup', groupId: 'group-arm-b', bundleName: '预热实验包', name: 'B 臂', available: true },
];
const fromApi = activity => ({
 ...activity,
 treatments: activity.treatments.map(({ actionRef, audienceRef, ...treatment }) => ({ ...treatment, actionId: actionRef.id,
  audienceRef: audienceRef.type === 'arm' ? { type: 'arm', bundleId: audienceRef.bundleId, groupId: audienceRef.groupId } : { type: 'group', groupId: audienceRef.groupId } })),
});

test('the graph uses the current activity treatments, audience references and receipt edges', () => {
 const graph = buildActivityGraph(fromApi(ACTIVITIES[0]), actions, references);
 assert.deepEqual(graph.roots.map(item => item.id), ['treatment-rcs']);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'entry').map(item => [item.from, item.to]), [['audience:group:group-recall', 'treatment-rcs']]);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'receipt').map(item => [item.id, item.from, item.to, item.label]), [
  ['edge-success', 'treatment-rcs', 'treatment-email', '成功'],
  ['edge-failed', 'treatment-rcs', 'treatment-sms', '失败'],
 ]);
 assert.ok(graph.nodes.some(item => item.id === 'treatment-email' && item.rules.some(([label, value]) => label === '引用人群' && value.includes('A 臂'))));
 assert.equal(graph.hasCycle, false);
});

test('no-edge treatments remain separate entries and each shared audience gets one source node', () => {
 const activity = fromApi(ACTIVITIES.find(item => item.id === 'campaign-no-edges'));
 activity.treatments[1].audienceRef = structuredClone(activity.treatments[0].audienceRef);
 const graph = buildActivityGraph(activity, actions, references);
 assert.deepEqual(graph.roots.map(item => item.id), ['treatment-a', 'treatment-b']);
 assert.equal(graph.nodes.filter(item => item.kind === 'audience').length, 1);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'entry').map(item => item.to), ['treatment-a', 'treatment-b']);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'complete').map(item => item.from), ['treatment-a', 'treatment-a', 'treatment-b', 'treatment-b']);
});

test('the graph flags a cycle and keeps empty activities free of invented nodes', () => {
 const cyclic = fromApi(ACTIVITIES[0]);
 cyclic.activityFlow.edges.push({ id: 'edge-cycle', fromTreatmentId: 'treatment-email', toTreatmentId: 'treatment-rcs', conditionLabel: '失败' });
 assert.equal(buildActivityGraph(cyclic, actions, references).hasCycle, true);
 const empty = buildActivityGraph(fromApi(ACTIVITIES.find(item => item.id === 'campaign-empty')), actions, references);
 assert.equal(empty.empty, true);
 assert.deepEqual(empty.nodes, []);
 assert.deepEqual(empty.connections, []);
});

test('an activity can place a shared audience entry before creating treatments', () => {
 const empty = fromApi(ACTIVITIES.find(item => item.id === 'campaign-empty'));
 empty.audienceRefs = [{ type: 'group', groupId: 'group-recall' }];
 const graph = buildActivityGraph(empty, actions, references);
 assert.deepEqual(graph.nodes.map(item => [item.kind, item.id]), [['audience', 'audience:group:group-recall']]);
 assert.deepEqual(graph.connections, []);
 assert.equal(graph.empty, true);
});

test('missing action receipt contracts do not invent success or failure branches', () => {
 const activity = fromApi(ACTIVITIES.find(item => item.id === 'campaign-single'));
 const graph = buildActivityGraph(activity, [{ id: activity.treatments[0].actionId, name: '未声明回执动作' }], references);
 assert.deepEqual(graph.connections.filter(item => item.kind === 'complete').map(item => item.label), ['动作结束']);
});
