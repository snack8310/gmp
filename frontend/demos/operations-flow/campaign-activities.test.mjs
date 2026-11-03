import test from 'node:test';
import assert from 'node:assert/strict';
import { ACTIVITIES, INVALID_ACTIVITY, REFERENCE_CATALOG, DEFAULT_AUDIENCE_CATALOG, validateActivities } from './campaign-activities.mjs';
import { renderActivitiesList, renderActivityDetail } from './campaign-ui.mjs';
import { createAudienceStore } from './audience.mjs';

test('activities select one audience and route starts separately from receipt edges', () => {
 assert.deepEqual(Object.keys(REFERENCE_CATALOG), ['actions']);
 assert.doesNotThrow(() => validateActivities(ACTIVITIES, DEFAULT_AUDIENCE_CATALOG));
 const branch = ACTIVITIES.find(activity => activity.id === 'campaign-branch');
 assert.deepEqual(branch.audience, { type: 'group', groupId: 'group-recall' });
 assert.deepEqual(branch.activityFlow.entries, [{ groupId: 'group-recall', kind: 'start', treatmentIds: ['treatment-rcs'] }]);
 assert.deepEqual(branch.activityFlow.edges.map(edge => [edge.fromTreatmentId, edge.toTreatmentId, edge.conditionLabel]), [
  ['treatment-rcs', 'treatment-email', '成功'],
  ['treatment-rcs', 'treatment-sms', '失败'],
 ]);
 const experiment = ACTIVITIES.find(activity => activity.id === 'campaign-no-edges');
 assert.deepEqual(experiment.audience, { type: 'bundle', bundleId: 'bundle-warmup' });
 assert.deepEqual(experiment.activityFlow.entries.map(entry => entry.kind), ['start', 'start', 'control']);
 assert.throws(() => validateActivities([...ACTIVITIES, ACTIVITIES[0]]), /重复活动 ID/);
 assert.throws(() => validateActivities([{ ...ACTIVITIES[1], name: '' }]), /活动名称必填/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [...branch.treatments, branch.treatments[0]] }]), /重复投放项 ID/);
 assert.throws(() => validateActivities([{ ...branch, activityFlow: { id: 'flow-bad', entries: branch.activityFlow.entries, edges: [{ id: 'edge-bad', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-other', conditionLabel: '成功' }] } }]), /端点/);
 assert.throws(() => validateActivities([{ ...branch, audience: { type: 'group', groupId: 'group-new' } }]), /入口不属于活动受众/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [{ ...branch.treatments[0], audienceRef: { type: 'group', groupId: 'group-recall' } }, ...branch.treatments.slice(1)] }]), /投放项引用不可用/);
 assert.throws(() => validateActivities([INVALID_ACTIVITY]), /活动受众引用不可用/);
 const renamedCatalog = DEFAULT_AUDIENCE_CATALOG.map(item => item.bundleId === 'bundle-warmup' ? { ...item, bundleName: '预热包新名称' } : item);
 assert.doesNotThrow(() => validateActivities(ACTIVITIES, renamedCatalog));
});

test('list and detail show one activity audience, action treatments and the original canvas mount point', () => {
 const list = renderActivitiesList();
 assert.match(list, /href="#activities\/campaign-branch\/campaign"/);
 assert.match(list, /跨境召回/);
 assert.match(list, /活动受众/);
 const detail = renderActivityDetail('campaign-branch');
 const editableDetail = renderActivityDetail('campaign-branch', undefined, undefined, true);
 assert.match(detail, /RCS 投放项/);
 assert.match(detail, /treatment-rcs/);
 assert.match(detail, /treatment-email/);
 assert.match(detail, /treatment-sms/);
 assert.match(detail, /墨西哥 · 沉睡召回/);
 assert.match(detail, /每个活动只选择一个人群组/);
 assert.doesNotMatch(detail, /引用人群/);
 assert.match(detail, /id="activity-flow-canvas"/);
 assert.match(detail, /投放项定义活动里的动作；入口分配和后续回执路径在下方活动流程画布配置。/);
 assert.match(detail, /受众库 · 新建或管理/);
 assert.doesNotMatch(detail, /实验库 · 新建或管理|href="#experimentation\?return=/);
 assert.match(editableDetail, /<a class="primary-link" href="#activities\/campaign-branch\/campaign\/edit">编辑活动<\/a>/);
 assert.match(editableDetail, /<a class="primary-link" href="#activities\/campaign-branch\/campaign\/treatments\/new">＋ 添加投放项<\/a>/);
 assert.match(editableDetail, /href="#audience\?return=activities%2Fcampaign-branch%2Fcampaign"/);
 assert.doesNotMatch(editableDetail, /href="#experimentation\?return=/);
});

test('empty and unavailable states stay explicit without changing stable audience IDs', () => {
 assert.match(renderActivitiesList([]), /暂无演示活动/);
 assert.match(renderActivityDetail('campaign-empty'), /暂无投放项/);
 assert.match(renderActivityDetail('campaign-empty'), /id="activity-flow-canvas"/);
 assert.match(renderActivityDetail('campaign-no-edges'), /id="activity-flow-canvas"/);
 const invalid = renderActivityDetail(INVALID_ACTIVITY.id);
 assert.match(invalid, /引用不可用/);
 assert.match(invalid, /group-removed/);
 assert.match(invalid, /构造引用/);
 const missing = renderActivityDetail('missing-id');
 assert.match(missing, /未找到该演示活动/);
 assert.match(missing, /href="#activities"/);
 assert.doesNotMatch(missing, /跨境召回/);
});

test('same-name treatment rows stay distinguishable by stable ID', () => {
 const activity = ACTIVITIES.find(item => item.id === 'campaign-no-edges');
 const detail = renderActivityDetail(activity.id, [activity]);
 assert.match(detail, /提醒投放项 <small>· treatment-a<\/small>/);
 assert.match(detail, /提醒投放项 <small>· treatment-b<\/small>/);
});

test('unavailable action and selected experiment bundle keep original IDs and names', () => {
 const base = ACTIVITIES.find(item => item.id === 'campaign-single');
 const activity = { ...base, audience: { type: 'bundle', bundleId: 'bundle-retired' }, treatments: [{
  ...base.treatments[0], actionRef: { id: 'action-retired', name: '旧动作' },
 }] };
 const retiredRefs = [...DEFAULT_AUDIENCE_CATALOG, { type: 'arm', bundleId: 'bundle-retired', bundleName: '旧实验包', groupId: 'group-retired', armName: '旧臂', part: 'a', available: false }];
 const detail = renderActivityDetail(activity.id, [activity], DEFAULT_AUDIENCE_CATALOG, false, REFERENCE_CATALOG.actions, retiredRefs);
 for (const value of ['action-retired', '旧动作', 'bundle-retired']) assert.match(detail, new RegExp(value));
 assert.match(detail, /引用不可用/);
});

test('activity audience resolves current directory names by stable ID', () => {
 const store = createAudienceStore(null);
 const audience = store.list().find(item => item.id === 'aud-mx-dormant');
 store.save({ ...audience, name: '目录中的新名称' });
 const branch = ACTIVITIES.find(item => item.id === 'campaign-branch');
 const renamed = renderActivityDetail(branch.id, [branch], store.catalog());
 assert.match(renamed, /目录中的新名称/);
 assert.match(renamed, /group-recall/);
 assert.doesNotMatch(renamed, /墨西哥 · 沉睡召回/);
 const missing = renderActivityDetail(branch.id, [branch], store.catalog().filter(item => item.groupId !== 'group-recall'));
 assert.match(missing, /group-recall/);
 assert.match(missing, /引用不可用/);
 store.setExperimentStatus('exp-recall', 'stopped');
 const experiment = ACTIVITIES.find(item => item.id === 'campaign-no-edges');
 const stopped = renderActivityDetail(experiment.id, [experiment], store.catalog());
 assert.match(stopped, /bundle-warmup/);
 assert.match(stopped, /引用不可用/);
});

test('the same reusable audience can be selected by multiple activities', () => {
 const activities = ['campaign-one', 'campaign-two'].map((id, index) => ({
  id, name: id, audience: { type: 'bundle', bundleId: 'bundle-warmup' },
  treatments: [{ id: `treatment-${index}`, name: id, actionRef: { id: 'action-email', name: '邮件' } }],
  activityFlow: { id: `flow-${id}`, entries: [
   { groupId: 'group-arm-a', kind: 'start', treatmentIds: [`treatment-${index}`] },
   { groupId: 'group-arm-b', kind: 'start', treatmentIds: [] },
   { groupId: 'group-arm-control', kind: 'control', treatmentIds: [] },
  ], edges: [] },
 }));
 assert.doesNotThrow(() => validateActivities(activities, DEFAULT_AUDIENCE_CATALOG));
 for (const activity of activities) assert.match(renderActivityDetail(activity.id, activities, DEFAULT_AUDIENCE_CATALOG), /预热实验人群包/);
});
