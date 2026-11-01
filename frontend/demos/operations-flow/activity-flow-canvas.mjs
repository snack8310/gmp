import { referenceKey } from './campaign-ui.mjs';
import { activityRoute, assetRoute } from './activity-routing.mjs';

const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
const colors = { blue: '#2c4a7c', paper: '#fbfaf7', white: '#ffffff', line: '#cfc9be', side: '#e4e0d8', ink: '#55504a' };

function treatmentRefLabel(ref, references) {
 if (!ref) return '未配置人群';
 const key = referenceKey(ref);
 const item = references.find(candidate => referenceKey(candidate) === key);
 return ref.type === 'arm'
  ? `${item?.bundleName || ref.bundleId} / ${item?.armName || item?.name || ref.groupId}`
  : item?.groupName || item?.name || ref.groupId;
}

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

export function buildActivityGraph(activity, actions = [], references = []) {
 const treatments = Array.isArray(activity?.treatments) ? activity.treatments : [];
 const edges = Array.isArray(activity?.activityFlow?.edges) ? activity.activityFlow.edges : [];
 const actionById = new Map(actions.map(item => [item.id, item]));
 const incoming = new Set(edges.map(edge => edge.toTreatmentId));
 const roots = treatments.filter(item => !incoming.has(item.id));
 const entryRefs = Array.isArray(activity?.audienceRefs) && activity.audienceRefs.length ? activity.audienceRefs : roots.map(item => item.audienceRef);
 const rootKeys = new Map();
 for (const ref of entryRefs) {
  const key = referenceKey(ref);
  if (!rootKeys.has(key)) rootKeys.set(key, { id: `audience:${key || 'missing'}`, key, label: treatmentRefLabel(ref, references) });
 }
 const nodes = [...rootKeys.values()].map((item, index, all) => ({
  id: item.id, name: item.label || '受众入口', kind: 'audience', x: -390,
  z: (index - (all.length - 1) / 2) * Math.min(190, 440 / Math.max(1, all.length - 1)), w: 126, d: 108, h: 16,
  desc: `共享受众入口。由活动投放项引用「${item.label}」。`, rules: [['入口身份', item.key || '未配置'], ['进入条件', '受众资产持有'], ['跨活动复用', '可复用']], audienceKey: item.key,
 }));
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
    rules: [['投放项 ID', treatment.id], ['动作', action?.name || treatment.actionId], ['引用人群', treatmentRefLabel(treatment.audienceRef, references)]],
    actionName: action?.name || treatment.actionId, treatment,
   };
   nodes.push(node);
  });
 }
 const terminal = { id: 'flow-terminal', name: '演练结束', kind: 'terminal', x: 440, z: 0, w: 94, d: 82, h: 30,
  desc: '当前活动流程在这里结束。只显示构造样本的本地路径，不表示真实发送或业务效果。',
  rules: [['活动', activity?.name || activity?.id], ['渠道连接', '无'], ['持久化', 'API 进程内状态']],
 };
 if (treatments.length) nodes.push(terminal);
 const byId = new Map(nodes.map(node => [node.id, node]));
 const connections = [];
 for (const treatment of roots) {
  const key = referenceKey(treatment.audienceRef);
  const source = `audience:${key || 'missing'}`;
  if (byId.has(source) && byId.has(treatment.id)) connections.push({ id: `entry:${source}:${treatment.id}`, from: source, to: treatment.id, kind: 'entry', label: '入口人群' });
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
  for (const outcome of endings) if (byId.has(terminal.id)) connections.push({ id: `complete:${treatment.id}:${outcome}`, from: treatment.id, to: terminal.id, kind: 'complete', label: outcome });
 }
 return { nodes, byId, connections, roots, treatments, edges, actionById, hasCycle: hasCycle(treatments, edges), empty: treatments.length === 0, maxColumn };
}

