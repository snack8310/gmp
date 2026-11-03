import { composeFlowExperimentGraph, flowAnchorSignature, flowExperimentRouteTarget } from './flow-experiment-graph.mjs';
import { escapeHTML } from './experiment-ui.mjs';
import { activityRoute } from './activity-routing.mjs';

const flowSafe = value => escapeHTML(String(value ?? ''));
const flowUiParts = [['a', 'A 组'], ['b', 'B 组'], ['control', '对照组']];

export function mountFlowExperiments({ root, activity, actions, references, experimentStore, draftStore, onChange }) {
 if (!root) return () => {};
 let editing = null, open = false, message = '';
 const experiments = () => experimentStore.currentExperiments?.() ?? experimentStore.experiments();
 const nodes = () => draftStore.nodes(activity.id);
 const graph = list => composeFlowExperimentGraph(activity, actions, references, experiments(), list);
 const libraryHref = experimentId => `#experimentation${experimentId ? `/${encodeURIComponent(experimentId)}` : ''}?return=${encodeURIComponent(activityRoute(activity.id, 'campaign'))}`;
 const routeName = (id, current) => id === 'flow-terminal' ? '在此结束' : current.byId.get(id)?.name || id;
 function render() {
  const list = nodes(), current = graph(list), draft = list.find(node => node.id === editing);
  const formGraph = draft ? graph(list.filter(node => node.id !== draft.id)) : current;
  const available = experiments().filter(item => item.runtimeStatus === 'running');
  const anchors = formGraph.anchors.filter(item => item.kind !== 'control' && item.kind !== 'split');
  const anchor = anchors.find(item => item.id === draft?.anchorId) || anchors[0];
  const destinations = [{ id: 'flow-terminal', name: '在此结束' }, ...formGraph.nodes.filter(item => item.kind === 'treatment' || item.kind === 'experiment')];
  const options = destinations.map(item => `<option value="${flowSafe(item.id)}">${flowSafe(item.name)}</option>`).join('');
  const rows = list.map(node => {
   const definition = experiments().find(item => item.id === node.experimentId);
   const anchorInfo = current.connections.find(item => item.id === `experiment-in:${node.id}`);
   const previous = graph(list.slice(0, list.indexOf(node)));
   const source = previous.connections.find(item => item.id === node.anchorId);
   const position = source ? `${previous.byId.get(source.from)?.name || source.from}的「${source.label}」路径` : '需要重新连接';
   return `<div class="flow-experiment-row"><div><strong>${flowSafe(definition?.name || node.experimentId)}</strong> <small class="flow-experiment-id">· ${flowSafe(node.id)}</small><p>位置：${flowSafe(position)}<br>${flowUiParts.map(([part, label]) => `${label} → ${flowSafe(routeName(flowExperimentRouteTarget(current, node, part), current))}`).join('；')}</p><small class="flow-experiment-state">${definition?.runtimeStatus === 'running' && anchorInfo ? '共享实验库配置 · 此节点独立分组' : '引用或路径不可用 · 请修复后演练'}</small></div><div class="campaign-actions"><button type="button" data-flow-edit="${flowSafe(node.id)}">编辑</button><button type="button" data-flow-remove="${flowSafe(node.id)}">移除</button></div></div>`;
  }).join('') || '<p class="campaign-empty">尚无流程实验节点。</p>';
  const experimentOptions = (draft && !available.some(item => item.id === draft.experimentId) ? '<option value="" selected>引用不可用，请重新选择运行中的实验</option>' : '')
   + available.map(item => `<option value="${flowSafe(item.id)}" ${item.id === draft?.experimentId ? 'selected' : ''}>${flowSafe(item.name)} · A ${item.split.a}% / B ${item.split.b}% / 对照 ${item.split.control}% · ${item.colored ? '记录首次分配' : '不记录分配'}</option>`).join('');
  const anchorOptions = anchors.map(item => `<option value="${flowSafe(item.id)}" ${item.id === draft?.anchorId ? 'selected' : ''}>${flowSafe(item.label)} · ${flowSafe(formGraph.byId.get(item.from)?.name)} → ${flowSafe(formGraph.byId.get(item.to)?.name)}</option>`).join('');
  root.innerHTML = `<div class="flow-experiment-heading"><div><h3>活动实验</h3><p class="campaign-note">在流程中到达节点才分组。比例和策略由实验库共享管理；受众和每个流程节点独立分组。</p></div><button class="primary-link" type="button" data-flow-add>＋ 添加实验</button></div><p class="flow-experiment-local">仅此浏览器的流程草稿，尚未保存到管理 API 或接入 Core，不参与真实执行。</p>${draftStore.warning ? `<p class="storage-warning" role="status">${flowSafe(draftStore.warning)}</p>` : ''}${current.issues.length ? `<p class="inline-error" role="status">${flowSafe(current.issues.join('；'))}</p>` : ''}${rows}${message ? `<p class="${message.startsWith('已保存') || message.startsWith('已从') ? 'campaign-note' : 'inline-error'}" role="status">${flowSafe(message)}</p>` : ''}${open ? `<form class="flow-experiment-form" data-flow-form><h4>${draft ? '编辑流程实验节点' : '添加流程实验节点'}</h4><label>实验库定义<select name="experimentId" required>${experimentOptions || '<option value="">先在实验库运行一个实验</option>'}</select></label><label>插入路径<select name="anchorId" required>${anchorOptions || '<option value="">先配置活动入口或投放项</option>'}</select></label><div class="flow-experiment-routes">${flowUiParts.map(([part, label]) => `<label>${label}去向<select name="route-${part}" required>${options}</select></label>`).join('')}</div><p class="campaign-note">在此结束表示该组不再进入后续投放项。原路径可在任一组中继续连接。</p><div class="campaign-actions"><button class="primary-link" type="submit">保存到此浏览器</button><button type="button" data-flow-cancel>取消</button><a href="${flowSafe(libraryHref(draft?.experimentId || available[0]?.id))}">管理实验库比例与策略</a></div></form>` : ''}`;
  if (open) {
   const form = root.querySelector('[data-flow-form]');
   for (const [part] of flowUiParts) form.elements[`route-${part}`].value = draft?.routes[part] || (part === 'a' ? anchor?.to || 'flow-terminal' : 'flow-terminal');
  }
 }
 const click = event => {
  const button = event.target.closest('button'); if (!button || !root.contains(button)) return;
  if (button.hasAttribute('data-flow-add')) { editing = null; open = true; message = ''; render(); root.querySelector('[name="experimentId"]')?.focus(); }
  if (button.hasAttribute('data-flow-cancel')) { open = false; render(); }
  if (button.dataset.flowEdit) { editing = button.dataset.flowEdit; open = true; message = ''; render(); root.querySelector('[name="anchorId"]')?.focus(); }
  if (button.dataset.flowRemove) {
   try {
    const list = nodes(), candidate = list.filter(node => node.id !== button.dataset.flowRemove);
    if (candidate.some(node => node.anchorId.startsWith(`experiment:${button.dataset.flowRemove}:`) || Object.values(node.routes).includes(`flow-experiment:${button.dataset.flowRemove}`))) throw new RangeError('其他实验节点仍引用此节点，请先调整后续连接。');
    draftStore.removeNode(activity.id, button.dataset.flowRemove); open = false; editing = null; message = '已从本浏览器草稿移除。'; onChange(); render();
   } catch (error) { message = error.message; render(); }
  }
 };
 const submit = event => {
  if (!event.target.matches('[data-flow-form]')) return;
  event.preventDefault();
  try {
   const form = event.target, list = nodes();
   const id = editing || `node-${globalThis.crypto?.randomUUID?.().slice(0, 12) || Date.now().toString(36)}`;
   const selectedAnchor = graph(editing ? list.filter(item => item.id !== editing) : list).anchors.find(item => item.id === form.elements.anchorId.value);
   if (!selectedAnchor) throw new RangeError('选定的插入路径不可用，请重新选择。');
   const node = { id, experimentId: form.elements.experimentId.value, anchorId: form.elements.anchorId.value, anchorSignature: flowAnchorSignature(selectedAnchor),
    routes: Object.fromEntries(flowUiParts.map(([part]) => [part, form.elements[`route-${part}`].value])) };
   if (!experiments().some(item => item.id === node.experimentId && item.runtimeStatus === 'running')) throw new RangeError('请选择运行中的实验定义。');
   const candidate = editing ? list.map(item => item.id === editing ? node : item) : [...list, node];
   const result = graph(candidate);
   if (result.issues.length) throw new RangeError(result.issues.join('；'));
   if (!result.connections.some(item => item.id === `experiment-in:${id}`)) throw new RangeError('选定的插入路径不可用。');
   draftStore.saveNode(activity.id, node);
   open = false; editing = null; message = draftStore.warning || '已保存到此浏览器。'; onChange(); render();
  } catch (error) { message = error.message; render(); }
 };
 root.addEventListener('click', click); root.addEventListener('submit', submit); render();
 return () => { root.removeEventListener?.('click', click); root.removeEventListener?.('submit', submit); root.innerHTML = ''; };
}
