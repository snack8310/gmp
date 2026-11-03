import { buildActivityGraph } from './activity-flow-canvas.mjs';

const flowGraphParts = ['a', 'b', 'control'];
const flowGraphLabels = { a: 'A 组', b: 'B 组', control: '对照组' };
const flowTerminal = 'flow-terminal';
const flowBranchId = (nodeId, part) => `experiment:${nodeId}:${part}`;
export const flowAnchorSignature = edge => JSON.stringify([edge.from, edge.to, edge.kind, edge.label]);

export function flowAnchors(base) {
 const anchors = base.connections.filter(item => item.kind === 'entry' || item.kind === 'receipt' || item.kind === 'complete' || item.kind === 'experiment');
 if (base.audienceNode && !anchors.some(item => item.id === 'empty:entry') && !base.connections.some(item => item.kind === 'entry' || item.kind === 'split') && base.entries.length === 0) {
  anchors.push({ id: 'empty:entry', from: base.audienceNode.id, to: flowTerminal, kind: 'entry', label: '受众入口（空流程）' });
 }
 return anchors;
}

export function composeFlowExperimentGraph(activity, actions, references, experiments, draftNodes = []) {
 const base = buildActivityGraph(activity, actions, references);
 const nodes = [...base.nodes], byId = new Map(base.byId), connections = [...base.connections];
 const issues = [];
 if (!byId.has(flowTerminal) && base.audienceNode) {
  const end = { id: flowTerminal, name: '演练结束', kind: 'terminal', x: 455, z: 0, w: 94, d: 82, h: 30, desc: '本地构造路径结束。', rules: [] };
  nodes.push(end); byId.set(flowTerminal, end);
 }
 if (base.audienceNode && !connections.some(item => item.kind === 'entry' || item.kind === 'split') && base.entries.length === 0) {
  connections.push({ id: 'empty:entry', from: base.audienceNode.id, to: flowTerminal, kind: 'entry', label: '受众入口（空流程）' });
 }
 const rawAnchors = [...connections];
 const ids = new Set();
 for (const draft of draftNodes) {
  if (ids.has(draft.id)) issues.push(`节点 ${draft.id} 的 ID 重复。`);
  ids.add(draft.id);
  const experiment = experiments.find(item => item.id === draft.experimentId);
  if (!experiment || experiment.runtimeStatus !== 'running') issues.push(`节点 ${draft.id} 引用的实验已停用或不存在。`);
  const anchorIndex = connections.findIndex(item => item.id === draft.anchorId);
  if (anchorIndex < 0) { issues.push(`节点 ${draft.id} 的插入路径已变化，请重新连接。`); continue; }
  const anchor = connections[anchorIndex];
  if (anchor.kind === 'control' || anchor.kind === 'split') { issues.push(`节点 ${draft.id} 不可插入受众包对照或分组入口。`); continue; }
  if (draft.anchorSignature !== flowAnchorSignature(anchor)) { issues.push(`节点 ${draft.id} 的插入路径已变化，请重新连接。`); continue; }
  const id = `flow-experiment:${draft.id}`;
  const targetSet = new Set(flowGraphParts.map(part => draft.routes?.[part]));
  if (targetSet.has(undefined) || targetSet.has(null)) { issues.push(`节点 ${draft.id} 的三组去向未配齐。`); continue; }
  const invalid = [...targetSet].filter(target => target !== flowTerminal && !['treatment', 'experiment'].includes(byId.get(target)?.kind)
   && !draftNodes.some(other => `flow-experiment:${other.id}` === target));
  if (invalid.length) { issues.push(`节点 ${draft.id} 的去向无效或已不存在，请重新连接。`); continue; }
  const x = (byId.get(anchor.from)?.x + byId.get(anchor.to)?.x) / 2 || 0;
  const z = (byId.get(anchor.from)?.z + byId.get(anchor.to)?.z) / 2 || 0;
  const node = { id, name: experiment?.name || draft.experimentId, kind: 'experiment', x, z, w: 80, d: 74, h: 44,
   desc: `活动流程实验节点 · ${draft.experimentId}。共享实验库比例与策略；本节点独立分组，仅本浏览器草稿。`,
   rules: [['实验定义', draft.experimentId], ['分流比例', experiment ? `A ${experiment.split.a}% · B ${experiment.split.b}% · 对照 ${experiment.split.control}%` : '引用不可用'], ['染色策略', experiment?.colored ? '记录首次模拟分配' : '不记录分配']], draft };
  nodes.push(node); byId.set(id, node);
  connections.splice(anchorIndex, 1, { ...anchor, id: `experiment-in:${draft.id}`, to: id });
  for (const part of flowGraphParts) connections.push({ id: flowBranchId(draft.id, part), from: id, to: draft.routes[part], kind: 'experiment', label: flowGraphLabels[part], part, experimentNodeId: draft.id });
 }
 // A draft can reference an existing branch even when persisted order differs.
 for (const draft of draftNodes) if (draft.anchorId.startsWith('experiment:') && !rawAnchors.some(item => item.id === draft.anchorId)
  && !connections.some(item => item.id === `experiment-in:${draft.id}`)) issues.push(`节点 ${draft.id} 的上游实验分支未接通。`);
 const outgoing = new Map(nodes.map(item => [item.id, []]));
 for (const connection of connections) outgoing.get(connection.from)?.push(connection.to);
 const visited = new Set(), visiting = new Set();
 function walk(id) {
  if (visiting.has(id)) return true;
  if (visited.has(id)) return false;
  visiting.add(id);
  for (const next of outgoing.get(id) || []) if (walk(next)) return true;
  visiting.delete(id); visited.add(id); return false;
 }
 if (nodes.some(node => walk(node.id))) issues.push('实验分支形成循环，请调整去向。');
 const reachable = new Set(), queue = base.audienceNode ? [base.audienceNode.id] : [];
 while (queue.length) {
  const id = queue.shift(); if (reachable.has(id)) continue; reachable.add(id);
  queue.push(...outgoing.get(id) || []);
 }
 const baseNext = new Map(base.nodes.map(item => [item.id, base.connections.filter(edge => edge.from === item.id).map(edge => edge.to)]));
 const originallyReachable = new Set(), baseQueue = base.audienceNode ? [base.audienceNode.id] : [];
 while (baseQueue.length) {
  const id = baseQueue.shift(); if (originallyReachable.has(id)) continue; originallyReachable.add(id);
  baseQueue.push(...baseNext.get(id) || []);
 }
 const draftStartsEmptyFlow = base.entries.length === 0 && draftNodes.some(node => node.anchorId === 'empty:entry');
 for (const item of base.treatments) if ((originallyReachable.has(item.id) || draftStartsEmptyFlow) && !reachable.has(item.id)) issues.push(`投放项 ${item.name || item.id} 不可达，请重新连接。`);
 const localEmptyEntry = draftNodes.some(node => node.anchorId === 'empty:entry') && base.entries.length === 0 && activity?.audience?.type === 'group';
 const readinessIssue = issues[0] || (localEmptyEntry && base.audienceNode && references.some(item => item.type === 'group' && item.groupId === activity.audience.groupId && item.available) ? '' : base.readinessIssue);
 return { ...base, nodes, byId, connections, draftNodes, issues, canRehearse: !readinessIssue, readinessIssue, anchors: flowAnchors({ ...base, connections, entries: base.entries }), hasCycle: base.hasCycle || issues.some(issue => issue.includes('循环')) };
}