function markup(activity, references) {
 const route = activityRoute(activity.id, 'campaign');
 const audienceOptions = references.filter(item => item.available).map(item => {
  const key = referenceKey(item), label = item.type === 'arm' ? `${item.bundleName} / ${item.armName}` : item.groupName;
  return `<option value="${esc(key)}">${esc(label)} · ${esc(item.type === 'arm' ? `${item.bundleId} / ${item.groupId}` : item.groupId)}</option>`;
 }).join('');
 return `<section class="activity-flow-canvas" aria-label="活动流程 3D 画布">
 <div class="flow-canvas-heading"><div><p class="eyebrow">ACTIVITY FLOW / LOCAL REHEARSAL</p><h3>活动流程</h3><p>在活动内查看受众入口、投放项与回执边。沿边进入的人员来自上一步回执，不会重新枚举目标投放项的人群。</p></div><div class="flow-canvas-actions"><a class="primary-link" href="#${route}/treatments/new">＋ 添加投放项</a><a class="primary-link flow-audience-link" href="#${assetRoute('audience', route)}">管理受众</a></div></div>
 <p class="flow-rehearsal-warning" role="note">本地演练 · 只使用构造样本和预设回执，不会触达真实用户或真实渠道。</p>
 <div class="flow-audience-form" hidden><form id="activity-audience-form"><label>共享受众入口<select name="audienceRef"><option value="">选择一个受众</option>${audienceOptions}</select></label><a href="#${assetRoute('audience', route)}">新建或管理受众</a><button class="primary" type="submit">放入活动流程</button><button type="button" data-audience-cancel>取消</button><p class="inline-error" role="alert"></p></form></div>
 <div class="flow-edge-form" hidden><form id="activity-flow-form"><p class="flow-edge-form-title" data-flow-form-title>添加回执边</p><label>起点投放项<select name="fromTreatmentId"></select></label><label>回执条件<select name="conditionLabel"></select></label><label>后续投放项<select name="toTreatmentId"></select></label><button class="primary" type="submit">添加回执边</button><button type="button" data-flow-cancel>取消</button><p class="inline-error" role="alert"></p></form></div>
 <div class="flow-edge-toolbar"><button type="button" data-flow-add-audience>＋ 添加受众入口</button><button type="button" data-flow-add-edge>＋ 添加回执边</button><span class="flow-save-status" role="status" aria-live="polite"></span></div>
 <section class="metrics" aria-label="本地演练统计"><div class="metric"><span>构造路径</span><strong data-flow-total>0</strong><small>本地虚构样本</small></div><div class="metric"><span>等待进入</span><strong data-flow-queued>0</strong><small>尚未开始</small></div><div class="metric"><span>演练中</span><strong data-flow-active>0</strong><small>沿当前图形流转</small></div><div class="metric"><span>已完成</span><strong data-flow-complete>0</strong><small>抵达流程终点</small></div><div class="metric"><span>已走回执边</span><strong data-flow-edges>0</strong><small>本地脚本统计</small></div><div class="balance"><span>状态<strong data-flow-state>已暂停</strong></span></div></section>
 <section class="workspace flow-canvas-workspace">
  <aside class="left-panel"><div class="panel-label"><span>流程节点</span><span data-node-total>0</span></div><div class="node-list" data-node-list aria-label="选择流程节点"></div><div class="left-note"><span class="tiny-heading">本地活动演练</span><p>图形读取当前活动 API 状态。播放时按动作回执条件轮流演示路径，不代表真实分布概率。</p><span class="version">SCENE MODEL / V1.0</span></div></aside>
  <div class="world-wrap"><div class="world-toolbar"><div class="world-title"><span class="live-dot"></span><span data-play-state>已暂停</span><span class="divider">/</span><span>活动流程立体画布</span></div><div class="view-controls" aria-label="视角控制"><button type="button" data-view="iso" class="active" aria-pressed="true">立体</button><button type="button" data-view="top" aria-pressed="false">俯视</button><button type="button" data-zoom="out" aria-label="缩小">−</button><button type="button" data-zoom="in" aria-label="放大">+</button><button type="button" data-view="reset">复位</button></div></div>
   <div class="world" data-world><svg data-scene viewBox="0 0 1100 680" role="img" aria-label="当前活动的立体流程图"><defs><pattern data-grid width="40" height="40" patternUnits="userSpaceOnUse"><path d="M40 0H0V40" fill="none" stroke="#e4e0d8" stroke-width=".6"/></pattern></defs><g data-ground></g><g data-routes></g><g data-objects></g><g data-labels></g></svg></div>
   <div class="world-foot"><span><span class="legend-dot"></span>构造样本 <span class="legend-line"></span>活动流程边 <span class="legend-dash"></span>失败回执条件</span><span>拖动旋转 · 点选节点或边</span></div><div class="scenario-note"><span>场景说明</span><p data-flow-note>图形来自当前活动；活动流程边决定回执后续步骤。</p></div>
  </div>
  <aside class="detail-panel" aria-label="节点检查器"><div class="panel-label"><span data-detail-kind>ACTIVITY FLOW</span><span data-detail-index>01</span></div><div class="detail-icon" data-detail-icon></div><p class="detail-kind" data-detail-type>ACTIVITY FLOW NODE</p><h2 data-detail-title>活动流程</h2><p class="detail-desc" data-detail-desc></p><div class="detail-stats"><div><span>当前到达</span><strong data-detail-arrived>0</strong></div><div><span>当前执行</span><strong data-detail-active>0</strong></div></div><div class="mini-track"><span data-detail-progress></span></div><div class="detail-rules" data-detail-rules></div><div data-edge-detail></div><div class="detail-bottom"><span>本地演练</span><p>成功回执只表示模拟动作完成，不代表用户阅读、下单或真实业务效果。</p></div></aside>
 </section>
 <section class="transport" aria-label="本地演练控制"><div class="play-controls"><button type="button" class="primary" data-flow-play>开始本地演练</button><button type="button" data-flow-reset>重置</button></div><div class="timeline"><div><span>演练时间 <strong data-flow-time>00:00.0</strong></span><span data-flow-duration>00:12.0</span></div><input type="range" min="0" max="12" step="any" value="0" data-flow-timeline aria-label="演练时间位置"></div><label class="speed">速度<select data-flow-speed><option value="0.5">0.5×</option><option value="1" selected>1×</option><option value="2">2×</option><option value="4">4×</option></select></label><span class="motion-note">SVG 立体投影<br>旧版沙盘实现</span></section>
 </section>`;
}

