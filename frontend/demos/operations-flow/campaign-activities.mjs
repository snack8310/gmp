import { buildReferenceCatalog, DEFAULT_AUDIENCES, DEFAULT_EXPERIMENTS, DEFAULT_BUNDLES } from './audience.mjs';

export const REFERENCE_CATALOG = {
 actions: [
  { id: 'action-rcs', name: 'RCS 消息' },
  { id: 'action-email', name: '邮件' },
  { id: 'action-sms', name: '短信' },
 ],
};
export const DEFAULT_AUDIENCE_CATALOG = buildReferenceCatalog(DEFAULT_AUDIENCES, DEFAULT_EXPERIMENTS, DEFAULT_BUNDLES);

export const ACTIVITIES = [
 { id: 'campaign-branch', name: '跨境召回', treatments: [
  { id: 'treatment-rcs', name: 'RCS 投放项', actionRef: { id: 'action-rcs', name: 'RCS 消息' }, audienceRef: { type: 'group', groupId: 'group-recall', groupName: '跨境召回人群组' } },
  { id: 'treatment-email', name: '邮件投放项', actionRef: { id: 'action-email', name: '邮件' }, audienceRef: { type: 'arm', bundleId: 'bundle-warmup', bundleName: '预热实验人群包', groupId: 'group-arm-a', armName: 'A 臂' } },
  { id: 'treatment-sms', name: '短信投放项', actionRef: { id: 'action-sms', name: '短信' }, audienceRef: { type: 'arm', bundleId: 'bundle-warmup', bundleName: '预热实验人群包', groupId: 'group-arm-b', armName: 'B 臂' } },
 ], activityFlow: { id: 'flow-recall', edges: [
  { id: 'edge-success', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-email', conditionLabel: '成功' },
  { id: 'edge-failed', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-sms', conditionLabel: '失败' },
 ] } },
 { id: 'campaign-single', name: '新客欢迎', treatments: [
  { id: 'treatment-welcome', name: '欢迎短信', actionRef: { id: 'action-sms', name: '短信' }, audienceRef: { type: 'group', groupId: 'group-new', groupName: '新客人群组' } },
 ] },
 { id: 'campaign-empty', name: '待装配活动', treatments: [] },
 { id: 'campaign-no-edges', name: '无边活动示意', treatments: [
  { id: 'treatment-a', name: '提醒投放项', actionRef: { id: 'action-email', name: '邮件' }, audienceRef: { type: 'group', groupId: 'group-new', groupName: '新客人群组' } },
  { id: 'treatment-b', name: '提醒投放项', actionRef: { id: 'action-sms', name: '短信' }, audienceRef: { type: 'group', groupId: 'group-recall', groupName: '跨境召回人群组' } },
 ], activityFlow: { id: 'flow-no-edges', edges: [] } },
];

export const INVALID_ACTIVITY = { id: 'campaign-invalid-ref', name: '失效引用示意', treatments: [
 { id: 'treatment-old', name: '旧引用投放项', actionRef: { id: 'action-email', name: '邮件' }, audienceRef: { type: 'group', groupId: 'group-removed', groupName: '旧人群组' } },
] };
export const BROWSE_ACTIVITIES = [...ACTIVITIES, INVALID_ACTIVITY];

const exactKeys = (value, keys) => !!value && typeof value === 'object' && Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key));
const nonBlank = value => typeof value === 'string' && !!value.trim();

export function actionAvailable(ref, catalog = REFERENCE_CATALOG) {
 return exactKeys(ref, ['id', 'name']) && nonBlank(ref.name) && catalog.actions.some(action => action.id === ref.id);
}

export function resolveAudience(ref, catalog = DEFAULT_AUDIENCE_CATALOG) {
 if (!ref) return false;
 if (ref.type === 'group' && exactKeys(ref, ['type', 'groupId', 'groupName']) && nonBlank(ref.groupName)) return catalog.find(group => group.type === 'group' && group.groupId === ref.groupId);
 if (ref.type === 'arm' && exactKeys(ref, ['type', 'bundleId', 'bundleName', 'groupId', 'armName']) && nonBlank(ref.bundleName) && nonBlank(ref.armName)) return catalog.find(arm => arm.type === 'arm' && arm.bundleId === ref.bundleId && arm.groupId === ref.groupId);
 return undefined;
}
export const audienceAvailable = (ref, catalog = DEFAULT_AUDIENCE_CATALOG) => Boolean(resolveAudience(ref, catalog)?.available);

function requireText(value, label) {
 if (typeof value !== 'string' || !value.trim()) throw new Error(`${label}必填`);
}

function requireUnique(id, seen, label) {
 requireText(id, `${label} ID `);
 if (seen.has(id)) throw new Error(`重复${label} ID：${id}`);
 seen.add(id);
}

function validateCatalog(catalog) {
 const groups = new Set();
 for (const group of catalog) {
  requireUnique(group.groupId, groups, '人群组');
  requireText(group.type === 'arm' ? group.armName : group.groupName, '人群组名称');
 }
}

function validateTreatments(activity, treatmentIds, catalog) {
 const localIds = new Set();
 for (const treatment of activity.treatments) {
  requireUnique(treatment.id, treatmentIds, '投放项');
  requireText(treatment.name, '投放项名称');
  localIds.add(treatment.id);
  if (!actionAvailable(treatment.actionRef) || !audienceAvailable(treatment.audienceRef, catalog)) throw new Error(`投放项引用不可用：${treatment.id}`);
 }
 return localIds;
}

function validateFlow(flow, localIds, flowIds, edgeIds) {
 if (!flow) return;
 requireUnique(flow.id, flowIds, '活动流程');
 for (const edge of flow.edges) {
  requireUnique(edge.id, edgeIds, '活动流程边');
  if (!localIds.has(edge.fromTreatmentId) || !localIds.has(edge.toTreatmentId)) throw new Error(`活动流程边端点不在同一活动：${edge.id}`);
  if (typeof edge.conditionLabel !== 'string' || !edge.conditionLabel.trim()) throw new Error(`回执条件必填：${edge.id}`);
 }
}

export function validateActivities(activities, catalog = DEFAULT_AUDIENCE_CATALOG) {
 validateCatalog(catalog);
 const activityIds = new Set(), treatmentIds = new Set(), flowIds = new Set(), edgeIds = new Set();
 for (const activity of activities) {
  requireUnique(activity.id, activityIds, '活动');
  requireText(activity.name, '活动名称');
  const localIds = validateTreatments(activity, treatmentIds, catalog);
  validateFlow(activity.activityFlow, localIds, flowIds, edgeIds);
 }
}

validateActivities(ACTIVITIES, DEFAULT_AUDIENCE_CATALOG);
