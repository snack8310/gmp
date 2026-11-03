import { activityRoute, assetRoute } from './activity-routing.mjs';
import { composeFlowExperimentGraph, simulateFlowRuns } from './flow-experiment-graph.mjs';
import { MOCK_USERS } from './audience.mjs';

const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
const colors = { blue: '#2c4a7c', paper: '#fbfaf7', white: '#ffffff', line: '#cfc9be', side: '#e4e0d8', ink: '#55504a' };

function normalizedOutcome(value) {
 const text = String(value || '').trim().toLocaleLowerCase();
 if (text === 'success' || text === '成功') return 'success';
 if (text === 'failure' || text === 'failed' || text === '失败') return 'failure';
 return text;
}

function hasCycle(treatments, edges) {
 const next = new Map(treatments.map(item => [item.id, []]));
 for (const edge of edges) next.get(edge.fromTreatmentId)?.push(edge.toTreatmentId);
 const visiting = new Set(), visited = new Set();
 const visit = id => {
  if (visiting.has(id)) return true;
  if (visited.has(id)) return false;
  visiting.add(id);
  for (const target of next.get(id) || []) if (visit(target)) return true;
  visiting.delete(id); visited.add(id); return false;
 };
 return treatments.some(item => visit(item.id));
}

function rehearsalReadiness(activity, treatments, entries, edges, selectedRefs, cyclic) {
 const selection = activity?.audience;
 if (!selection) return '先为活动选择一个受众，再配置完整流程入口。';
 if (!selectedRefs.length || selectedRefs.some(item => !item.available)) return '当前活动受众不可用，请选择有效的人群定义。';
 if (!activity?.activityFlow?.id) return '先创建活动流程并配置入口。';
 if (cyclic) return '当前活动边包含循环，先删除循环边再运行。';

 const treatmentById = new Map(treatments.map(item => [item.id, item]));
 if (treatmentById.size !== treatments.length || treatments.some(item => !item.id)) return '投放项 ID 缺失或重复，暂时无法演练。';
 const refsByGroup = new Map(selectedRefs.map(item => [item.groupId, item]));
 if (refsByGroup.size !== selectedRefs.length) return '活动受众入口存在重复定义，暂时无法演练。';
 const entriesByGroup = new Map(entries.map(item => [item.groupId, item]));
 if (entriesByGroup.size !== entries.length || entries.length !== refsByGroup.size) return '请为活动受众的每个入口配置且只配置一条路由。';
 for (const [groupId, ref] of refsByGroup) {
  const entry = entriesByGroup.get(groupId);
  if (!entry) return '请为活动受众的每个入口配置且只配置一条路由。';
  if (!Array.isArray(entry.treatmentIds)) return `入口 ${ref.armName || ref.groupName || ref.name || groupId} 缺少投放项列表。`;
  const isControl = ref.type === 'arm' && ref.part === 'control';
  if (isControl) {
   if (entry.kind !== 'control' || entry.treatmentIds.length) return `对照臂 ${ref.armName || ref.name || groupId} 应配置为空路径。`;
  } else if (entry.kind !== 'start' || !entry.treatmentIds.length) {
   return `请为 ${ref.armName || ref.groupName || ref.name || groupId} 配置至少一个起始投放项。`;
  }
 }
 if (entries.some(entry => !refsByGroup.has(entry.groupId))) return '流程入口中包含不属于当前活动受众的路由。';

 const starts = new Set();
 for (const entry of entries) {
  for (const id of entry.treatmentIds) {
   if (!treatmentById.has(id) || starts.has(id)) return '入口路由引用了无效或重复的投放项。';
   starts.add(id);
  }
 }
 const incoming = new Set();
 const next = new Map(treatments.map(item => [item.id, []]));
 for (const edge of edges) {
  if (!treatmentById.has(edge.fromTreatmentId) || !treatmentById.has(edge.toTreatmentId) || edge.fromTreatmentId === edge.toTreatmentId || !edge.conditionLabel) {
   return '活动流程边引用了无效投放项或缺少回执条件。';
  }
  incoming.add(edge.toTreatmentId);
  next.get(edge.fromTreatmentId).push(edge.toTreatmentId);
 }
 if ([...starts].some(id => incoming.has(id))) return '入口投放项不能同时作为回执边的后续目标。';
 const reachable = new Set(starts), queue = [...starts];
 while (queue.length) {
  for (const id of next.get(queue.shift()) || []) {
   if (!reachable.has(id)) { reachable.add(id); queue.push(id); }
  }
 }
 if (reachable.size !== treatments.length) return '还有投放项未连接到任何受众入口，请补齐活动流程边。';
 return '';
}

