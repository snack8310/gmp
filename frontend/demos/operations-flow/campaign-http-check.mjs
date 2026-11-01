import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { CampaignApi, CAMPAIGN_API_BASE } from './campaign-api-client.mjs';

let created;
let actual;
let stepName;
const api = new CampaignApi(process.argv[2] || CAMPAIGN_API_BASE, async (url, options) => {
 const response = await fetch(url, options);
 actual = response.status;
 if (stepName === 'create_campaign' && response.ok) created = await response.clone().json();
 return response;
});
const step = async (name, status, request) => {
 stepName = name;
 actual = 0;
 try { return await request(); }
 finally {
  console.log(JSON.stringify({ step: name, expected: status, actual }));
  assert.equal(actual, status, name);
 }
};
const group = groupId => ({ type: 'group', groupId });
const bundle = bundleId => ({ type: 'bundle', bundleId });

const before = (await step('list_before', 200, () => api.list())).campaigns;
assert.ok((await step('actions', 200, () => api.actions())).actions.some(item => item.id === 'action-show-image' && item.requiresSlot));
assert.ok((await step('references', 200, () => api.audienceReferences())).audienceReferences.some(item => item.bundleId === 'bundle-warmup'));
await assert.rejects(step('unknown_campaign', 404, () => api.get(`missing-${randomUUID()}`)), error => error.status === 404 && error.code === 'not_found');
await assert.rejects(step('entry_conflict', 409, () => api.deleteTreatment('campaign-branch', 'treatment-rcs')), error => error.status === 409 && error.code === 'entry_conflict');

try {
 created = await step('create_campaign', 201, () => api.create({ name: 'HTTP 集成验收' }));
 assert.ok(created.id);
 assert.ok((await step('list_after_create', 200, () => api.list())).campaigns.some(item => item.id === created.id));
 created = await step('select_one_bundle', 200, () => api.updateAudience(created.id, bundle('bundle-warmup'), null));
 const imageA = await step('create_arm_a', 201, () => api.createTreatment(created.id, { name: 'A 展示', actionId: 'action-show-image', slotId: 'slot-home-banner' }));
 const imageB = await step('create_arm_b', 201, () => api.createTreatment(created.id, { name: 'B 展示', actionId: 'action-show-image', slotId: 'slot-home-banner' }));
 const flow = { id: `flow-${created.id}`, entries: [
  { groupId: 'group-arm-a', kind: 'start', treatmentIds: [imageA.id] },
  { groupId: 'group-arm-b', kind: 'start', treatmentIds: [imageB.id] },
  { groupId: 'group-arm-control', kind: 'control', treatmentIds: [] },
 ], edges: [] };
 created = await step('save_experiment_routes', 200, () => api.updateFlow(created.id, flow, [], []));
 assert.equal(created.audience.bundleId, 'bundle-warmup');
 assert.equal(created.activityFlow.entries.length, 3);
 await assert.rejects(step('slot_conflict', 409, () => api.createTreatment(created.id, { name: '冲突展示', actionId: 'action-show-image', slotId: 'slot-home-banner' })), error => error.status === 409 && error.code === 'slot_conflict');
 const detail = await step('get_campaign', 200, () => api.get(created.id));
 assert.equal(detail.treatments.length, 2);
 const renamed = await step('update_campaign', 200, () => api.update(created.id, { ...detail, name: 'HTTP 集成已改名' }));
 assert.equal(renamed.name, 'HTTP 集成已改名');
 assert.equal(renamed.treatments.every(item => !Object.hasOwn(item, 'audienceRef')), true);
 assert.ok((await step('arm_usage', 200, () => api.usages({ type: 'arm', bundleId: 'bundle-warmup', groupId: 'group-arm-a' }))).usages.some(item => item.campaignId === created.id && item.role === 'experiment-audience'));
 console.log(JSON.stringify({ step: 'campaign_one_audience_and_arm_routes', result: 'ok' }));
} finally {
 if (created?.id) {
  await step('delete_campaign', 204, () => api.delete(created.id));
  assert.ok(!(await step('list_after_delete', 200, () => api.list())).campaigns.some(item => item.id === created.id));
  assert.equal((await step('list_count_after_delete', 200, () => api.list())).campaigns.length, before.length);
 }
}
