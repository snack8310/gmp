import test from 'node:test';
import assert from 'node:assert/strict';
import { validateTreatmentDraft, renderActivityDetail, renderActivitiesList } from './campaign-ui.mjs';

const actions = [{ id: 'action-sms', name: '短信', available: true, requiresSlot: false }, { id: 'action-show-image', name: '图片展示', available: true, requiresSlot: true }];

test('treatment editor validates its action and slot without assigning an audience', () => {
 const base = { name: '展示', actionId: 'action-show-image', slotId: 'slot-home-banner' };
 assert.equal(validateTreatmentDraft(base, actions, [], []), '');
 assert.match(validateTreatmentDraft({ ...base, actionId: '' }, actions, [], []), /动作/);
 assert.match(validateTreatmentDraft({ ...base, slotId: '' }, actions, [], []), /展示位/);
 assert.match(validateTreatmentDraft({ ...base, actionId: 'action-sms' }, actions, [], []), /不得带展示位/);
 assert.match(validateTreatmentDraft({ ...base, actionId: 'action-retired' }, actions, [], []), /不可用/);
 assert.equal(validateTreatmentDraft({ ...base, actionId: 'action-sms', slotId: '' }, actions, [], []), '');
});

test('API detail shows unavailable audience on the activity and action on the treatment', () => {
 const activity = { id: 'campaign-api', name: '接口活动', audience: { type: 'group', groupId: 'group-lost' }, treatments: [{ id: 't-1', name: '投放项', actionId: 'action-sms' }] };
 const detail = renderActivityDetail(activity.id, [activity], [], true, actions, [{ type: 'group', groupId: 'group-lost', name: '原名称', available: false }]);
 assert.match(detail, /group-lost/); assert.match(detail, /原名称/); assert.match(detail, /引用不可用/);
 assert.match(detail, /treatments\/t-1\/edit/);
 assert.doesNotMatch(detail, /treatment-audience|引用人群/);
 const missingRemote = renderActivityDetail(activity.id, [activity], [{ type: 'group', groupId: 'group-lost', groupName: '原名称', available: true }], true, actions, []);
 assert.match(missingRemote, /引用不可用/);
 assert.doesNotMatch(renderActivitiesList([], true), /campaign-branch/);
});

test('API audience references are authoritative when local browser fixtures do not contain the selection', () => {
 const activity = { id: 'campaign-api', name: '接口活动', audience: { type: 'group', groupId: 'group-recall' }, treatments: [] };
 const detail = renderActivityDetail(activity.id, [activity], [], true, actions,
  [{ type: 'group', groupId: 'group-recall', name: '跨境召回人群组', available: true }]);
 assert.match(detail, /跨境召回人群组/);
 assert.doesNotMatch(detail, /引用不可用/);
});
