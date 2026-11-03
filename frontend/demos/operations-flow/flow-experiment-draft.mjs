import { assignGroup, MOCK_USERS } from './audience.mjs';

export const FLOW_EXPERIMENT_KEY = 'gmp-flow-experiment-draft-v1';
const flowDraftParts = ['a', 'b', 'control'];
const flowMockIds = new Set(MOCK_USERS.map(user => user.uid));
const flowValidId = value => typeof value === 'string' && /^[a-zA-Z0-9_-]{1,64}$/.test(value) && !['__proto__', 'constructor', 'prototype'].includes(value);
const flowActivityId = value => typeof value === 'string' && value.trim() && value.length <= 200 && !['__proto__', 'constructor', 'prototype'].includes(value);
const flowPlain = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const flowCopy = value => JSON.parse(JSON.stringify(value));

export function parseFlowDraftJSON(raw) {
 const value = JSON.parse(raw);
 const stack = [];
 for (let index = 0; index < raw.length; index++) {
  const character = raw[index];
  if (character === '{') stack.push({ object: true, keys: new Set(), expectKey: true });
  else if (character === '[') stack.push({ object: false });
  else if (character === '}' || character === ']') stack.pop();
  else if (character === ',' && stack.at(-1)?.object) stack.at(-1).expectKey = true;
  else if (character === '"') {
   const start = index;
   for (index++; index < raw.length; index++) {
    if (raw[index] === '\\') { index++; continue; }
    if (raw[index] === '"') break;
   }
   const frame = stack.at(-1);
   if (frame?.object && frame.expectKey) {
    const key = JSON.parse(raw.slice(start, index + 1));
    if (frame.keys.has(key)) throw new SyntaxError('Duplicate JSON key');
    frame.keys.add(key); frame.expectKey = false;
   }
  }
 }
 return value;
}

export function validateFlowDraft(value) {
 if (!flowPlain(value) || value.version !== 1 || !flowPlain(value.activities) || Object.keys(value.activities).length > 100) return '流程实验草稿版本或结构无效。';
 for (const [activityId, activity] of Object.entries(value.activities)) {
  if (!flowActivityId(activityId) || !flowPlain(activity) || !Array.isArray(activity.nodes) || activity.nodes.length > 100 || !flowPlain(activity.coloring)) return '流程实验活动记录无效。';
  const nodeIds = new Set();
  for (const node of activity.nodes) {
   if (!flowPlain(node) || !flowValidId(node.id) || !flowValidId(node.experimentId) || typeof node.anchorId !== 'string' || node.anchorId.length > 200
    || node.anchorSignature !== undefined && (typeof node.anchorSignature !== 'string' || node.anchorSignature.length > 500)
    || !flowPlain(node.routes) || Object.keys(node.routes).length !== 3 || !flowDraftParts.every(part => typeof node.routes[part] === 'string' && node.routes[part].length <= 200)
    || nodeIds.has(node.id)) return '流程实验节点记录无效。';
   nodeIds.add(node.id);
  }
  for (const [nodeId, records] of Object.entries(activity.coloring)) {
   if (!nodeIds.has(nodeId) || !flowPlain(records) || Object.keys(records).length > flowMockIds.size
    || Object.entries(records).some(([uid, part]) => !flowMockIds.has(uid) || !flowDraftParts.includes(part))) return '流程实验模拟染色记录无效。';
  }
 }
 return '';
}