export function simulateFlowRuns(graph, users, groupFor, entryAllowed = () => true) {
 const runs = [], visits = new Map(graph.draftNodes.map(node => [node.id, { people: new Set(), arrivals: 0, groups: { a: new Set(), b: new Set(), control: new Set() } }]));
 const outgoing = new Map(graph.nodes.map(node => [node.id, graph.connections.filter(edge => edge.from === node.id)]));
 const root = graph.audienceNode?.id;
 if (!root) return { runs, visits };
 const each = graph.entries.length && graph.entryNodes.size ? graph.entries.map(entry => ({ from: `entry:${entry.groupId}` })) : [{ from: root }];
 for (const entry of each) for (let index = 0; index < users.length; index++) {
  const uid = users[index].uid, path = entry.from === root ? [root] : [root, entry.from];
  if (entry.from !== root && !entryAllowed(entry.from.slice('entry:'.length), uid)) continue;
  const route = (current, trail) => {
   if (trail.has(current) || path.length > graph.nodes.length + 2) return;
   const nextTrail = new Set(trail); nextTrail.add(current);
   const node = graph.byId.get(current);
   let branches = outgoing.get(current) || [];
   if (node?.kind === 'experiment') {
    const record = visits.get(node.draft.id); record.arrivals++; record.people.add(uid);
    const part = groupFor(node.draft, uid); record.groups[part].add(uid);
    branches = branches.filter(edge => edge.part === part);
   } else if (node?.kind === 'treatment') {
    const declared = graph.actionById.get(node.treatment.actionId)?.receiptOutcomes || [];
    const condition = declared.length ? declared[index % declared.length] : '';
    branches = branches.filter(edge => edge.kind === 'receipt' ? edge.label === condition : edge.kind === 'complete' ? (!condition || edge.label === condition) : true);
   }
   if (!branches.length) { runs.push({ user: index, uid, path: [...path] }); return; }
   for (const edge of branches) { path.push(edge.to); route(edge.to, nextTrail); path.pop(); }
  };
  route(entry.from, new Set());
 }
 return { runs, visits };
}
