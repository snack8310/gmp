import test from 'node:test';
import assert from 'node:assert/strict';
import { validateTreatmentDraft, renderActivityDetail, renderActivitiesList } from './campaign-ui.mjs';

const actions = [{ id: 'action-sms', name: '短信', available: true, requiresSlot: false }, { id: 'action-show-image', name: '图片展示', available: true, requiresSlot: true }];
const refs = [{ type: 'arm', bundleId: 'bundle-a', groupId: 'group-a', name: 'A 臂', available: true }, { type: 'arm', bundleId: 'bundle-a', groupId: 'group-b', name: 'B 臂', available: true }, { type: 'group', groupId: 'group-c', name: '普通组', available: true }];
const arm = groupId => ({ type: 'arm', bundleId: 'bundle-a', groupId });

test('treatment editor enforces action, audience, slot and same-slot exclusivity', () => {
 const base = { name: '展示', actionId: 'action-show-image', audienceRef: arm('group-a'), slotId: 'slot-home-banner' };
 assert.equal(validateTreatmentDraft(base, actions, refs, []), '');
 assert.match(validateTreatmentDraft({ ...base, actionId: '' }, actions, refs, []), /动作/);
 assert.match(validateTreatmentDraft({ ...base, audienceRef: null }, actions, refs, []), /人群/);
 assert.match(validateTreatmentDraft({ ...base, slotId: '' }, actions, refs, []), /展示位/);
 assert.match(validateTreatmentDraft({ ...base, actionId: 'action-sms' }, actions, refs, []), /不得带展示位/);
 assert.match(validateTreatmentDraft({ ...base, actionId: 'action-retired' }, actions, refs, []), /不可用/);
 assert.match(validateTreatmentDraft({ ...base, audienceRef: { type: 'group', groupId: 'missing' } }, actions, refs, []), /不可用/);
 assert.equal(validateTreatmentDraft({ ...base, audienceRef: arm('group-b') }, actions, refs, [base]), '');
 assert.match(validateTreatmentDraft({ ...base, audienceRef: refs[2] }, actions, refs, [base]), /同一展示位/);
 assert.equal(validateTreatmentDraft({ ...base, actionId: 'action-sms', slotId: '' }, actions, refs, [base]), '');
});

test('API-shaped detail renders stable IDs and unavailable references without local seeds', () => {
 const activity = { id: 'campaign-api', name: '接口活动', treatments: [{ id: 't-1', name: '投放项', actionId: 'action-sms', audienceRef: { type: 'group', groupId: 'group-lost' } }] };
 const detail = renderActivityDetail(activity.id, [activity], [], true, actions, [{ type: 'group', groupId: 'group-lost', name: '原名称', available: false }]);
 assert.match(detail, /group-lost/); assert.match(detail, /原名称/); assert.match(detail, /引用不可用/);
 assert.match(detail, /treatments\/t-1\/edit/);
 const missingRemote = renderActivityDetail(activity.id, [activity], [{ type: 'group', groupId: 'group-lost', groupName: '原名称', available: true }], true, actions, []);
 assert.match(missingRemote, /引用不可用/);
 assert.doesNotMatch(renderActivitiesList([], true), /campaign-branch/);
});