export function createFlowExperimentStore(storage) {
 let data = { version: 1, activities: {} }, warning = '', writable = true, observedRaw = null;
 const log = (operation, outcome) => globalThis.console?.info?.(JSON.stringify({ component: 'flow-experiment-draft', operation, outcome }));
 try {
  if (!storage?.getItem || !storage?.setItem) throw new Error('storage unavailable');
  const raw = storage.getItem(FLOW_EXPERIMENT_KEY);
  if (raw !== null) {
   let parsed;
   try { parsed = parseFlowDraftJSON(raw); } catch { throw new SyntaxError('invalid json'); }
   if (validateFlowDraft(parsed)) throw new SyntaxError('invalid draft');
   data = parsed;
  }
  observedRaw = raw;
  log('read', 'ok');
 } catch (error) {
  writable = false;
  warning = error instanceof SyntaxError ? '本地流程实验草稿损坏或版本不兼容；原文已保留，当前页面不可保存或演练。' : '浏览器存储不可用；流程实验草稿只能保留在当前会话。';
  log('read', error instanceof SyntaxError ? 'invalid' : 'unavailable');
 }
 const sync = () => {
  if (!writable) return;
  try {
   const raw = storage.getItem(FLOW_EXPERIMENT_KEY);
   if (raw === observedRaw) return;
   const latest = raw === null ? { version: 1, activities: {} } : parseFlowDraftJSON(raw);
   if (validateFlowDraft(latest)) throw new SyntaxError('invalid draft');
   data = latest; observedRaw = raw; log('read', 'ok');
  } catch (error) {
   writable = false;
   warning = error instanceof SyntaxError ? '本地流程实验草稿损坏或版本不兼容；原文已保留，当前页面不可保存或演练。' : '浏览器存储不可用；无法确认流程实验引用占用。';
   log('read', error instanceof SyntaxError ? 'invalid' : 'unavailable');
  }
 };
 const persist = () => {
  if (!writable) return false;
  try { const raw = JSON.stringify(data); storage.setItem(FLOW_EXPERIMENT_KEY, raw); observedRaw = raw; log('write', 'ok'); return true; }
  catch { writable = false; warning = '浏览器写入失败；修改仅保留在当前会话，刷新后可能丢失。'; log('write', 'unavailable'); return false; }
 };
 const activity = id => data.activities[id] ?? { nodes: [], coloring: {} };
 const definition = (experiments, node) => experiments.find(item => item.id === node.experimentId && item.runtimeStatus === 'running');
 return {
  get warning() { return warning; },
  get canUse() { return !warning.includes('损坏'); },
  nodes(id) { sync(); return flowCopy(activity(id).nodes); },
  references(experimentId) {
   sync();
   if (!writable || !this.canUse) throw new RangeError('无法确认流程实验草稿的引用占用；请先恢复浏览器存储再删除实验。');
   return Object.entries(data.activities).flatMap(([activityId, value]) => value.nodes.filter(node => node.experimentId === experimentId).map(node => ({ activityId, nodeId: node.id })));
  },
  saveNode(activityId, node) {
   if (!flowActivityId(activityId) || !flowValidId(node?.id)) throw new RangeError('活动或节点 ID 无效。');
   sync();
   if (!this.canUse) throw new RangeError(warning);
   const next = flowCopy(data), record = next.activities[activityId] ?? { nodes: [], coloring: {} };
   const index = record.nodes.findIndex(item => item.id === node.id);
   if (index < 0) record.nodes.push(flowCopy(node));
   else {
    if (record.nodes[index].experimentId !== node.experimentId) delete record.coloring[node.id];
    record.nodes[index] = flowCopy(node);
   }
   next.activities[activityId] = record;
   const problem = validateFlowDraft(next);
   if (problem) throw new RangeError(problem);
   data = next; persist(); return flowCopy(node);
  },
  removeNode(activityId, nodeId) {
   sync();
   if (!this.canUse) throw new RangeError(warning);
   const next = flowCopy(data), record = next.activities[activityId];
   if (!record || !record.nodes.some(node => node.id === nodeId)) throw new RangeError('流程实验节点不存在。');
   record.nodes = record.nodes.filter(node => node.id !== nodeId); delete record.coloring[nodeId];
   data = next; persist();
  },
  removeActivity(activityId) {
   sync();
   if (!this.canUse) throw new RangeError(warning);
   if (!Object.hasOwn(data.activities, activityId)) return;
   const next = flowCopy(data); delete next.activities[activityId]; data = next; persist();
  },
  coloring(activityId, nodeId) { return flowCopy(activity(activityId).coloring[nodeId] ?? {}); },
  group(activityId, node, uid, experiments, record = false) {
   if (!flowMockIds.has(uid)) throw new RangeError('仅支持样本池内的构造 ID。');
   sync();
   if (!this.canUse) throw new RangeError(warning);
   const experiment = definition(experiments, node);
   if (!experiment) throw new RangeError('实验定义已停用或不存在。');
   const existing = activity(activityId).coloring[node.id]?.[uid];
   const part = existing ?? assignGroup(uid, node.experimentId, experiment.split, [activityId, node.id]);
   if (record && experiment.colored && !existing) {
    const next = flowCopy(data), value = next.activities[activityId] ?? { nodes: [], coloring: {} };
    (value.coloring[node.id] ??= {})[uid] = part; next.activities[activityId] = value;
    data = next; persist();
   }
   return part;
  },
 };
}
