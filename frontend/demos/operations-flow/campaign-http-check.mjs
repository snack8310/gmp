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
const arm = groupId => ({ type: 'arm', bundleId: 'bundle-warmup', groupId });

const before = (await step('list_before', 200, () => api.list())).campaigns;
assert.ok((await step('actions', 200, () => api.actions())).actions.some(item => item.id === 'action-show-image' && item.requiresSlot));
assert.ok((await step('references', 200, () => api.audienceReferences())).audienceReferences.some(item => item.bundleId === 'bundle-warmup'));
await assert.rejects(step('unknown_campaign', 404, () => api.get(`missing-${randomUUID()}`)), error => error.status === 404 && error.code === 'not_found');
await assert.rejects(step('edge_conflict', 409, () => api.deleteTreatment('campaign-branch', 'treatment-rcs')), error => error.status === 409 && error.code === 'edge_conflict');

try {
 created = await step('create_campaign', 201, () => api.create({ name: 'HTTP 集成验收' }));
 assert.ok(created.id);
 assert.ok((await step('list_after_create', 200, () => api.list())).campaigns.some(item => item.id === created.id));
 const added = await step('create_treatment', 201, () => api.createTreatment(created.id, { name: '短信投放项', actionId: 'action-sms', audienceRef: group('group-new') }));
 assert.ok((await step('usage_after_create', 200, () => api.usages(group('group-new')))).usages.some(item => item.campaignId === created.id && item.treatmentId === added.id));
 await step('update_treatment', 200, () => api.updateTreatment(created.id, added.id, { ...added, audienceRef: group('group-recall') }));
 assert.ok(!(await step('old_usage_after_update', 200, () => api.usages(group('group-new')))).usages.some(item => item.campaignId === created.id));
 assert.ok((await step('new_usage_after_update', 200, () => api.usages(group('group-recall')))).usages.some(item => item.campaignId === created.id));
 await assert.rejects(step('missing_slot', 422, () => api.createTreatment(created.id, { name: '无展示位', actionId: 'action-show-image', audienceRef: group('group-new') })), error => error.status === 422 && error.code === 'missing_slot');
 await assert.rejects(step('unexpected_slot', 422, () => api.createTreatment(created.id, { name: '带位推送', actionId: 'action-sms', audienceRef: group('group-new'), slotId: 'slot-home-banner' })), error => error.status === 422 && error.code === 'unexpected_slot');
 const imageA = await step('create_arm_a', 201, () => api.createTreatment(created.id, { name: 'A 展示', actionId: 'action-show-image', audienceRef: arm('group-arm-a'), slotId: 'slot-home-banner' }));
 const imageB = await step('create_arm_b', 201, () => api.createTreatment(created.id, { name: 'B 展示', actionId: 'action-show-image', audienceRef: arm('group-arm-b'), slotId: 'slot-home-banner' }));
 await assert.rejects(step('slot_conflict', 409, () => api.createTreatment(created.id, { name: '冲突展示', actionId: 'action-show-image', audienceRef: group('group-new'), slotId: 'slot-home-banner' })), error => error.status === 409 && error.code === 'slot_conflict');
 const detail = await step('get_campaign', 200, () => api.get(created.id));
 assert.equal(detail.treatments.length, 3);
 const renamed = await step('update_campaign', 200, () => api.update(created.id, { ...detail, name: 'HTTP 集成已改名' }));
 assert.equal(renamed.name, 'HTTP 集成已改名');
 await step('delete_treatment', 204, () => api.deleteTreatment(created.id, added.id));
 await step('delete_arm_a', 204, () => api.deleteTreatment(created.id, imageA.id));
 await step('delete_arm_b', 204, () => api.deleteTreatment(created.id, imageB.id));
 assert.ok(!(await step('usage_after_delete', 200, () => api.usages(group('group-recall')))).usages.some(item => item.campaignId === created.id));
 console.log(JSON.stringify({ step: 'campaign_crud_failure_and_usages', result: 'ok' }));
} finally {
 if (created?.id) {
  await step('delete_campaign', 204, () => api.delete(created.id));
  assert.ok(!(await step('list_after_delete', 200, () => api.list())).campaigns.some(item => item.id === created.id));
  assert.equal((await step('list_count_after_delete', 200, () => api.list())).campaigns.length, before.length);
 }
}