export function buildActivityGraph(activity, actions = [], references = []) {
 const treatments = Array.isArray(activity?.treatments) ? activity.treatments : [];
 const edges = Array.isArray(activity?.activityFlow?.edges) ? activity.activityFlow.edges : [];
 const entries = Array.isArray(activity?.activityFlow?.entries) ? activity.activityFlow.entries : [];
 const actionById = new Map(actions.map(item => [item.id, item]));
 const incoming = new Set(edges.map(edge => edge.toTreatmentId));
 const selection = activity?.audience;
 const selectedRefs = selection?.type === 'group'
  ? references.filter(item => item.type === 'group' && item.groupId === selection.groupId)
  : selection?.type === 'bundle' ? references.filter(item => item.type === 'arm' && item.bundleId === selection.bundleId) : [];
 const audienceName = selection?.type === 'bundle'
  ? selectedRefs[0]?.bundleName || selection.bundleId
  : selectedRefs[0]?.groupName || selectedRefs[0]?.name || selection?.groupId || '未选择受众';
 const nodes = [];
 const audienceNode = selection ? { id: 'audience:activity', name: audienceName, kind: 'audience', x: -440, z: 0, w: 138, d: 108, h: 16,
  desc: selection.type === 'bundle' ? '活动唯一受众是一个实验人群包；各实验臂在流程入口处分配到不同起始路径。' : '活动唯一受众是一个共享人群组；流程从该人群组分配的起始投放项进入。',
  rules: [['受众类型', selection.type === 'bundle' ? '实验人群包' : '普通人群组'], ['稳定 ID', selection.type === 'bundle' ? selection.bundleId : selection.groupId], ['活动内数量', '一个']],
 } : null;
 if (audienceNode) nodes.push(audienceNode);
 const routeStep = Math.min(185, 420 / Math.max(1, entries.length - 1));
 const entryNodes = new Map();
 if (selection?.type === 'bundle') entries.forEach((entry, index) => {
  const ref = selectedRefs.find(item => item.groupId === entry.groupId);
  const label = ref?.armName || ref?.name || entry.groupId;
  const node = { id: `entry:${entry.groupId}`, name: entry.kind === 'control' ? `${label}（对照）` : label, kind: 'entry', x: -220,
   z: (index - (entries.length - 1) / 2) * routeStep, w: 92, d: 72, h: 22,
   desc: `实验包入口 · ${label}。${entry.kind === 'control' ? '对照路径不绑定投放项。' : '从这里进入指定的起始投放项。'}`,
   rules: [['入口人群组', entry.groupId], ['入口类型', entry.kind === 'control' ? '对照臂' : '起始路径'], ['起始投放项', entry.treatmentIds.join('、') || '未配置']], entry,
  };
  entryNodes.set(entry.groupId, node); nodes.push(node);
 });
 const roots = treatments.filter(item => entries.some(entry => entry.kind === 'start' && entry.treatmentIds.includes(item.id)) && !incoming.has(item.id));
 const level = new Map(roots.map(item => [item.id, 0]));
 if (!hasCycle(treatments, edges)) {
  for (let pass = 0; pass < treatments.length; pass++) {
   let changed = false;
   for (const edge of edges) {
    if (!level.has(edge.fromTreatmentId)) continue;
    const target = level.get(edge.fromTreatmentId) + 1;
    if (!level.has(edge.toTreatmentId) || level.get(edge.toTreatmentId) < target) { level.set(edge.toTreatmentId, target); changed = true; }
   }
   if (!changed) break;
  }
 }
 const maxLevel = Math.max(0, ...level.values());
 const treatmentColumns = new Map();
 for (const treatment of treatments) {
  const col = level.get(treatment.id) ?? Math.min(maxLevel + 1, treatments.findIndex(item => item.id === treatment.id));
  if (!treatmentColumns.has(col)) treatmentColumns.set(col, []);
  treatmentColumns.get(col).push(treatment);
 }
 const maxColumn = Math.max(0, ...treatmentColumns.keys());
 for (const [col, items] of treatmentColumns) {
  const x = -145 + (maxColumn ? col / maxColumn : 0) * 510;
  const zStep = Math.min(190, 440 / Math.max(1, items.length - 1));
  items.forEach((treatment, index) => {
   const action = actionById.get(treatment.actionId);
   const node = {
    id: treatment.id, name: treatment.name || treatment.id, kind: 'treatment',
    x, z: (index - (items.length - 1) / 2) * zStep, w: 72, d: 62, h: 66,
    desc: `活动投放项 · ${action?.name || treatment.actionId || '未选择动作'}。画面中的人员由本地演练脚本驱动，不会真实触达。`,
    rules: [['投放项 ID', treatment.id], ['动作', action?.name || treatment.actionId], ['入口配置', entries.some(entry => entry.treatmentIds.includes(treatment.id)) ? '活动流程入口' : '回执后续步骤']],
    actionName: action?.name || treatment.actionId, treatment,
   };
   nodes.push(node);
  });
 }
 const terminal = { id: 'flow-terminal', name: '演练结束', kind: 'terminal', x: 455, z: 0, w: 94, d: 82, h: 30,
  desc: '当前活动流程在这里结束。只显示构造样本的本地路径，不表示真实发送或业务效果。',
  rules: [['活动', activity?.name || activity?.id], ['渠道连接', '无'], ['持久化', 'API 进程内状态']],
 };
 const hasTerminal = treatments.length > 0 || entries.some(entry => entry.kind === 'control');
 if (hasTerminal) nodes.push(terminal);
 const byId = new Map(nodes.map(node => [node.id, node]));
 const connections = [];
 if (audienceNode && selection?.type === 'bundle') {
  for (const entry of entries) {
   const routeNode = entryNodes.get(entry.groupId);
   if (routeNode) connections.push({ id: `split:${entry.groupId}`, from: audienceNode.id, to: routeNode.id, kind: 'split', label: routeNode.name });
   if (entry.kind === 'control' && routeNode && byId.has(terminal.id)) connections.push({ id: `control:${entry.groupId}`, from: routeNode.id, to: terminal.id, kind: 'control', label: '对照路径' });
   for (const id of entry.treatmentIds) if (routeNode && byId.has(id)) connections.push({ id: `entry:${entry.groupId}:${id}`, from: routeNode.id, to: id, kind: 'entry', label: '起始投放项' });
  }
 } else if (audienceNode) {
  for (const entry of entries) for (const id of entry.treatmentIds) {
   if (byId.has(id)) connections.push({ id: `entry:${entry.groupId}:${id}`, from: audienceNode.id, to: id, kind: 'entry', label: '起始投放项' });
  }
 }
 for (const edge of edges) {
  if (byId.has(edge.fromTreatmentId) && byId.has(edge.toTreatmentId)) connections.push({ id: edge.id, from: edge.fromTreatmentId, to: edge.toTreatmentId, kind: 'receipt', label: edge.conditionLabel, edge });
 }
 for (const treatment of treatments) {
  const action = actionById.get(treatment.actionId);
  const declared = Array.isArray(action?.receiptOutcomes) ? action.receiptOutcomes : [];
  const configured = new Set(edges.filter(edge => edge.fromTreatmentId === treatment.id).map(edge => edge.conditionLabel));
  const endings = declared.filter(outcome => !configured.has(outcome));
  if (!declared.length && !edges.some(edge => edge.fromTreatmentId === treatment.id)) endings.push('动作结束');
  if (!endings.length && !edges.some(edge => edge.fromTreatmentId === treatment.id)) endings.push('未配置下一步');
  for (const outcome of endings) if (hasTerminal) connections.push({ id: `complete:${treatment.id}:${outcome}`, from: treatment.id, to: terminal.id, kind: 'complete', label: outcome });
 }
 const cyclic = hasCycle(treatments, edges);
 const readinessIssue = rehearsalReadiness(activity, treatments, entries, edges, selectedRefs, cyclic);
 return { nodes, byId, connections, roots, entries, entryNodes, audienceNode, treatments, edges, actionById, hasCycle: cyclic, canRehearse: !readinessIssue, readinessIssue, empty: treatments.length === 0, maxColumn };
}

export function buildEntryRoutes(routeRefs, treatmentIdsByGroup) {
 return routeRefs.map(ref => {
  const control = ref.type === 'arm' && ref.part === 'control';
  return {
   groupId: ref.groupId,
   kind: control ? 'control' : 'start',
   treatmentIds: control ? [] : [...(treatmentIdsByGroup.get(ref.groupId) || [])],
  };
 });
}

