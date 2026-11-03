import { escapeHTML } from './experiment-ui.mjs';
import { referenceKey, renderActivitiesList, renderActivityDetail, validateTreatmentDraft } from './campaign-ui.mjs';
import { assetRoute } from './activity-routing.mjs';
import { mountActivityFlowCanvas } from './activity-flow-canvas.mjs';

const url = id => encodeURIComponent(id);

export function mountCampaigns({ api, audienceStore, onNavigate, onChanged }) {
 const safe = value => escapeHTML(String(value ?? ''));
 const el = id => document.getElementById(id);
 const section = el('campaign-section');
 let serial = 0, activity = null, treatment = null, actions = [], serverRefs = [], destroyFlowCanvas = () => {};
 const references = () => audienceStore.catalog().map(local => {
  const remote = serverRefs.find(item => referenceKey(item) === referenceKey(local));
  return { ...local, available: Boolean(local.available && remote?.available) };
 });
 const status = message => { el('campaign-status').textContent = message; };
 const error = message => { const target = el('campaign-error'); if (target) target.textContent = message; else status(message); };
 const detailRoute = () => `activities/${url(activity.id)}/campaign`;

 function selectionOptions() {
  const refs = references();
  const groups = refs.filter(item => item.type === 'group').map(item => ({ key: `group:${item.groupId}`, label: `${item.groupName} · ${item.groupId}`, available: item.available }));
  const bundles = [...new Map(refs.filter(item => item.type === 'arm').map(item => [item.bundleId, item])).values()].map(item => ({
   key: `bundle:${item.bundleId}`, label: `${item.bundleName} · ${item.bundleId}`,
   available: refs.filter(ref => ref.type === 'arm' && ref.bundleId === item.bundleId).every(ref => ref.available),
  }));
  return [...groups, ...bundles];
 }
 const selectionKey = value => value?.type === 'group' ? `group:${value.groupId}` : value?.type === 'bundle' ? `bundle:${value.bundleId}` : '';
 const selectionFromKey = key => key.startsWith('group:') ? { type: 'group', groupId: key.slice(6) } : key.startsWith('bundle:') ? { type: 'bundle', bundleId: key.slice(7) } : null;

 function activityEditor(version) {
  const selected = selectionKey(activity?.audience);
  const options = selectionOptions();
  const missing = activity?.audience && !options.some(item => item.key === selected) ? `<option value="${safe(selected)}" selected>当前受众不可用 · ${safe(selected)}</option>` : '';
  const markup = options.map(item => `<option value="${safe(item.key)}" ${item.key === selected ? 'selected' : ''} ${item.available ? '' : 'disabled'}>${safe(item.label)}${item.available ? '' : '（不可用）'}</option>`).join('');
  el('campaign-editor').innerHTML = `<a class="campaign-back" href="${activity ? `#${detailRoute()}` : '#activities'}">← 返回</a><div class="section-heading"><div><h2>${activity ? '编辑活动' : '新建活动'}</h2><p>${activity ? `稳定 ID · ${safe(activity.id)}` : '保存后由服务端分配稳定 ID'}</p></div></div><form id="campaign-form" class="campaign-form" data-controller-version="${version}"><label>活动名称<input id="campaign-name" value="${safe(activity?.name)}" maxlength="48" required></label><label>活动受众<select id="campaign-audience"><option value="">选择一个人群组或一个实验人群包</option>${missing}${markup}</select></label><div class="campaign-actions"><a class="primary-link" href="#${assetRoute('audience', activity ? detailRoute() : 'activities')}">新建或管理受众</a></div><p class="campaign-note">每个活动只选择一个受众。更换受众会清空画布入口分配、保留回执边；保存后请在流程画布重新分配起始投放项。</p><p id="campaign-error" class="inline-error" role="alert"></p><div class="campaign-actions"><button class="primary" type="submit">保存活动</button><button type="button" data-cancel-campaign data-controller-version="${version}">取消</button></div></form>`;
 }

 function treatmentEditor(version) {
  const action = actions.find(item => item.id === treatment?.actionId);
  const actionOptions = actions.filter(item => item.available).map(item => `<option value="${safe(item.id)}" ${item.id === treatment?.actionId ? 'selected' : ''}>${safe(item.name)} · ${safe(item.id)}</option>`).join('');
  const missingAction = treatment?.actionId && !action?.available ? `<option value="${safe(treatment.actionId)}" selected>${safe(action?.name || treatment.actionId)} · ${safe(treatment.actionId)}（不可用）</option>` : '';
  const slot = treatment?.slotId || '';
  const slots = ['', 'slot-home-banner', 'slot-home-hero'];
  const slotOptions = slots.map(item => `<option value="${safe(item)}" ${item === slot ? 'selected' : ''}>${safe(item || '不占展示位')}</option>`).join('');
  const unknownSlot = slot && !slots.includes(slot) ? `<option value="${safe(slot)}" selected>${safe(slot)}（不可用）</option>` : '';
  el('campaign-editor').innerHTML = `<a class="campaign-back" href="#${detailRoute()}">← 返回活动</a><div class="section-heading"><div><h2>${treatment?.id ? '编辑投放项' : '新增投放项'}</h2><p>${treatment?.id ? `稳定 ID · ${safe(treatment.id)}` : '保存后由服务端分配稳定 ID'}</p></div></div><form id="treatment-form" class="campaign-form" data-controller-version="${version}"><label>名称<input id="treatment-name" value="${safe(treatment?.name)}" maxlength="48" required></label><label>动作<select id="treatment-action"><option value="">选择动作</option>${missingAction}${actionOptions}</select></label><label>展示位<select id="treatment-slot">${unknownSlot}${slotOptions}</select></label><p class="campaign-note">投放项不单独绑定受众。保存后请在活动流程画布将它分配到活动受众的入口路径。</p><p id="campaign-error" class="inline-error" role="alert"></p><div class="campaign-actions"><button class="primary" type="submit">保存投放项</button><button type="button" data-cancel-campaign data-controller-version="${version}">取消</button></div></form>`;
 }

 async function render(route) {
  destroyFlowCanvas(); destroyFlowCanvas = () => {};
  const request = ++serial;
  const parts = route.split('/');
  const view = route === 'campaign/activities' ? 'list' : route === 'campaign/activities/new' || parts[3] === 'edit' || parts[3] === 'treatments' ? 'edit' : parts.length === 3 && parts[0] === 'campaign' && parts[1] === 'activities' ? 'detail' : 'overview';
  if (view === 'edit') status('');
  for (const [id, kind] of [['campaign-overview', 'overview'], ['campaign-activities', 'list'], ['campaign-detail', 'detail'], ['campaign-editor', 'edit']]) el(id).hidden = view !== kind;
  if (view === 'overview') return;
  const target = el(view === 'edit' ? 'campaign-editor' : view === 'list' ? 'campaign-activities' : 'campaign-detail');
  target.innerHTML = '<p class="campaign-empty">正在读取活动管理 API…</p>';
  activity = null; treatment = null;
  try {
   if (view === 'list') {
    const result = await api.list();
    if (request === serial) target.innerHTML = renderActivitiesList(result.campaigns, true);
    return;
   }
   if (route === 'campaign/activities/new') {
    const [actionResult, refResult] = await Promise.all([api.actions(), api.audienceReferences()]);
    if (request !== serial) return;
    actions = actionResult.actions; serverRefs = refResult.audienceReferences;
    activityEditor(request); return;
   }
   let id;
   try { id = decodeURIComponent(parts[2]); } catch { if (request === serial) target.innerHTML = renderActivityDetail('', []); return; }
   const result = await api.get(id).catch(error => { if (error.status === 404) return null; throw error; });
   if (request !== serial) return;
   if (!result) { target.innerHTML = renderActivityDetail(id, []); return; }
   activity = result;
   const [actionResult, refResult] = await Promise.all([api.actions(), api.audienceReferences()]);
   if (request !== serial) return;
   actions = actionResult.actions; serverRefs = refResult.audienceReferences;
   if (view === 'detail') {
    target.innerHTML = renderActivityDetail(id, [activity], audienceStore.catalog(), true, actions, serverRefs, request);
    const canvasRoot = target.querySelector?.('#activity-flow-canvas');
    if (canvasRoot?.querySelector) destroyFlowCanvas = mountActivityFlowCanvas({ root: canvasRoot, activity, actions, references: references(), api,
     onSaved: () => changed('活动流程边已保存。', `activities/${url(activity.id)}/campaign`, request) });
   }
   else if (parts[3] === 'edit' && parts.length === 4) activityEditor(request);
   else if (parts[3] === 'treatments' && (parts[4] === 'new' || parts[5] === 'edit')) {
    treatment = parts[4] === 'new' ? null : activity.treatments.find(item => item.id === decodeURIComponent(parts[4]));
    target.innerHTML = parts[4] === 'new' || treatment ? '' : '<p class="campaign-empty">未找到该投放项</p>';
    if (parts[4] === 'new' || treatment) treatmentEditor(request);
   } else target.innerHTML = '<p class="campaign-empty">未找到该页面</p>';
  } catch (failure) {
   if (request === serial) target.innerHTML = `<p class="inline-error" role="alert">无法读取活动管理 API：${safe(failure.message)}</p>`;
  }
 }

 function clear() {
  destroyFlowCanvas(); destroyFlowCanvas = () => {};
  serial++;
  activity = null; treatment = null;
  for (const id of ['campaign-overview', 'campaign-activities', 'campaign-detail', 'campaign-editor']) el(id).hidden = true;
  status('');
 }

 const isCurrentControl = control => Number(control?.dataset?.controllerVersion) === serial;

 async function changed(message, route, version) {
  try { await onChanged(); if (serial === version) status(message); }
  catch (failure) { if (serial === version) status(`${message} 受众占用刷新失败：${failure.message}`); }
  if (serial === version) onNavigate(route);
 }

 section.addEventListener('click', async event => {
  const button = event.target.closest('button');
  if (!button) return;
   if (button.hasAttribute('data-cancel-campaign')) { if (!isCurrentControl(button)) return; onNavigate(activity ? detailRoute() : 'activities'); return; }
  if (button.dataset.deleteActivity) {
   if (!isCurrentControl(button)) return;
   if (!window.confirm('删除整项活动及其投放项、活动流程边？')) return;
   const version = serial;
   try { await api.delete(button.dataset.deleteActivity); await changed('活动已删除。', 'activities', version); }
   catch (failure) { if (serial === version) status(`删除失败：${failure.message}`); }
  }
  if (button.dataset.deleteTreatment && activity) {
   if (!isCurrentControl(button)) return;
   if (!window.confirm('删除该投放项？')) return;
   const version = serial, campaignId = activity.id, route = detailRoute();
   try { await api.deleteTreatment(campaignId, button.dataset.deleteTreatment); await changed('投放项已删除。', route, version); }
   catch (failure) { if (serial === version) status(`删除失败：${failure.message}`); }
  }
 });

 section.addEventListener('submit', async event => {
  if (!['campaign-form', 'treatment-form'].includes(event.target.id)) return;
  event.preventDefault();
  if (!isCurrentControl(event.target)) return;
  const version = serial;
  const button = event.target.querySelector('button[type=submit]');
  if (button) button.disabled = true;
  try {
   if (event.target.id === 'campaign-form') {
    const name = el('campaign-name').value.trim();
    if (!name) throw Error('活动名称必填。');
    const audience = selectionFromKey(el('campaign-audience').value);
    if (!audience) throw Error('请选择活动受众。');
    let saved;
    if (activity) {
     saved = await api.update(activity.id, { ...activity, name });
     if (selectionKey(audience) !== selectionKey(activity.audience)) saved = await api.updateAudience(activity.id, audience, activity.audience ?? null);
    } else {
     saved = await api.create({ name });
     saved = await api.updateAudience(saved.id, audience, null);
    }
    await changed('活动已保存。', `activities/${url(saved.id)}/campaign`, version);
   } else {
    const value = { name: el('treatment-name').value.trim(), actionId: el('treatment-action').value,
     ...(el('treatment-slot').value ? { slotId: el('treatment-slot').value } : {}) };
    const problem = validateTreatmentDraft(value, actions, references(), activity.treatments.filter(item => item.id !== treatment?.id));
    if (problem) throw Error(problem);
    const campaignId = activity.id, treatmentId = treatment?.id, route = detailRoute();
    if (treatmentId) await api.updateTreatment(campaignId, treatmentId, { id: treatmentId, ...value });
    else await api.createTreatment(campaignId, value);
    await changed('投放项已保存。', route, version);
   }
  } catch (failure) { if (serial === version) error(failure.message); }
  finally { if (button) button.disabled = false; }
 });
 return { render, clear };
}
