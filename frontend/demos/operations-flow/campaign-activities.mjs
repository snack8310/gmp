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
 { id: 'campaign-branch', name: '跨境召回', audience: { type: 'group', groupId: 'group-recall' }, treatments: [
  { id: 'treatment-rcs', name: 'RCS 投放项', actionRef: { id: 'action-rcs', name: 'RCS 消息' } },
  { id: 'treatment-email', name: '邮件投放项', actionRef: { id: 'action-email', name: '邮件' } },
  { id: 'treatment-sms', name: '短信投放项', actionRef: { id: 'action-sms', name: '短信' } },
 ], activityFlow: { id: 'flow-recall', entries: [
  { groupId: 'group-recall', kind: 'start', treatmentIds: ['treatment-rcs'] },
 ], edges: [
  { id: 'edge-success', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-email', conditionLabel: '成功' },
  { id: 'edge-failed', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-sms', conditionLabel: '失败' },
 ] } },
 { id: 'campaign-single', name: '新客欢迎', audience: { type: 'group', groupId: 'group-new' }, treatments: [
  { id: 'treatment-welcome', name: '欢迎短信', actionRef: { id: 'action-sms', name: '短信' } },
 ], activityFlow: { id: 'flow-welcome', entries: [
  { groupId: 'group-new', kind: 'start', treatmentIds: ['treatment-welcome'] },
 ], edges: [] } },
 { id: 'campaign-empty', name: '待装配活动', treatments: [] },
 { id: 'campaign-no-edges', name: '预热实验演练', audience: { type: 'bundle', bundleId: 'bundle-warmup' }, treatments: [
  { id: 'treatment-a', name: '提醒投放项', actionRef: { id: 'action-email', name: '邮件' } },
  { id: 'treatment-b', name: '提醒投放项', actionRef: { id: 'action-sms', name: '短信' } },
 ], activityFlow: { id: 'flow-no-edges', entries: [
  { groupId: 'group-arm-a', kind: 'start', treatmentIds: ['treatment-a'] },
  { groupId: 'group-arm-b', kind: 'start', treatmentIds: ['treatment-b'] },
  { groupId: 'group-arm-control', kind: 'control', treatmentIds: [] },
 ], edges: [] } },
];

export const INVALID_ACTIVITY = { id: 'campaign-invalid-ref', name: '失效引用示意', audience: { type: 'group', groupId: 'group-removed' }, treatments: [
 { id: 'treatment-old', name: '旧引用投放项', actionRef: { id: 'action-email', name: '邮件' } },
], activityFlow: { id: 'flow-invalid', entries: [{ groupId: 'group-removed', kind: 'start', treatmentIds: ['treatment-old'] }], edges: [] } };
export const BROWSE_ACTIVITIES = [...ACTIVITIES, INVALID_ACTIVITY];

const exactKeys = (value, keys) => !!value && typeof value === 'object' && Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key));
const nonBlank = value => typeof value === 'string' && !!value.trim();

export function actionAvailable(ref, catalog = REFERENCE_CATALOG) {
 return exactKeys(ref, ['id', 'name']) && nonBlank(ref.name) && catalog.actions.some(action => action.id === ref.id);
}

export function selectionReferences(selection, catalog = DEFAULT_AUDIENCE_CATALOG) {
 if (!selection) return [];
 if (selection.type === 'group' && exactKeys(selection, ['type', 'groupId'])) {
  return catalog.filter(item => item.type === 'group' && item.groupId === selection.groupId);
 }
 if (selection.type === 'bundle' && exactKeys(selection, ['type', 'bundleId'])) {
  return catalog.filter(item => item.type === 'arm' && item.bundleId === selection.bundleId);
 }
 return [];
}

export function audienceSelectionAvailable(selection, catalog = DEFAULT_AUDIENCE_CATALOG) {
 const selected = selectionReferences(selection, catalog);
 if (selection?.type === 'bundle') return selected.length > 0 && selected.every(item => item.available);
 return selected.length === 1 && selected[0].available;
}

export function audienceSelectionLabel(selection, catalog = DEFAULT_AUDIENCE_CATALOG) {
 const selected = selectionReferences(selection, catalog);
 if (selection?.type === 'group') return selected[0]?.groupName || selection.groupId || '未选择受众';
 if (selection?.type === 'bundle') return selected[0]?.bundleName || selection.bundleId || '未选择实验人群包';
 return '未选择受众';
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
  if (!actionAvailable(treatment.actionRef) || Object.hasOwn(treatment, 'audienceRef')) throw new Error(`投放项引用不可用：${treatment.id}`);
 }
 if (activity.audience && !audienceSelectionAvailable(activity.audience, catalog)) throw new Error(`活动受众引用不可用：${activity.id}`);
 return localIds;
}

function validateFlow(activity, flow, localIds, flowIds, edgeIds, catalog) {
 if (!flow) return;
 requireUnique(flow.id, flowIds, '活动流程');
 const selectedGroups = new Set(selectionReferences(activity.audience, catalog).map(item => item.groupId));
 const entryGroups = new Set();
 const routedTreatments = new Set();
 for (const entry of flow.entries || []) {
  requireText(entry.groupId, '流程入口人群组');
  if (entryGroups.has(entry.groupId)) throw new Error(`重复流程入口人群组：${entry.groupId}`);
  entryGroups.add(entry.groupId);
  if (!selectedGroups.has(entry.groupId)) throw new Error(`流程入口不属于活动受众：${entry.groupId}`);
  if (!['start', 'control'].includes(entry.kind)) throw new Error(`流程入口类型无效：${entry.kind}`);
  if (entry.kind === 'control' && entry.treatmentIds.length) throw new Error(`对照入口不得绑定投放项：${entry.groupId}`);
  for (const id of entry.treatmentIds) {
   if (!localIds.has(id) || routedTreatments.has(id)) throw new Error(`入口投放项无效或重复：${id}`);
   routedTreatments.add(id);
  }
 }
 if (entryGroups.size && [...selectedGroups].some(groupId => !entryGroups.has(groupId))) throw new Error('活动流程入口没有覆盖完整受众');
 for (const edge of flow.edges || []) {
  requireUnique(edge.id, edgeIds, '活动流程边');
  if (!localIds.has(edge.fromTreatmentId) || !localIds.has(edge.toTreatmentId)) throw new Error(`活动流程边端点不在同一活动：${edge.id}`);
  if (routedTreatments.has(edge.toTreatmentId)) throw new Error(`投放项不能同时作为入口和回执目标：${edge.toTreatmentId}`);
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
  validateFlow(activity, activity.activityFlow, localIds, flowIds, edgeIds, catalog);
 }
}

validateActivities(ACTIVITIES, DEFAULT_AUDIENCE_CATALOG);