function markup(activity, references, hasDraft = false) {
 const route = activityRoute(activity.id, 'campaign');
 return `<section class="activity-flow-canvas" aria-label="活动流程 3D 画布">
 <div class="flow-canvas-heading"><div><p class="eyebrow">ACTIVITY FLOW / LOCAL REHEARSAL</p><h3>活动流程</h3><p>活动受众只选一次；流程实验可在到达节点时独立分组，再沿分组路径或回执边推进。</p></div><div class="flow-canvas-actions"><a class="primary-link" href="#${route}/treatments/new">＋ 添加投放项</a><a class="primary-link flow-audience-link" href="#${assetRoute('audience', route)}">管理受众</a></div></div>
 <p class="flow-rehearsal-warning" role="note">本地演练 · 只使用构造样本和预设回执，不会触达真实用户或真实渠道。</p>
 <div class="flow-audience-form flow-entry-form" hidden><form id="activity-entry-form"><strong>入口路由</strong><div data-entry-route-fields></div><button class="primary" type="submit">保存入口路由</button><button type="button" data-entry-cancel>取消</button><p class="inline-error" role="alert"></p></form></div>
 <div class="flow-edge-form" hidden><form id="activity-flow-form"><p class="flow-edge-form-title" data-flow-form-title>添加回执边</p><label>起点投放项<select name="fromTreatmentId"></select></label><label>回执条件<select name="conditionLabel"></select></label><label>后续投放项<select name="toTreatmentId"></select></label><button class="primary" type="submit">添加回执边</button><button type="button" data-flow-cancel>取消</button><p class="inline-error" role="alert"></p></form></div>
 <div class="flow-edge-toolbar"><button type="button" data-flow-config-entry>配置入口投放项</button><button type="button" data-flow-add-edge>＋ 添加回执边</button><span class="flow-save-status" role="status" aria-live="polite"></span></div>
 <section class="metrics" aria-label="本地演练统计"><div class="metric"><span>构造路径</span><strong data-flow-total>0</strong><small>本地虚构样本</small></div><div class="metric"><span>等待进入</span><strong data-flow-queued>0</strong><small>尚未开始</small></div><div class="metric"><span>演练中</span><strong data-flow-active>0</strong><small>沿当前图形流转</small></div><div class="metric"><span>已完成</span><strong data-flow-complete>0</strong><small>抵达流程终点</small></div><div class="metric"><span>已走回执边</span><strong data-flow-edges>0</strong><small>本地脚本统计</small></div><div class="balance"><span>状态<strong data-flow-state>已暂停</strong></span></div></section>
 <section class="workspace flow-canvas-workspace">
  <aside class="left-panel"><div class="panel-label"><span>流程节点</span><span data-node-total>0</span></div><div class="node-list" data-node-list aria-label="选择流程节点"></div><div class="left-note"><span class="tiny-heading">本地活动演练</span><p>${hasDraft ? '图形合并活动 API 流程与此浏览器实验草稿。' : '图形读取当前活动 API 状态。'}播放时按动作回执条件轮流演示路径，不代表真实分布概率。</p><span class="version">SCENE MODEL / V1.0</span></div></aside>
  <div class="world-wrap"><div class="world-toolbar"><div class="world-title"><span class="live-dot"></span><span data-play-state>已暂停</span><span class="divider">/</span><span>活动流程立体画布</span></div><div class="view-controls" aria-label="视角控制"><button type="button" data-view="iso" class="active" aria-pressed="true">立体</button><button type="button" data-view="top" aria-pressed="false">俯视</button><button type="button" data-zoom="out" aria-label="缩小">−</button><button type="button" data-zoom="in" aria-label="放大">+</button><button type="button" data-view="reset">复位</button></div></div>
   <div class="world" data-world><svg data-scene viewBox="0 0 1100 680" role="img" aria-label="当前活动的立体流程图"><defs><pattern data-grid width="40" height="40" patternUnits="userSpaceOnUse"><path d="M40 0H0V40" fill="none" stroke="#e4e0d8" stroke-width=".6"/></pattern></defs><g data-ground></g><g data-routes></g><g data-objects></g><g data-labels></g></svg></div>
   <div class="world-foot"><span><span class="legend-dot"></span>构造样本 <span class="legend-line"></span>活动流程边 <span class="legend-dash"></span>失败回执条件</span><span>拖动旋转 · 点选节点或边</span></div><div class="scenario-note"><span>场景说明</span><p data-flow-note>图形来自当前活动；活动流程边决定回执后续步骤。</p></div>
  </div>
  <aside class="detail-panel" aria-label="节点检查器"><div class="panel-label"><span data-detail-kind>ACTIVITY FLOW</span><span data-detail-index>01</span></div><div class="detail-icon" data-detail-icon></div><p class="detail-kind" data-detail-type>ACTIVITY FLOW NODE</p><h2 data-detail-title>活动流程</h2><p class="detail-desc" data-detail-desc></p><div class="detail-stats"><div><span>当前到达</span><strong data-detail-arrived>0</strong></div><div><span>当前执行</span><strong data-detail-active>0</strong></div></div><div class="mini-track"><span data-detail-progress></span></div><div class="detail-rules" data-detail-rules></div><div data-edge-detail></div><div class="detail-bottom"><span>本地演练</span><p>成功回执只表示模拟动作完成，不代表用户阅读、下单或真实业务效果。</p></div></aside>
 </section>
 <section class="transport" aria-label="本地演练控制"><div class="play-controls"><button type="button" class="primary" data-flow-play>开始本地演练</button><button type="button" data-flow-reset>重置</button></div><div class="timeline"><div><span>演练时间 <strong data-flow-time>00:00.0</strong></span><span data-flow-duration>00:12.0</span></div><input type="range" min="0" max="12" step="any" value="0" data-flow-timeline aria-label="演练时间位置"></div><label class="speed">速度<select data-flow-speed><option value="0.5">0.5×</option><option value="1" selected>1×</option><option value="2">2×</option><option value="4">4×</option></select></label><span class="motion-note">SVG 立体投影<br>旧版沙盘实现</span></section>
 </section>`;
}

