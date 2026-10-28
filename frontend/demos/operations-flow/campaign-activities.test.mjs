import test from 'node:test';
import assert from 'node:assert/strict';
import { ACTIVITIES, INVALID_ACTIVITY, REFERENCE_CATALOG, validateActivities } from './campaign-activities.mjs';
import { renderActivitiesList, renderActivityDetail } from './campaign-ui.mjs';

test('normal activities keep unique identities and same-activity flow references', () => {
 assert.doesNotThrow(() => validateActivities(ACTIVITIES, REFERENCE_CATALOG));
 const branch = ACTIVITIES.find(activity => activity.id === 'campaign-branch');
 assert.deepEqual(branch.activityFlow.edges.map(edge => [edge.fromTreatmentId, edge.toTreatmentId, edge.conditionLabel]), [
  ['treatment-rcs', 'treatment-email', '成功'],
  ['treatment-rcs', 'treatment-sms', '失败'],
 ]);
 assert.throws(() => validateActivities([...ACTIVITIES, ACTIVITIES[0]], REFERENCE_CATALOG), /重复活动 ID/);
 assert.throws(() => validateActivities([{ ...ACTIVITIES[1], name: '' }], REFERENCE_CATALOG), /活动名称必填/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [...branch.treatments, branch.treatments[0]] }], REFERENCE_CATALOG), /重复投放项 ID/);
 assert.throws(() => validateActivities([{ ...branch, activityFlow: { id: 'flow-bad', edges: [{ id: 'edge-bad', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-other', conditionLabel: '成功' }] } }], REFERENCE_CATALOG), /端点/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [{ ...branch.treatments[0], audienceRef: { ...branch.treatments[0].audienceRef, bundleId: 'bundle-warmup' } }, ...branch.treatments.slice(1)] }], REFERENCE_CATALOG), /引用不可用/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [{ ...branch.treatments[0], audienceRef: { ...branch.treatments[0].audienceRef, bundleName: '预热实验人群包' } }, ...branch.treatments.slice(1)] }], REFERENCE_CATALOG), /引用不可用/);
 assert.throws(() => validateActivities([{ ...branch, treatments: [{ ...branch.treatments[0], actionRef: { ...branch.treatments[0].actionRef, secondActionId: 'action-sms' } }, ...branch.treatments.slice(1)] }], REFERENCE_CATALOG), /引用不可用/);
 assert.throws(() => validateActivities([INVALID_ACTIVITY], REFERENCE_CATALOG), /引用不可用/);
 const renamedCatalog = {
  ...REFERENCE_CATALOG,
  actions: REFERENCE_CATALOG.actions.map(action => action.id === 'action-rcs' ? { ...action, name: 'RCS 新名称' } : action),
  bundles: REFERENCE_CATALOG.bundles.map(bundle => bundle.bundleId === 'bundle-warmup' ? { ...bundle, bundleName: '预热包新名称' } : bundle),
 };
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
 assert.match(detail, /运营引用受众侧人群组或实验人群包的臂；分流定义归实验侧。/);
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
