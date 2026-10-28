import test from 'node:test';
import assert from 'node:assert/strict';
import { ACTIVITIES, INVALID_ACTIVITY, REFERENCE_CATALOG, DEFAULT_AUDIENCE_CATALOG, validateActivities } from './campaign-activities.mjs';
import { renderActivitiesList, renderActivityDetail } from './campaign-ui.mjs';
import { createAudienceStore } from './audience.mjs';

test('normal activities keep unique identities and same-activity flow references', () => {
 assert.deepEqual(Object.keys(REFERENCE_CATALOG), ['actions']);
 assert.doesNotThrow(() => validateActivities(ACTIVITIES, DEFAULT_AUDIENCE_CATALOG));
 const branch = ACTIVITIES.find(activity => activity.id === 'campaign-branch');
 assert.deepEqual(branch.activityFlow.edges.map(edge => [edge.fromTreatmentId, edge.toTreatmentId, edge.conditionLabel]), [
  ['treatment-rcs', 'treatment-email', '成功'],
  ['treatment-rcs', 'treatment-sms', '失败'],
 ]);
 assert.throws(() => validateActivities([...ACTIVITIES, ACTIVITIES[0]]), /重复活动 ID/);
 assert.throws(() => validateActivities([{ ...ACTIVITIES[1], name: '' }]), /活动名称必填/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [...branch.treatments, branch.treatments[0]] }]), /重复投放项 ID/);
 assert.throws(() => validateActivities([{ ...branch, activityFlow: { id: 'flow-bad', edges: [{ id: 'edge-bad', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-other', conditionLabel: '成功' }] } }]), /端点/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [{ ...branch.treatments[0], audienceRef: { ...branch.treatments[0].audienceRef, bundleId: 'bundle-warmup' } }, ...branch.treatments.slice(1)] }]), /引用不可用/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [{ ...branch.treatments[0], audienceRef: { ...branch.treatments[0].audienceRef, bundleName: '预热实验人群包' } }, ...branch.treatments.slice(1)] }]), /引用不可用/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [{ ...branch.treatments[0], actionRef: { ...branch.treatments[0].actionRef, secondActionId: 'action-sms' } }, ...branch.treatments.slice(1)] }]), /引用不可用/);
 assert.throws(() => validateActivities([INVALID_ACTIVITY]), /引用不可用/);
 const renamedCatalog = DEFAULT_AUDIENCE_CATALOG.map(item => item.bundleId === 'bundle-warmup' ? { ...item, bundleName: '预热包新名称' } : item);
 assert.doesNotThrow(() => validateActivities(ACTIVITIES, renamedCatalog));
});

test('list and detail expose stable links, distinct audience types and receipt edges', () => {
 const list = renderActivitiesList();
 assert.match(list, /href="#campaign\/activities\/campaign-branch"/);
 assert.match(list, /跨境召回/);
 const detail = renderActivityDetail('campaign-branch');
 assert.match(detail, /RCS 投放项/);
 assert.match(detail, /预热实验人群包 \/ A 臂/);
 assert.match(detail, /treatment-rcs/);
 assert.match(detail, /treatment-email/);
 assert.match(detail, /treatment-sms/);
 assert.match(detail, /成功/);
 assert.match(detail, /失败/);
 assert.match(detail, /结果含义来自演示动作说明/);
 assert.match(detail, /普通人群组/);
 assert.match(detail, /实验人群包臂/);
 assert.match(detail, /运营引用受众侧统一目录中的人群组或实验人群包臂；分流定义归实验侧。/);
 assert.doesNotMatch(detail, /投放箱|<button|contenteditable|draggable/);
});

test('empty and unavailable states stay explicit without replacing references', () => {
 assert.match(renderActivitiesList([]), /暂无演示活动/);
 assert.match(renderActivityDetail('campaign-empty'), /暂无投放项/);
 assert.match(renderActivityDetail('campaign-empty'), /未配置活动流程/);
 assert.match(renderActivityDetail('campaign-no-edges'), /暂无活动流程边/);
 const invalid = renderActivityDetail(INVALID_ACTIVITY.id);
 assert.match(invalid, /引用不可用/);
 assert.match(invalid, /旧人群组/);
 assert.match(invalid, /group-removed/);
 assert.match(invalid, /构造引用/);
 const missing = renderActivityDetail('missing-id');
 assert.match(missing, /未找到该演示活动/);
 assert.match(missing, /href="#campaign\/activities"/);
 assert.doesNotMatch(missing, /跨境召回/);
});

test('same-name edge endpoints stay distinguishable by ID', () => {
 const activity = { ...ACTIVITIES.find(item => item.id === 'campaign-no-edges'), activityFlow: { id: 'flow-same-name', edges: [
  { id: 'edge-same-name', fromTreatmentId: 'treatment-a', toTreatmentId: 'treatment-b', conditionLabel: '成功' },
 ] } };
 const detail = renderActivityDetail(activity.id, [activity]);
 assert.match(detail, /提醒投放项 <small>· treatment-a<\/small> <span aria-hidden="true">→<\/span> 提醒投放项 <small>· treatment-b<\/small>/);
});

test('unavailable action and bundle keep their original IDs and names', () => {
 const base = ACTIVITIES.find(item => item.id === 'campaign-single');
 const activity = { ...base, treatments: [{
  ...base.treatments[0],
  actionRef: { id: 'action-retired', name: '旧动作' },
  audienceRef: { type: 'arm', bundleId: 'bundle-retired', bundleName: '旧实验包', groupId: 'group-retired', armName: '旧臂' },
 }] };
 const detail = renderActivityDetail(activity.id, [activity]);
 for (const value of ['action-retired', '旧动作', 'bundle-retired', '旧实验包', 'group-retired', '旧臂']) assert.match(detail, new RegExp(value));
 assert.match(detail, /引用不可用/);
});

test('activity resolves current directory names by stable ID and never falls back to sample names', () => {
 const store = createAudienceStore(null);
 const audience = store.list().find(item => item.id === 'aud-mx-dormant');
 store.save({ ...audience, name: '目录中的新名称' });
 const branch = ACTIVITIES.find(item => item.id === 'campaign-branch');
 const renamed = renderActivityDetail(branch.id, [branch], store.catalog());
 assert.match(renamed, /目录中的新名称/);
 assert.match(renamed, /group-recall/);
 assert.doesNotMatch(renamed, /跨境召回人群组/);
 const missing = renderActivityDetail(branch.id, [branch], store.catalog().filter(item => item.groupId !== 'group-recall'));
 assert.match(missing, /跨境召回人群组/);
 assert.match(missing, /group-recall/);
 assert.match(missing, /引用不可用/);
 store.setExperimentStatus('exp-recall', 'stopped');
 const stopped = renderActivityDetail(branch.id, [branch], store.catalog());
 assert.match(stopped, /bundle-warmup \/ group-arm-a/);
 assert.match(stopped, /预热实验人群包 \/ A 臂/);
 assert.match(stopped, /引用不可用/);
});

test('one published arm can be cited by multiple activities under the same bundle and group IDs', () => {
 const store = createAudienceStore(null);
 const arm = store.catalog().find(item => item.bundleId === 'bundle-warmup' && item.part === 'a');
 const ref = { type: 'arm', bundleId: arm.bundleId, bundleName: arm.bundleName, groupId: arm.groupId, armName: arm.armName };
 const activities = ['campaign-one', 'campaign-two'].map((id, index) => ({ id, name: id, treatments: [
  { id: `treatment-${index}`, name: id, actionRef: { id: 'action-email', name: '邮件' }, audienceRef: ref },
 ] }));
 assert.doesNotThrow(() => validateActivities(activities, store.catalog()));
 for (const activity of activities) assert.match(renderActivityDetail(activity.id, activities, store.catalog()), /bundle-warmup \/ group-arm-a/);
});
