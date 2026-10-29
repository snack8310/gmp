import assert from 'node:assert/strict';
import { CampaignApi, CAMPAIGN_API_BASE } from './campaign-api-client.mjs';

const api = new CampaignApi(process.argv[2] || CAMPAIGN_API_BASE);
const group = groupId => ({ type: 'group', groupId });
const arm = groupId => ({ type: 'arm', bundleId: 'bundle-warmup', groupId });
const before = (await api.list()).campaigns;
assert.ok((await api.actions()).actions.some(item => item.id === 'action-show-image' && item.requiresSlot));
assert.ok((await api.audienceReferences()).audienceReferences.some(item => item.bundleId === 'bundle-warmup'));
await assert.rejects(api.deleteTreatment('campaign-branch', 'treatment-rcs'), error => error.status === 409 && error.code === 'edge_conflict');
const created = await api.create({ name: 'HTTP 集成验收' });
try {
 assert.ok(created.id);
 assert.ok((await api.list()).campaigns.some(item => item.id === created.id));
 const added = await api.createTreatment(created.id, { name: '短信投放项', actionId: 'action-sms', audienceRef: group('group-new') });
 assert.ok((await api.usages(group('group-new'))).usages.some(item => item.campaignId === created.id && item.treatmentId === added.id));
 await api.updateTreatment(created.id, added.id, { ...added, audienceRef: group('group-recall') });
 assert.ok(!(await api.usages(group('group-new'))).usages.some(item => item.campaignId === created.id));
 assert.ok((await api.usages(group('group-recall'))).usages.some(item => item.campaignId === created.id));
 await assert.rejects(api.createTreatment(created.id, { name: '无展示位', actionId: 'action-show-image', audienceRef: group('group-new') }), error => error.status === 422 && error.code === 'missing_slot');
 await assert.rejects(api.createTreatment(created.id, { name: '带位推送', actionId: 'action-sms', audienceRef: group('group-new'), slotId: 'slot-home-banner' }), error => error.status === 422 && error.code === 'unexpected_slot');
 const imageA = await api.createTreatment(created.id, { name: 'A 展示', actionId: 'action-show-image', audienceRef: arm('group-arm-a'), slotId: 'slot-home-banner' });
 const imageB = await api.createTreatment(created.id, { name: 'B 展示', actionId: 'action-show-image', audienceRef: arm('group-arm-b'), slotId: 'slot-home-banner' });
 await assert.rejects(api.createTreatment(created.id, { name: '冲突展示', actionId: 'action-show-image', audienceRef: group('group-new'), slotId: 'slot-home-banner' }), error => error.status === 409 && error.code === 'slot_conflict');
 const detail = await api.get(created.id);
 assert.equal(detail.treatments.length, 3);
 const renamed = await api.update(created.id, { ...detail, name: 'HTTP 集成已改名' });
 assert.equal(renamed.name, 'HTTP 集成已改名');
 await api.deleteTreatment(created.id, added.id);
 await api.deleteTreatment(created.id, imageA.id);
 await api.deleteTreatment(created.id, imageB.id);
 assert.ok(!(await api.usages(group('group-recall'))).usages.some(item => item.campaignId === created.id));
 console.log(JSON.stringify({ step: 'campaign_crud_failure_and_usages', result: 'ok' }));
} finally {
 await api.delete(created.id);
 assert.ok(!(await api.list()).campaigns.some(item => item.id === created.id));
 assert.equal((await api.list()).campaigns.length, before.length);
}
