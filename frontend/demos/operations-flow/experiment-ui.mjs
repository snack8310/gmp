import { validateExperiment } from './audience.mjs';

export const escapeHTML = value => String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

export function mountExperimentPicker({rootId,store,onChange}) {
 const el=id=>document.getElementById(`${rootId}-${id}`);
 document.getElementById(rootId).innerHTML=`<div class="experiment-picker"><label for="${rootId}-select">引用已有实验</label><select id="${rootId}-select" aria-label="选择复用实验"></select><p class="experiment-fixed">演示固定 A 45% / B 45% / 对照 10%<br>同一实验 ID + 同一模拟用户 → 相同分组</p><a href="#experimentation">前往实验域管理 ↗</a></div>`;
 function refresh(id=el('select').value){const values=store.experiments();el('select').innerHTML=values.map(e=>`<option value="${escapeHTML(e.id)}">${escapeHTML(e.name)} · ${escapeHTML(e.id)}</option>`).join('')||'<option value="">请先在实验域新建实验</option>';el('select').value=values.some(e=>e.id===id)?id:values[0]?.id||'';}
 el('select').addEventListener('change',()=>onChange?.(el('select').value));
 refresh();return {getValue:()=>el('select').value||null,setValue:id=>refresh(id),refresh};
}

export function mountExperiments({store,onChanged}) {
 const el=id=>document.getElementById(id);
 let editingId=null;
 function refresh(){
  const experiments=store.experiments();
  el('experiment-count').textContent=experiments.length;
  el('experiment-list').innerHTML=experiments.map(experiment=>`<article class="domain-card"><span class="eyebrow">${escapeHTML(experiment.id)}</span><h3>${escapeHTML(experiment.name)}</h3><p>A 45% · B 45% · 对照 10% · ${experiment.runtimeStatus === 'running' ? '运行中' : '已停用'}</p><div class="card-actions"><button type="button" data-toggle-experiment="${escapeHTML(experiment.id)}">${experiment.runtimeStatus === 'running' ? '停用分流' : '运行分流'}</button><button type="button" data-edit-experiment="${escapeHTML(experiment.id)}">改名</button><button type="button" data-delete-experiment="${escapeHTML(experiment.id)}">删除</button></div></article>`).join('')||'<p class="empty-state">尚无实验，创建后即可被受众或组合引用。</p>';
  el('experiment-storage-warning').textContent=store.warning;
 }
 function open(id=null){const experiment=store.experiments().find(value=>value.id===id);editingId=experiment?.id||null;el('experiment-name').value=experiment?.name||'';el('experiment-form').hidden=false;el('experiment-error').textContent='';el('experiment-name').focus?.();}
 function close(){editingId=null;el('experiment-form').hidden=true;el('experiment-error').textContent='';}
 el('new-experiment').addEventListener('click',()=>open());
 el('cancel-experiment').addEventListener('click',close);
 el('experiment-list').addEventListener('click',event=>{
  const edit=event.target.closest('[data-edit-experiment]'),remove=event.target.closest('[data-delete-experiment]'),toggle=event.target.closest('[data-toggle-experiment]');
  if(toggle){const experiment=store.experiments().find(item=>item.id===toggle.dataset.toggleExperiment);try{store.setExperimentStatus(experiment.id,experiment.runtimeStatus==='running'?'stopped':'running');refresh();onChanged?.();el('experiment-status').textContent=experiment.runtimeStatus==='running'?'分流已停用；旧包臂不可用，再运行也不会复活。':'分流运行中；可发布新包。';}catch(error){el('experiment-status').textContent=error.message;}}
  else if(edit)open(edit.dataset.editExperiment);
  else if(remove){try{store.deleteExperiment(remove.dataset.deleteExperiment);close();refresh();onChanged?.();el('experiment-status').textContent='实验已删除。';}catch(error){el('experiment-status').textContent=error.message;}}
 });
 el('experiment-form').addEventListener('submit',event=>{
  event.preventDefault();
  const definition={id:editingId||'',name:String(el('experiment-name').value)},errors=validateExperiment(definition);
  if(errors.length){el('experiment-error').textContent=errors.join('；');return;}
  try{store.saveExperiment(definition);close();refresh();onChanged?.();el('experiment-status').textContent='实验已保存，可在受众和组合中引用。';}
  catch(error){el('experiment-error').textContent=error.message;}
 });
 refresh();return {refresh};
}