export function mountActivityFlowCanvas({ root, activity, actions = [], references = [], api, onSaved = () => {}, onExperimentAction = () => {}, experimentStore = null, draftStore = null }) {
 if (!root || !activity) return () => {};
 root.innerHTML = markup(activity, references, Boolean(draftStore?.nodes(activity.id).length));
 const q = selector => root.querySelector(selector);
 const currentExperiments = () => experimentStore.currentExperiments?.() ?? experimentStore.experiments();
 const graph = draftStore ? composeFlowExperimentGraph(activity, actions, references, currentExperiments(), draftStore.nodes(activity.id)) : buildActivityGraph(activity, actions, references);
 let selected = graph.nodes[0]?.id || null, selectedEdge = null, editingEdgeId = null, savingFlow = false;
 let yaw = -.28, pitch = .55, zoom = 1, playing = false, time = 0, speed = 1, frame = 0, lastFrame = 0;
 let dragging = false, dragStart = null, moved = false, pressed = null;
 const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;

 const project = (x, y, z) => { const rx = x * Math.cos(yaw) - z * Math.sin(yaw), rz = x * Math.sin(yaw) + z * Math.cos(yaw); return [550 + rx * zoom, 345 + (rz * pitch - y * Math.sqrt(1 - pitch * pitch)) * zoom]; };
 const point = p => p.map(value => value.toFixed(1)).join(',');
 const points = vertices => vertices.map(vertex => point(project(...vertex))).join(' ');
 const poly = (vertices, fill, stroke = colors.line, width = .8) => `<polygon points="${points(vertices)}" fill="${fill}" stroke="${stroke}" stroke-width="${width}"/>`;
 const depth = (x, z) => x * Math.sin(yaw) + z * Math.cos(yaw);
 const line = (vertices, stroke, width = 1, dash = '') => `<polyline points="${points(vertices)}" fill="none" stroke="${stroke}" stroke-width="${width}" stroke-linecap="round" stroke-linejoin="round" ${dash ? `stroke-dasharray="${dash}"` : ''}/>`;
 function cube(x, z, w, d, h, bottom = 0, accent = false) {
  const a = [x - w / 2, bottom, z - d / 2], b = [x + w / 2, bottom, z - d / 2], c = [x + w / 2, bottom, z + d / 2], e = [x - w / 2, bottom, z + d / 2];
  const top = vertex => [vertex[0], bottom + h, vertex[2]];
  const faces = [[a,b,top(b),top(a)],[b,c,top(c),top(b)],[c,e,top(e),top(c)],[e,a,top(a),top(e)]];
  faces.sort((first, second) => depth(first.reduce((sum, vertex) => sum + vertex[0], 0) / 4, first.reduce((sum, vertex) => sum + vertex[2], 0) / 4) - depth(second.reduce((sum, vertex) => sum + vertex[0], 0) / 4, second.reduce((sum, vertex) => sum + vertex[2], 0) / 4));
  return faces.map((face, index) => poly(face, accent ? (index % 2 ? '#2c4a7c' : '#1f365b') : (index % 2 ? colors.side : '#ece8df'), accent ? colors.blue : colors.line, .7)).join('') + poly([top(a), top(b), top(c), top(e)], accent ? colors.blue : colors.white, accent ? colors.blue : colors.line);
 }
 function segment(from, to) {
  const a = graph.byId.get(from), b = graph.byId.get(to);
  if (!a || !b) return [];
  const mid = (a.x + b.x) / 2;
  return [[a.x, 17, a.z], [mid, 17, a.z], [mid, 17, b.z], [b.x, 17, b.z]];
 }
 function nodeIcon(node) {
  if (node.kind === 'audience') return cube(node.x - 54, node.z - 54, 6, 8, 30, 15) + cube(node.x + 54, node.z - 54, 6, 8, 30, 15) + cube(node.x, node.z - 54, 114, 8, 8, 45, true);
  if (node.kind === 'entry') return cube(node.x, node.z, 34, 28, 28, 15, true);
  if (node.kind === 'experiment') return cube(node.x, node.z, 58, 58, 14, 15, true) + cube(node.x, node.z, 32, 32, 23, 29, true);
  if (node.kind === 'terminal') return [0,1,2].map(index => cube(node.x - 22 + index * 22, node.z, 18, 34, 16 + index * 8, 15, true)).join('');
  return cube(node.x, node.z, 44, 38, 52, 15) + cube(node.x, node.z, 48, 42, 6, 67, true);
 }
 function renderNode(node) {
  const symbol = node.kind === 'treatment' ? esc((node.actionName || node.name).slice(0, 1).toUpperCase()) : node.kind === 'terminal' ? '✓' : node.kind === 'experiment' ? '实' : node.kind === 'entry' ? '分' : '人';
  let svg = cube(node.x, node.z, node.w + 16, node.d + 16, 8, 0) + cube(node.x, node.z, node.w, node.d, 7, 8) + nodeIcon(node);
  const [sx, sy] = project(node.x, node.kind === 'treatment' ? 52 : 49, node.z);
  svg += `<text x="${sx.toFixed(1)}" y="${sy.toFixed(1)}" text-anchor="middle" font-family="Georgia,serif" font-size="22" fill="${colors.blue}">${symbol}</text>`;
  if (node.id === selected) svg += line([[node.x - node.w / 2 - 12, 9, node.z - node.d / 2 - 12], [node.x + node.w / 2 + 12, 9, node.z - node.d / 2 - 12], [node.x + node.w / 2 + 12, 9, node.z + node.d / 2 + 12], [node.x - node.w / 2 - 12, 9, node.z + node.d / 2 + 12], [node.x - node.w / 2 - 12, 9, node.z - node.d / 2 - 12]], colors.blue, 2);
  return { z: depth(node.x, node.z), svg: `<g class="scene-node" data-flow-node="${esc(node.id)}" aria-label="${esc(node.name)}"><title>${esc(node.name)}：点击查看详情</title>${svg}</g>` };
 }
 function renderGround() {
  return cube(0, 0, 1040, 680, 9, -9) + `<polygon points="${points([[-520,.2,-340],[520,.2,-340],[520,.2,340],[-520,.2,340]])}" fill="${colors.paper}" stroke="${colors.line}" stroke-width=".8"/>` + Array.from({ length: 18 }, (_, index) => line([[-500,1,-320 + index * 38],[500,1,-320 + index * 38]], colors.line, .35)).join('') + Array.from({ length: 27 }, (_, index) => line([[-500 + index * 38,1,-320],[-500 + index * 38,1,320]], colors.line, .35)).join('');
 }
 function renderConnection(connection) {
  const path = segment(connection.from, connection.to);
  if (!path.length) return '';
  if (connection.kind === 'experiment') {
   const offset = connection.part === 'a' ? -26 : connection.part === 'control' ? 26 : 0;
   path[1] = [path[1][0], path[1][1], path[1][2] + offset];
   path[2] = [path[2][0], path[2][1], path[2][2] + offset];
  }
  const failure = normalizedOutcome(connection.label) === 'failure';
  const color = failure ? colors.ink : colors.blue;
  let output = line(path, colors.white, 9) + line(path, color, 1.25, failure ? '4 5' : '');
  if (connection.kind === 'receipt' || connection.kind === 'split' || connection.kind === 'control' || connection.kind === 'experiment') {
   const [x, y] = connection.kind === 'experiment' ? project((path[1][0] + path[2][0]) / 2, 20, (path[1][2] + path[2][2]) / 2)
    : project((graph.byId.get(connection.from).x + graph.byId.get(connection.to).x) / 2, 20, (graph.byId.get(connection.from).z + graph.byId.get(connection.to).z) / 2);
   const width = Math.max(56, Math.min(140, connection.label.length * 9 + 16));
   const clickable = connection.kind === 'receipt' ? ` data-flow-edge="${esc(connection.edge?.id || connection.id)}" tabindex="0" role="button" aria-label="回执边：${esc(connection.label)}"` : '';
   output += `<g class="scene-node"${clickable}><rect class="node-label-back node-outline" x="${(x - width / 2).toFixed(1)}" y="${(y - 17).toFixed(1)}" width="${width}" height="20" rx="3"/><text class="scene-sub" text-anchor="middle" x="${x.toFixed(1)}" y="${(y - 3).toFixed(1)}">${esc(connection.label)}</text></g>`;
  }
  return output;
 }
 function renderLabels() {
  return graph.nodes.map(node => {
   const [x, y] = node.kind === 'treatment' ? project(node.x, 0, node.z + node.d / 2 + 20) : project(node.x, 0, node.z + node.d / 2 + 20);
   const width = Math.max(88, Math.min(150, node.name.length * 9 + 20));
   const sub = node.kind === 'treatment' ? node.actionName : node.kind === 'audience' ? '活动唯一受众' : node.kind === 'experiment' ? '流程实验 · 本地草稿' : node.kind === 'entry' ? '实验臂入口' : '当前流程终点';
   return `<g class="scene-node ${node.id === selected ? 'selected-label' : ''}" data-flow-node="${esc(node.id)}" transform="translate(${x.toFixed(1)},${y.toFixed(1)})"><rect class="node-label-back node-outline" x="${(-width / 2).toFixed(1)}" y="-2" width="${width}" height="38" rx="3"/><text class="scene-label" text-anchor="middle" y="13">${esc(node.name)}</text><text class="scene-sub" text-anchor="middle" y="28">${esc(sub || '')}</text></g>`;
  }).join('');
 }
 function renderScene() {
  q('[data-ground]').innerHTML = renderGround();
  q('[data-routes]').innerHTML = graph.connections.map(renderConnection).join('');
  const figures = playing && !reduced ? buildFigures(time) : [];
  q('[data-objects]').innerHTML = [...graph.nodes.map(renderNode), ...figures].sort((a,b) => a.z - b.z).map(item => item.svg).join('');
  q('[data-labels]').innerHTML = renderLabels();
 }
 function buildRuns() {
  const runs = [];
  const out = new Map(graph.treatments.map(item => [item.id, graph.edges.filter(edge => edge.fromTreatmentId === item.id)]));
  const terminalId = graph.byId.has('flow-terminal') ? 'flow-terminal' : null;
  const expand = (current, user, path, trail) => {
   if (trail.has(current) || path.length > graph.treatments.length + 2) { runs.push({ user, path: [...path, terminalId].filter(Boolean) }); return; }
   const receiptOutcomes = graph.actionById.get(graph.treatments.find(item => item.id === current)?.actionId)?.receiptOutcomes;
   const declared = Array.isArray(receiptOutcomes) ? receiptOutcomes : [];
   const conditions = [...new Set(declared)];
   const condition = conditions.length ? conditions[user % conditions.length] : '';
   const branches = (out.get(current) || []).filter(edge => edge.conditionLabel === condition);
   if (!branches.length) { runs.push({ user, path: terminalId ? [...path, terminalId] : path }); return; }
   const nextTrail = new Set(trail); nextTrail.add(current);
   for (const edge of branches) expand(edge.toTreatmentId, user, [...path, edge.toTreatmentId], nextTrail);
  };
  for (const entry of graph.entries) {
   const prefix = graph.audienceNode ? [graph.audienceNode.id] : [];
   const routeNode = graph.entryNodes.get(entry.groupId);
   if (routeNode) prefix.push(routeNode.id);
   if (entry.kind === 'control') {
    if (terminalId) for (let user = 0; user < 12; user++) runs.push({ user, path: [...prefix, terminalId] });
    continue;
   }
   for (const treatmentId of entry.treatmentIds) {
    for (let user = 0; user < 12; user++) expand(treatmentId, user, [...prefix, treatmentId], new Set());
   }
  }
  return runs;
 }
 let runs = graph.draftNodes?.length ? [] : buildRuns();
 let visits = new Map();
 let duration = Math.max(8, Math.min(30, Math.max(...runs.map(run => run.path.length), 1) * 2.2));
 function pathPosition(vertices, progress) {
  const distances = vertices.slice(1).map((vertex, index) => Math.hypot(...vertex.map((value, axis) => value - vertices[index][axis])));
  let remaining = progress * distances.reduce((sum, distance) => sum + distance, 0);
  for (let index = 0; index < distances.length; index++) {
   if (remaining <= distances[index] || index === distances.length - 1) { const ratio = Math.min(1, remaining / (distances[index] || 1)); return vertices[index].map((value, axis) => value + (vertices[index + 1][axis] - value) * ratio); }
   remaining -= distances[index];
  }
  return vertices.at(-1);
 }
 function positionOnRun(run, now) {
  const offset = (run.user % 6) * .08;
  const elapsed = Math.max(0, now - offset);
  const total = Math.max(.001, run.path.length - 1);
  const step = Math.min(total - .0001, elapsed / duration * total);
  const index = Math.floor(step), progress = step - index;
  const from = run.path[index], to = run.path[Math.min(index + 1, run.path.length - 1)];
  const path = segment(from, to);
  return path.length ? pathPosition(path, progress) : [0, 20, 0];
 }
 function buildFigures(now) {
  return runs.map(run => {
   const pos = positionOnRun(run, now), [x,y] = project(...pos);
   return { z: depth(pos[0], pos[2]) + 4, svg: `<g transform="translate(${x.toFixed(1)} ${y.toFixed(1)}) scale(${zoom * .78})" aria-hidden="true"><ellipse cy="1" rx="4.8" ry="2" fill="#1c1a17" opacity=".12"/><path d="M-2-2v-5q2-2 4 0v5M-1-2v3M1-2v3" fill="${run.user % 2 ? colors.ink : colors.blue}" stroke="${run.user % 2 ? colors.ink : colors.blue}" stroke-width="1.8" stroke-linecap="round"/><circle cy="-11" r="2.5" fill="${run.user % 2 ? colors.ink : colors.blue}"/></g>` };
  });
 }
 function renderNodeList() {
  q('[data-node-list]').innerHTML = graph.nodes.map(node => `<button type="button" class="node-button ${node.id === selected ? 'active' : ''}" data-select-node="${esc(node.id)}" aria-pressed="${node.id === selected}"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 6h16v12H4zM8 10h8m-8 4h5"/></svg><span>${esc(node.name)}</span><span class="count">${playing ? '·' : ''}</span></button>`).join('');
  q('[data-node-total]').textContent = String(graph.nodes.length);
 }
 function renderDetail() {
  const node = graph.byId.get(selected);
  if (!node) return;
  q('[data-detail-title]').textContent = node.name;
  q('[data-detail-desc]').textContent = node.desc;
  q('[data-detail-kind]').textContent = graph.byId.has(node.id) ? `${String(node.kind).toUpperCase()} · ${activity.id}` : 'ACTIVITY FLOW';
  q('[data-detail-index]').textContent = String(graph.nodes.indexOf(node) + 1).padStart(2, '0');
  q('[data-detail-type]').textContent = node.kind === 'treatment' ? 'TREATMENT NODE' : node.kind === 'experiment' ? 'FLOW EXPERIMENT' : node.kind === 'audience' ? 'ACTIVITY AUDIENCE' : node.kind === 'entry' ? 'EXPERIMENT ARM ROUTE' : 'FLOW TERMINAL';
  q('[data-detail-icon]').innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v14H4zM8 9h8m-8 4h8m-8 4h4"/></svg>';
  const visit = node.kind === 'experiment' ? visits.get(node.draft.id) : null;
  q('[data-detail-rules]').innerHTML = (node.rules || []).map(([label,value]) => `<div><span>${esc(label)}</span><strong>${esc(value)}</strong></div>`).join('')
   + (visit ? `<div><span>分组人数</span><strong>${visit.people.size}</strong></div><div><span>路径到达次数</span><strong>${visit.arrivals}</strong></div>${['a','b','control'].map(part => `<div><span>${part === 'control' ? '对照组' : part.toUpperCase() + ' 组'}</span><strong>${visit.groups[part].size}</strong></div>`).join('')}` : '');
  const at = graph.treatments.findIndex(item => item.id === node.id);
  const treatment = at >= 0 ? graph.treatments[at] : null;
  const edit = treatment ? `<a class="primary-link" href="#${activityRoute(activity.id, 'campaign')}/treatments/${encodeURIComponent(treatment.id)}/edit">编辑此投放项</a>` : '';
  const edges = selectedEdge ? graph.edges.filter(edge => edge.id === selectedEdge) : [];
  const edgeMarkup = edges.map(edge => `<div class="flow-selected-edge"><p>活动流程边 · ${esc(edge.conditionLabel)}</p><p>${esc(graph.byId.get(edge.fromTreatmentId)?.name)} → ${esc(graph.byId.get(edge.toTreatmentId)?.name)}</p><div class="flow-edge-actions"><button type="button" data-edit-edge="${esc(edge.id)}">编辑这条边</button><button type="button" data-delete-edge="${esc(edge.id)}">删除这条边</button></div></div>`).join('');
  let experimentMarkup = '';
  if (node.kind === 'experiment') {
   const route = part => graph.byId.get(node.draft.routes[part])?.name || node.draft.routes[part];
   let preview = '引用不可用，暂时不能预测分组。';
   try {
    preview = MOCK_USERS.slice(0, 6).map(user => `${user.uid} → ${({ a: 'A 组', b: 'B 组', control: '对照组' })[draftStore.group(activity.id, node.draft, user.uid, currentExperiments())]}`).join('；');
   } catch { /* Invalid references are reported by graph validation. */ }
   experimentMarkup = `<div class="flow-selected-edge"><p>A 组 → ${esc(route('a'))}</p><p>B 组 → ${esc(route('b'))}</p><p>对照组 → ${esc(route('control'))}</p><p>构造样本只读预测：${esc(preview)}</p><div class="flow-edge-actions"><button type="button" data-edit-experiment="${esc(node.draft.id)}">编辑此实验节点</button><button type="button" data-remove-experiment="${esc(node.draft.id)}">移除此实验节点</button></div></div>`;
  }
  q('[data-edge-detail]').innerHTML = `${edit}${edgeMarkup}${experimentMarkup}`;
  q('[data-detail-arrived]').textContent = visit ? String(visit.people.size) : playing ? String(Math.min(runs.length, Math.floor(time / duration * runs.length))) : '0';
  q('[data-detail-active]').textContent = playing && time < duration ? String(runs.length) : '0';
  q('[data-detail-progress]').style.width = `${playing ? Math.min(100, time / duration * 100) : 0}%`;
 }
 function updateMetrics() {
  const progress = duration ? Math.min(1, time / duration) : 0;
  q('[data-flow-total]').textContent = String(runs.length);
  q('[data-flow-queued]').textContent = String(Math.floor(runs.length * (1 - progress)));
  q('[data-flow-active]').textContent = playing && progress < 1 ? String(runs.length) : '0';
  q('[data-flow-complete]').textContent = String(Math.floor(runs.length * progress));
  const traversedReceipts = graph.draftNodes?.length ? runs.reduce((sum, run) => sum + run.path.slice(1).filter((id, index) => graph.connections.some(edge => edge.kind === 'receipt' && edge.from === run.path[index] && edge.to === id)).length, 0) : graph.edges.length;
  q('[data-flow-edges]').textContent = String(Math.floor(traversedReceipts * progress));
  q('[data-flow-state]').textContent = graph.hasCycle ? '配置含环' : playing ? progress >= 1 ? '演练完成' : '演练中' : '已暂停';
  q('[data-play-state]').textContent = playing ? '本地演练中' : progress >= 1 ? '演练结束' : '已暂停';
  q('[data-flow-play]').textContent = graph.hasCycle ? '请先修正循环边' : !graph.canRehearse ? '补全流程后演练' : playing ? '暂停演练' : progress >= 1 ? '重新演练' : '开始本地演练';
  q('[data-flow-play]').disabled = (!graph.canRehearse || draftStore && !draftStore.canUse) && !playing;
  q('[data-flow-time]').textContent = `${String(Math.floor(time / 60)).padStart(2,'0')}:${(time % 60).toFixed(1).padStart(4,'0')}`;
  q('[data-flow-duration]').textContent = `${String(Math.floor(duration / 60)).padStart(2,'0')}:${(duration % 60).toFixed(1).padStart(4,'0')}`;
  q('[data-flow-timeline]').max = String(duration); q('[data-flow-timeline]').value = String(time);
 }
 function render() { renderNodeList(); renderDetail(); updateMetrics(); renderScene(); }
 function selectNode(id) { if (!graph.byId.has(id)) return; selected = id; selectedEdge = null; render(); }
 function stop() { playing = false; if (frame) cancelAnimationFrame(frame); frame = 0; }
 function tick(now) {
  if (!playing) return;
  if (lastFrame) time = Math.min(duration, time + Math.max(0, now - lastFrame) / 1000 * speed);
  lastFrame = now;
  if (time >= duration) stop();
  render();
  if (playing) frame = requestAnimationFrame(tick);
 }
 function setView(mode) {
  pitch = mode === 'top' ? .96 : .55; yaw = mode === 'top' ? 0 : -.28; zoom = 1;
  root.querySelectorAll('[data-view="iso"],[data-view="top"]').forEach(button => { const active = button.dataset.view === (mode === 'top' ? 'top' : 'iso'); button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active)); });
  renderScene();
 }
 function edgeForm(open, edge = null) {
  const form = q('.flow-edge-form'); form.hidden = !open;
  if (!open) { editingEdgeId = null; return; }
  const from = form.querySelector('[name="fromTreatmentId"]'), to = form.querySelector('[name="toTreatmentId"]'), condition = form.querySelector('[name="conditionLabel"]'), submit = form.querySelector('[type="submit"]');
  editingEdgeId = edge?.id || null;
  q('[data-flow-form-title]').textContent = editingEdgeId ? '编辑回执边' : '添加回执边';
  submit.textContent = editingEdgeId ? '保存修改' : '添加回执边';
  from.innerHTML = graph.treatments.map(item => `<option value="${esc(item.id)}">${esc(item.name)}</option>`).join('');
  to.innerHTML = from.innerHTML;
  if (edge) { from.value = edge.fromTreatmentId; to.value = edge.toTreatmentId; }
  const fillConditions = () => {
   const source = graph.treatments.find(item => item.id === from.value);
   const sourceAction = graph.actionById.get(source?.actionId);
   const outcomes = Array.isArray(sourceAction?.receiptOutcomes) ? sourceAction.receiptOutcomes : [];
   condition.innerHTML = outcomes.length ? outcomes.map(item => `<option value="${esc(item)}">${esc(item)}</option>`).join('') : '<option value="">此动作未声明回执结果</option>';
   submit.disabled = outcomes.length === 0;
  };
  fillConditions();
  if (edge) condition.value = edge.conditionLabel;
  from.onchange = fillConditions;
 }
 async function saveFlow(nextEntries, nextEdges) {
  if (savingFlow) return false;
  savingFlow = true;
  const status = q('.flow-save-status');
  const controls = [...root.querySelectorAll('[data-flow-config-entry], [data-flow-add-edge], #activity-entry-form button, #activity-entry-form select, #activity-flow-form button, #activity-flow-form select, [data-edge-detail] [data-edit-edge], [data-edge-detail] [data-delete-edge]')];
  const priorDisabled = controls.map(control => control.disabled);
  controls.forEach(control => { control.disabled = true; });
  q('#activity-entry-form')?.setAttribute('aria-busy', 'true');
  q('#activity-flow-form')?.setAttribute('aria-busy', 'true');
  status.textContent = '正在保存…';
  try {
   await api.updateFlow(activity.id, { id: activity.activityFlow?.id || `flow-${activity.id}`, entries: nextEntries, edges: nextEdges }, graph.entries, graph.edges);
   status.textContent = '活动入口和回执边已保存到当前管理 API 进程。';
   await onSaved();
   return true;
  } catch (error) { status.textContent = `保存失败：${error.message}`; return false; }
  finally {
   savingFlow = false;
   q('#activity-entry-form')?.removeAttribute('aria-busy');
   q('#activity-flow-form')?.removeAttribute('aria-busy');
   controls.forEach((control, index) => { if (control.isConnected) control.disabled = priorDisabled[index]; });
  }
 }

 function entryFormMarkup() {
  const form = q('#activity-entry-form');
  const target = form.querySelector('[data-entry-route-fields]');
  if (!activity.audience) {
   target.innerHTML = '<p class="campaign-note">先在「编辑活动」中选择一个受众，再回来分配入口投放项。</p>';
   return;
  }
  const routeRefs = activity.audience.type === 'group'
   ? references.filter(item => item.type === 'group' && item.groupId === activity.audience.groupId)
   : references.filter(item => item.type === 'arm' && item.bundleId === activity.audience.bundleId);
  if (!routeRefs.length) {
   target.innerHTML = '<p class="campaign-note">当前活动受众不可用，请从受众或实验页面修复引用。</p>';
   return;
  }
  target.innerHTML = routeRefs.map(ref => {
   const current = graph.entries.find(entry => entry.groupId === ref.groupId);
   const isControl = ref.type === 'arm' && ref.part === 'control';
   const title = ref.type === 'arm' ? `${ref.bundleName} / ${ref.armName}` : ref.groupName;
   if (isControl) return `<div class="flow-entry-row"><strong>${esc(title)}</strong><span>对照臂 · 不绑定起始投放项</span></div>`;
   const treatmentOptions = graph.treatments.map(item => `<option value="${esc(item.id)}" ${current?.treatmentIds?.includes(item.id) ? 'selected' : ''}>${esc(item.name)} · ${esc(item.id)}</option>`).join('');
   return `<label class="flow-entry-row"><span>${esc(title)} <small>· ${esc(ref.groupId)} · 可多选并行起始投放项</small></span><select multiple size="4" data-entry-group="${esc(ref.groupId)}">${treatmentOptions}</select></label>`;
  }).join('');
 }

 q('[data-flow-play]').addEventListener('click', () => {
  if (!graph.canRehearse || draftStore && !draftStore.canUse) return;
  if (playing) stop();
  else {
   if (graph.draftNodes?.length) {
    try {
     const definitions = currentExperiments();
     const latestGraph = composeFlowExperimentGraph(activity, actions, references, definitions, draftStore.nodes(activity.id));
     if (!latestGraph.canRehearse) {
      q('[data-flow-note]').textContent = latestGraph.readinessIssue || '实验引用不可用，请修复后演练。';
      q('.flow-save-status').textContent = q('[data-flow-note]').textContent;
      return;
     }
     const bundle = activity.audience?.type === 'bundle' ? experimentStore.bundles().find(item => item.id === activity.audience.bundleId) : null;
     const result = simulateFlowRuns(latestGraph, MOCK_USERS.slice(0, 12),
      (node, uid) => draftStore.group(activity.id, node, uid, definitions, true),
      (groupId, uid) => !bundle || experimentStore.peekExperimentGroup(bundle.experimentId, uid) === bundle.arms.find(arm => arm.groupId === groupId)?.part);
     runs = result.runs; visits = result.visits;
     duration = Math.max(8, Math.min(30, Math.max(...runs.map(run => run.path.length), 1) * 2.2));
     if (draftStore.warning) { q('[data-flow-note]').textContent = draftStore.warning; q('.flow-save-status').textContent = draftStore.warning; }
    } catch (error) { q('[data-flow-note]').textContent = error.message; return; }
   }
   if (!runs.length) return;
   if (time >= duration) time = 0;
   playing = true; lastFrame = 0; frame = requestAnimationFrame(tick);
  }
  render();
 });
 q('[data-flow-reset]').addEventListener('click', () => { stop(); time = 0; lastFrame = 0; render(); });
 q('[data-flow-timeline]').addEventListener('input', event => { stop(); time = Number(event.target.value); render(); });
 q('[data-flow-speed]').addEventListener('change', event => { speed = Number(event.target.value); });
 q('[data-flow-add-edge]').disabled = graph.treatments.length < 2;
 q('[data-flow-add-edge]').addEventListener('click', () => edgeForm(true));
 const entryForm = q('.flow-entry-form');
 entryFormMarkup();
 q('[data-flow-config-entry]').addEventListener('click', () => { entryForm.hidden = false; entryForm.querySelector('select')?.focus(); });
 q('[data-entry-cancel]').addEventListener('click', () => { entryForm.hidden = true; entryForm.querySelector('.inline-error').textContent = ''; });
 q('#activity-entry-form').addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget, error = form.querySelector('.inline-error'); error.textContent = '';
  if (!activity.audience) { error.textContent = '请先选择活动受众。'; return; }
  const routeRefs = activity.audience.type === 'group'
   ? references.filter(item => item.type === 'group' && item.groupId === activity.audience.groupId)
   : references.filter(item => item.type === 'arm' && item.bundleId === activity.audience.bundleId);
  const selectedByGroup = new Map([...form.querySelectorAll('[data-entry-group]')].map(select => [select.dataset.entryGroup, [...select.selectedOptions].map(option => option.value)]));
  const nextEntries = buildEntryRoutes(routeRefs, selectedByGroup);
  const routed = nextEntries.flatMap(entry => entry.treatmentIds);
  if (new Set(routed).size !== routed.length) { error.textContent = '一个起始投放项不能同时分配到多个实验臂。'; return; }
  await saveFlow(nextEntries, graph.edges);
 });
 root.querySelector('[data-flow-cancel]').addEventListener('click', () => edgeForm(false));
 root.querySelector('[data-view="iso"]').addEventListener('click', () => setView('iso'));
 root.querySelector('[data-view="top"]').addEventListener('click', () => setView('top'));
 root.querySelector('[data-view="reset"]').addEventListener('click', () => setView('iso'));
 root.querySelector('[data-zoom="in"]').addEventListener('click', () => { zoom = Math.min(1.25, zoom + .1); renderScene(); });
 root.querySelector('[data-zoom="out"]').addEventListener('click', () => { zoom = Math.max(.7, zoom - .1); renderScene(); });
 q('[data-node-list]').addEventListener('click', event => { const button = event.target.closest('[data-select-node]'); if (button) selectNode(button.dataset.selectNode); });
 const world = q('[data-world]');
 function finishPointer(event, select = false) {
  if (!dragging) return;
  const node = select && !moved ? event.target.closest('[data-flow-node]')?.dataset.flowNode || pressed : null;
  dragging = false; dragStart = null; pressed = null; moved = false;
  world.classList.remove('dragging');
  if (world.hasPointerCapture?.(event.pointerId)) world.releasePointerCapture(event.pointerId);
  if (node) selectNode(node);
 }
 world.addEventListener('pointerdown', event => { if (event.pointerType !== 'touch' && event.button !== 0) return; dragStart = { x: event.clientX, yaw }; pressed = event.target.closest('[data-flow-node]')?.dataset.flowNode || null; dragging = true; moved = false; if (event.pointerType !== 'touch') world.classList.add('dragging'); });
 world.addEventListener('pointermove', event => {
  if (!dragging) return;
  const dx = event.clientX - dragStart.x, wasMoved = moved;
  if (Math.abs(dx) > 4) moved = true;
  if (moved && !wasMoved && event.pointerType !== 'touch') world.setPointerCapture(event.pointerId);
  if (!moved || event.pointerType === 'touch') return;
  yaw = Math.max(-.7, Math.min(.35, dragStart.yaw + dx * .002)); renderScene();
 });
 world.addEventListener('pointerup', event => finishPointer(event, true));
 world.addEventListener('pointercancel', event => finishPointer(event));
 world.addEventListener('lostpointercapture', event => finishPointer(event));
 world.addEventListener('pointerleave', event => { if (!moved) finishPointer(event); });
 world.addEventListener('click', event => { const edge = event.target.closest('[data-flow-edge]'); if (!edge) return; selectedEdge = edge.dataset.flowEdge; selected = graph.connections.find(item => item.id === selectedEdge)?.from || selected; render(); });
 q('.flow-edge-form form').addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget, error = form.querySelector('.inline-error'); error.textContent = '';
  const from = form.elements.fromTreatmentId.value, to = form.elements.toTreatmentId.value, conditionLabel = form.elements.conditionLabel.value;
  if (!from || !to || from === to) { error.textContent = '请选择不同的起点和后续投放项。'; return; }
  const current = editingEdgeId ? graph.edges.find(item => item.id === editingEdgeId) : null;
  if (editingEdgeId && !current) { error.textContent = '这条回执边已不存在，请刷新活动后重试。'; return; }
  const edge = { id: current?.id || `edge-${Date.now().toString(36)}`, fromTreatmentId: from, toTreatmentId: to, conditionLabel };
  const nextEdges = current ? graph.edges.map(item => item.id === current.id ? edge : item) : [...graph.edges, edge];
  if (hasCycle(graph.treatments, nextEdges)) { error.textContent = '这条边会形成循环，请调整起点或终点。'; return; }
  if (graph.edges.some(item => item.id !== edge.id && item.fromTreatmentId === from && item.toTreatmentId === to && item.conditionLabel === conditionLabel)) { error.textContent = '这条回执边已存在。'; return; }
  await saveFlow(graph.entries, nextEdges);
 });
 q('[data-edge-detail]').addEventListener('click', async event => {
  const editExperiment = event.target.closest('[data-edit-experiment]');
  if (editExperiment) { onExperimentAction('edit', editExperiment.dataset.editExperiment); return; }
  const removeExperiment = event.target.closest('[data-remove-experiment]');
  if (removeExperiment) { onExperimentAction('remove', removeExperiment.dataset.removeExperiment); return; }
  const edit = event.target.closest('[data-edit-edge]');
  if (edit) { const edge = graph.edges.find(item => item.id === edit.dataset.editEdge); if (edge) edgeForm(true, edge); return; }
  const remove = event.target.closest('[data-delete-edge]');
  if (remove) await saveFlow(graph.entries, graph.edges.filter(edge => edge.id !== remove.dataset.deleteEdge));
 });
 if (graph.empty && !graph.draftNodes?.length) q('[data-flow-note]').textContent = '当前活动尚无投放项。先添加投放项，画布会按活动 API 状态显示节点。';
 else if (!graph.canRehearse || draftStore && !draftStore.canUse) q('[data-flow-note]').textContent = draftStore && !draftStore.canUse ? draftStore.warning : graph.readinessIssue;
 else q('[data-flow-note]').textContent = '本地回执按动作声明的结果依次轮转，用于查看当前边的路径；不表示实际发送或回执概率。';
 render();
 return () => { stop(); root.innerHTML = ''; };
}
