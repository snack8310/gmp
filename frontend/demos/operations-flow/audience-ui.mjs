import { MOCK_USERS, createAudienceStore, validateAudience, filterAudience, summarizeRules, assignGroup, summarizeEntry } from './audience.mjs';
import { mountExperimentPicker, escapeHTML } from './experiment-ui.mjs';

export function mountAudiences({ onUse, onSaved, onDeleted, onNavigate }) {
 const el = id => document.getElementById(id);
 let storage;
 try { storage = window.localStorage; } catch { storage = null; }
 const store = createAudienceStore(storage);
 let editing = null;
 const picker=mountExperimentPicker({rootId:'audience-experiment-picker',store,onChange:()=>preview()});
 const lifecycleLabel = {new:'新用户',active:'活跃用户',dormant:'沉睡用户'};
 function list() {
  const items = store.list(), query = String(el('audience-search').value || '').trim().toLowerCase();
  el('audience-count').textContent = items.length;
  const filtered = items.filter(a => a.name.toLowerCase().includes(query));
  el('audience-list').innerHTML = filtered.map(a => `<article class="audience-card"><div class="audience-card-icon" aria-hidden="true">♙</div><div><h3>${escapeHTML(a.name)}</h3><small>${escapeHTML(a.id)} · 已保存定义</small></div><p>${escapeHTML(summarizeEntry(a))}<br>${escapeHTML(summarizeRules(a.rules))}<br><span class="audience-experiment-tag">${a.experimentId?`引用实验 · ${escapeHTML(store.experiments().find(e=>e.id===a.experimentId)?.name||a.experimentId)}`:'未引用实验 · 单一出口'}</span></p><div class="card-count">${filterAudience(a.rules).length}<span>模拟匹配</span></div><div class="card-actions"><button data-edit-audience="${escapeHTML(a.id)}" aria-label="编辑 ${escapeHTML(a.name)}">编辑</button><button data-delete-audience="${escapeHTML(a.id)}" aria-label="删除 ${escapeHTML(a.name)}">删除</button><button class="use-audience" data-use-audience="${escapeHTML(a.id)}" aria-label="将 ${escapeHTML(a.name)} 引用到组合">引用到组合 ↗</button></div></article>`).join('') || `<div class="empty-state">${items.length ? '没有找到匹配的人群名称。<br>试试其他关键词，或清空搜索。' : '还没有保存的人群。<br>点击「新建人群」，从一组条件开始。'}</div>`;
  el('storage-warning').textContent = store.warning;
 }
 function draft() {
  const number = id => String(el(id).value).trim() === '' ? NaN : Number(el(id).value);
  const entry=el('entry-mode').value==='event'?{mode:'event',eventType:el('event-type').value}:{mode:el('entry-mode').value,schedule:el('batch-schedule').value==='daily'?{type:'daily',time:el('batch-time').value,timezone:'UTC'}:{type:el('batch-schedule').value}};
  return {entry,id:editing || '',name:String(el('audience-name').value),experimentId:el('audience-experiment-enabled').checked?picker.getValue():null,rules:{source:el('rule-source').value,country:el('rule-country').value,lifecycle:el('rule-lifecycle').value,minInactive:number('rule-min'),maxInactive:number('rule-max')}};
 }
 function preview() {
  const value = draft(), errors = validateAudience(value,store.experiments());
  // A temporary name keeps incomplete naming separate from rule preview validity.
  const ruleErrors = validateAudience({...value,name:'预览',experimentId:null,entry:{mode:'batch',schedule:{type:'manual'}}});
  const users = ruleErrors.length ? [] : filterAudience(value.rules);
  el('batch-entry-fields').hidden=value.entry.mode!=='batch';el('event-entry-fields').hidden=value.entry.mode!=='event';el('batch-time-fields').hidden=value.entry.mode!=='batch'||value.entry.schedule?.type!=='daily';
  el('entry-mode-help').textContent=value.entry.mode==='event'?'事件到来时判定资格；模式和触发配置随这份人群一起复用。':'按手动或每日配置从来源圈选；画布不再设置模式或排期。';
  el('preview-count-label').textContent=value.entry.mode==='event'?'位模拟用户具备事件进入资格':'位模拟用户符合批量条件';
  el('preview-entry').textContent=summarizeEntry(value);
  el('editor-error').textContent = errors.join('；');
  if(el('audience-experiment-enabled').checked&&!value.experimentId)errors.push('请选择或新建一个实验。');
  el('editor-error').textContent=errors.join('；');
  el('audience-experiment-config').hidden=!el('audience-experiment-enabled').checked;
  el('save-audience').disabled = errors.length > 0;
  el('preview-count').textContent = ruleErrors.length ? '—' : users.length;
  el('preview-bar').style.width = `${users.length/MOCK_USERS.length*100}%`;
  el('preview-ratio').textContent = ruleErrors.length ? '请先修正规则，再预览人数。' : `占模拟样本池 ${(users.length/MOCK_USERS.length*100).toFixed(1)}% / ${MOCK_USERS.length} 位`;
  const experiment=store.experiments().find(e=>e.id===value.experimentId);
  const groups={a:0,b:0,control:0};if(experiment)for(const user of users)groups[assignGroup(user.uid,experiment.id)]++;
  el('preview-assignment').textContent=experiment?`引用 ${experiment.name} · 分母 ${users.length} 位符合条件用户 → A ${groups.a} / B ${groups.b} / 对照 ${groups.control}（不触达）`:el('audience-experiment-enabled').checked?'请选择实验后预览分配。':'未引用实验 · 符合条件用户从单一出口进入画布';
  el('preview-rules').textContent = ruleErrors.length ? '条件暂不可计算' : summarizeRules(value.rules);
  el('preview-message').textContent = ruleErrors.length ? ruleErrors.join('；') : users.length ? value.entry.mode==='event'?'以下是可判定资格样本，不是已收到的事件或真实进入人数；不模拟事件分发。':'所有条件同时满足，以下是符合条件的构造样本。' : '当前条件没有匹配用户。可放宽条件，或保存这个空人群；空人群不能播放触达。';
  el('preview-samples').innerHTML = users.slice(0,6).map(u=>`<tr><td>${escapeHTML(u.uid)}</td><td>${escapeHTML(u.country)}</td><td>${lifecycleLabel[u.lifecycle]}</td><td>${u.inactiveDays} 天</td></tr>`).join('') || '<tr><td colspan="4">暂无可展示样本</td></tr>';
 }
 function edit(id = null) {
  const item = id ? store.list().find(a=>a.id===id) : null;
  if (id && !item) return;
  editing = item?.id || null;
  const value = item || {name:'',entry:{mode:'batch',schedule:{type:'manual'}},rules:{source:'app',country:'all',lifecycle:'all',minInactive:0,maxInactive:365}};
  el('editor-title').textContent = item ? '编辑人群定义' : '新建人群';
  el('entry-mode').value=value.entry.mode;el('batch-schedule').value=value.entry.mode==='batch'?value.entry.schedule.type:'manual';el('batch-time').value=value.entry.mode==='batch'&&value.entry.schedule.type==='daily'?value.entry.schedule.time:'09:00';el('event-type').value=value.entry.mode==='event'?value.entry.eventType:'app_opened';
  el('audience-name').value=value.name;el('audience-experiment-enabled').checked=Boolean(value.experimentId);picker.setValue(value.experimentId);
  for(const [key,field] of [['source','source'],['country','country'],['lifecycle','lifecycle'],['minInactive','min'],['maxInactive','max']])el(`rule-${field}`).value=value.rules[key];
  el('audience-browse').hidden=true;el('audience-editor').hidden=false;el('audience-status').textContent='';
  onNavigate('audience');preview();el('audience-name').focus?.();
 }
 function close(message) { editing=null;el('audience-editor').hidden=true;el('audience-browse').hidden=false;el('audience-status').textContent=message;list();el('new-audience').focus?.(); }
 function use(id) { const item=store.list().find(a=>a.id===id);if(item){onUse(item);onNavigate('compose');} }
 el('new-audience').addEventListener('click',()=>edit());
 el('cancel-audience').addEventListener('click',()=>close('已取消，保存的定义与沙盘均未改变。'));
 el('audience-search').addEventListener('input',list);
 el('audience-list').addEventListener('click',e=>{const editButton=e.target.closest('[data-edit-audience]'),useButton=e.target.closest('[data-use-audience]'),deleteButton=e.target.closest('[data-delete-audience]');if(editButton)edit(editButton.dataset.editAudience);else if(useButton)use(useButton.dataset.useAudience);else if(deleteButton){const id=deleteButton.dataset.deleteAudience;try{store.delete(id);list();onDeleted(id,store.list());el('audience-status').textContent='人群已删除。';}catch(error){el('audience-status').textContent=error.message;}}});
 el('audience-form').addEventListener('input',preview);
 el('audience-form').addEventListener('change',preview);
 el('audience-form').addEventListener('submit',e=>{e.preventDefault();const value=draft();if(validateAudience(value,store.experiments()).length||(el('audience-experiment-enabled').checked&&!value.experimentId)){preview();return;}let saved;try{saved=store.save(value);}catch(error){el('editor-error').textContent=error.message;return;}close(store.warning?'人群定义已保存；请留意下方存储或兼容提示。':'人群定义已保存，可引用到组合沙盘。');onSaved(saved,store.list());});
 list();return {list:()=>store.list(),edit,store,refreshExperiments:()=>{picker.refresh();list();if(!el('audience-editor').hidden)preview();}};
}