export function mountActivityFlowCanvas({ root, activity, actions = [], references = [], api, onSaved = () => {} }) {
 if (!root || !activity) return () => {};
 root.innerHTML = markup(activity, references);
 const q = selector => root.querySelector(selector);
 const graph = buildActivityGraph(activity, actions, references);
 let selected = graph.nodes[0]?.id || null, selectedEdge = null, editingEdgeId = null, savingEdges = false, savingAudienceRefs = false;
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
  if (node.kind === 'terminal') return [0,1,2].map(index => cube(node.x - 22 + index * 22, node.z, 18, 34, 16 + index * 8, 15, true)).join('');
  return cube(node.x, node.z, 44, 38, 52, 15) + cube(node.x, node.z, 48, 42, 6, 67, true);
 }
 function renderNode(node) {
  const symbol = node.kind === 'treatment' ? esc((node.actionName || node.name).slice(0, 1).toUpperCase()) : node.kind === 'terminal' ? '✓' : '人';
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
  const failure = normalizedOutcome(connection.label) === 'failure';
  const color = failure ? colors.ink : colors.blue;
  let output = line(path, colors.white, 9) + line(path, color, 1.25, failure ? '4 5' : '');
  if (connection.kind === 'receipt') {
   const [x, y] = project((graph.byId.get(connection.from).x + graph.byId.get(connection.to).x) / 2, 20, (graph.byId.get(connection.from).z + graph.byId.get(connection.to).z) / 2);
   output += `<g class="scene-node" data-flow-edge="${esc(connection.id)}" tabindex="0" role="button" aria-label="回执边：${esc(connection.label)}"><rect class="node-label-back node-outline" x="${(x - 40).toFixed(1)}" y="${(y - 17).toFixed(1)}" width="80" height="20" rx="3"/><text class="scene-sub" text-anchor="middle" x="${x.toFixed(1)}" y="${(y - 3).toFixed(1)}">${esc(connection.label)}</text></g>`;
  }
  return output;
 }
 function renderLabels() {
  return graph.nodes.map(node => {
   const [x, y] = node.kind === 'treatment' ? project(node.x, 0, node.z + node.d / 2 + 20) : project(node.x, 0, node.z + node.d / 2 + 20);
   const width = Math.max(88, Math.min(150, node.name.length * 9 + 20));
   const sub = node.kind === 'treatment' ? node.actionName : node.kind === 'audience' ? '共享受众入口' : '当前流程终点';
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
  for (const rootTreatment of graph.roots) {
   const key = referenceKey(rootTreatment.audienceRef);
   const audienceId = `audience:${key || 'missing'}`;
   for (let user = 0; user < 12; user++) expand(rootTreatment.id, user, [audienceId, rootTreatment.id], new Set());
  }
  return runs;
 }
 const runs = buildRuns();
 const duration = Math.max(8, Math.min(30, Math.max(...runs.map(run => run.path.length), 1) * 2.2));
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
  q('[data-detail-type]').textContent = node.kind === 'treatment' ? 'TREATMENT NODE' : node.kind === 'audience' ? 'AUDIENCE SOURCE' : 'FLOW TERMINAL';
  q('[data-detail-icon]').innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v14H4zM8 9h8m-8 4h8m-8 4h4"/></svg>';
  q('[data-detail-rules]').innerHTML = (node.rules || []).map(([label,value]) => `<div><span>${esc(label)}</span><strong>${esc(value)}</strong></div>`).join('');
  const at = graph.treatments.findIndex(item => item.id === node.id);
  const treatment = at >= 0 ? graph.treatments[at] : null;
  const edit = treatment ? `<a class="primary-link" href="#${activityRoute(activity.id, 'campaign')}/treatments/${encodeURIComponent(treatment.id)}/edit">编辑此投放项</a>` : '';
  const audienceInUse = node.kind === 'audience' && graph.roots.some(item => referenceKey(item.audienceRef) === node.audienceKey);
  const audienceAction = node.kind === 'audience' ? audienceInUse
   ? '<p class="flow-audience-use-note">入口已绑定起始投放项；先编辑投放项或调整流程边，再移除入口。</p>'
   : `<button type="button" data-remove-audience-entry="${esc(node.audienceKey)}">从本活动移除入口</button>` : '';
  const edges = selectedEdge ? graph.edges.filter(edge => edge.id === selectedEdge) : [];
  const edgeMarkup = edges.map(edge => `<div class="flow-selected-edge"><p>活动流程边 · ${esc(edge.conditionLabel)}</p><p>${esc(graph.byId.get(edge.fromTreatmentId)?.name)} → ${esc(graph.byId.get(edge.toTreatmentId)?.name)}</p><div class="flow-edge-actions"><button type="button" data-edit-edge="${esc(edge.id)}">编辑这条边</button><button type="button" data-delete-edge="${esc(edge.id)}">删除这条边</button></div></div>`).join('');
  q('[data-edge-detail]').innerHTML = `${edit}${audienceAction}${edgeMarkup}`;
  q('[data-detail-arrived]').textContent = playing ? String(Math.min(runs.length, Math.floor(time / duration * runs.length))) : '0';
  q('[data-detail-active]').textContent = playing && time < duration ? String(runs.length) : '0';
  q('[data-detail-progress]').style.width = `${playing ? Math.min(100, time / duration * 100) : 0}%`;
 }
 function updateMetrics() {
  const progress = duration ? Math.min(1, time / duration) : 0;
  q('[data-flow-total]').textContent = String(runs.length);
  q('[data-flow-queued]').textContent = String(Math.floor(runs.length * (1 - progress)));
  q('[data-flow-active]').textContent = playing && progress < 1 ? String(runs.length) : '0';
  q('[data-flow-complete]').textContent = String(Math.floor(runs.length * progress));
  q('[data-flow-edges]').textContent = String(Math.floor(graph.edges.length * progress));
  q('[data-flow-state]').textContent = graph.hasCycle ? '配置含环' : playing ? progress >= 1 ? '演练完成' : '演练中' : '已暂停';
  q('[data-play-state]').textContent = playing ? '本地演练中' : progress >= 1 ? '演练结束' : '已暂停';
  q('[data-flow-play]').textContent = graph.hasCycle ? '请先修正循环边' : playing ? '暂停演练' : progress >= 1 ? '重新演练' : '开始本地演练';
  q('[data-flow-play]').disabled = graph.hasCycle || runs.length === 0;
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
 async function saveEdges(nextEdges) {
  if (savingEdges) return false;
  savingEdges = true;
  const status = q('.flow-save-status');
  const controls = [...root.querySelectorAll('[data-flow-add-edge], #activity-flow-form button, #activity-flow-form select, [data-edge-detail] [data-edit-edge], [data-edge-detail] [data-delete-edge]')];
  const priorDisabled = controls.map(control => control.disabled);
  controls.forEach(control => { control.disabled = true; });
  q('#activity-flow-form')?.setAttribute('aria-busy', 'true');
  status.textContent = '正在保存…';
  try {
   await api.updateFlow(activity.id, { id: activity.activityFlow?.id || `flow-${activity.id}`, edges: nextEdges }, graph.edges);
   status.textContent = '已保存到当前管理 API 进程。';
   await onSaved();
   return true;
  } catch (error) { status.textContent = `保存失败：${error.message}`; return false; }
  finally {
   savingEdges = false;
   q('#activity-flow-form')?.removeAttribute('aria-busy');
   controls.forEach((control, index) => { if (control.isConnected) control.disabled = priorDisabled[index]; });
  }
 }

 function currentAudienceRefs() {
  if (Array.isArray(activity.audienceRefs)) return activity.audienceRefs;
  return [...new Map(graph.roots.map(item => [referenceKey(item.audienceRef), item.audienceRef])).values()];
 }
 async function saveAudienceRefs(nextRefs) {
  if (savingAudienceRefs) return false;
  savingAudienceRefs = true;
  const status = q('.flow-save-status');
  const controls = [...root.querySelectorAll('[data-flow-add-audience], #activity-audience-form button, #activity-audience-form select, [data-edge-detail] [data-remove-audience-entry]')];
  const priorDisabled = controls.map(control => control.disabled);
  controls.forEach(control => { control.disabled = true; });
  q('#activity-audience-form')?.setAttribute('aria-busy', 'true');
  status.textContent = '正在保存受众入口…';
  try {
   await api.updateAudiences(activity.id, nextRefs, currentAudienceRefs());
   status.textContent = '受众入口已保存到当前管理 API 进程。';
   await onSaved();
   return true;
  } catch (error) { status.textContent = `保存受众入口失败：${error.message}`; return false; }
  finally {
   savingAudienceRefs = false;
   q('#activity-audience-form')?.removeAttribute('aria-busy');
   controls.forEach((control, index) => { if (control.isConnected) control.disabled = priorDisabled[index]; });
  }
 }

 q('[data-flow-play]').addEventListener('click', () => { if (graph.hasCycle || !runs.length) return; if (playing) { stop(); } else { if (time >= duration) time = 0; playing = true; lastFrame = 0; frame = requestAnimationFrame(tick); } render(); });
 q('[data-flow-reset]').addEventListener('click', () => { stop(); time = 0; lastFrame = 0; render(); });
 q('[data-flow-timeline]').addEventListener('input', event => { stop(); time = Number(event.target.value); render(); });
 q('[data-flow-speed]').addEventListener('change', event => { speed = Number(event.target.value); });
 q('[data-flow-add-edge]').disabled = graph.treatments.length < 2;
 q('[data-flow-add-edge]').addEventListener('click', () => edgeForm(true));
 const audienceForm = q('.flow-audience-form');
 q('[data-flow-add-audience]').addEventListener('click', () => { audienceForm.hidden = false; audienceForm.querySelector('select').focus(); });
 q('[data-audience-cancel]').addEventListener('click', () => { audienceForm.hidden = true; audienceForm.querySelector('.inline-error').textContent = ''; });
 q('#activity-audience-form').addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.currentTarget, error = form.querySelector('.inline-error'); error.textContent = '';
  const key = form.elements.audienceRef.value;
  const reference = references.find(item => item.available && referenceKey(item) === key);
  if (!reference) { error.textContent = '请选择当前共享目录中可用的受众。'; return; }
  const refs = currentAudienceRefs();
  if (refs.some(item => referenceKey(item) === key)) { error.textContent = '这个受众已经是活动入口。'; return; }
  const nextRef = reference.type === 'arm'
   ? { type: 'arm', bundleId: reference.bundleId, groupId: reference.groupId }
   : { type: 'group', groupId: reference.groupId };
  await saveAudienceRefs([...refs, nextRef]);
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
  await saveEdges(nextEdges);
 });
 q('[data-edge-detail]').addEventListener('click', async event => {
  const edit = event.target.closest('[data-edit-edge]');
  if (edit) { const edge = graph.edges.find(item => item.id === edit.dataset.editEdge); if (edge) edgeForm(true, edge); return; }
  const remove = event.target.closest('[data-delete-edge]');
  if (remove) await saveEdges(graph.edges.filter(edge => edge.id !== remove.dataset.deleteEdge));
 });
 q('[data-edge-detail]').addEventListener('click', async event => {
  const button = event.target.closest('[data-remove-audience-entry]');
  if (!button) return;
  const refs = currentAudienceRefs().filter(item => referenceKey(item) !== button.dataset.removeAudienceEntry);
  await saveAudienceRefs(refs);
 });
 if (graph.empty) q('[data-flow-note]').textContent = '当前活动尚无投放项。先添加投放项，画布会按活动 API 状态显示节点。';
 else if (graph.hasCycle) q('[data-flow-note]').textContent = '当前活动边包含循环，已暂停本地演练；先删除循环边再运行。';
 else q('[data-flow-note]').textContent = '本地回执按动作声明的结果依次轮转，用于查看当前边的路径；不表示实际发送或回执概率。';
 render();
 return () => { stop(); root.innerHTML = ''; };
}
