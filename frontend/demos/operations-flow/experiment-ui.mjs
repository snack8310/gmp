import { DEFAULT_SPLIT, MOCK_USERS, validateExperiment, validateSplit } from './audience.mjs';

export const escapeHTML = value => String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

const ratio = split => `A 组 ${split.a}% · B 组 ${split.b}% · 对照组 ${split.control}%`;
const policy = colored => colored ? '记录首次分配' : '不记录分配';
const groupLabel = part => part === 'control' ? '对照组' : `${part.toUpperCase()} 组`;
const routeFor = (id, returnTo) => `#experimentation${id ? '/' + encodeURIComponent(id) : ''}${returnTo ? '?return=' + encodeURIComponent(returnTo) : ''}`;

export function mountExperimentPicker({rootId,store,onChange,onNavigate}) {
 const el=id=>document.getElementById(`${rootId}-${id}`);
 const root=document.getElementById(rootId);
 root.innerHTML=`<div class="experiment-picker"><label for="${rootId}-select">引用已有实验</label><select id="${rootId}-select" aria-label="选择复用实验"></select><p id="${rootId}-summary" class="experiment-fixed"></p><a href="#experimentation" data-domain-nav="experimentation">前往实验库管理 ↗</a></div>`;
 root.addEventListener('click',event=>{const link=event.target.closest('[data-domain-nav]');if(link){event.preventDefault();onNavigate?.(link.dataset.domainNav);}});
 function refresh(id=el('select').value){const values=store.experiments();el('select').innerHTML=values.map(e=>`<option value="${escapeHTML(e.id)}">${escapeHTML(e.name)} · ${escapeHTML(e.id)}</option>`).join('')||'<option value="">请先在实验域新建实验</option>';el('select').value=values.some(e=>e.id===id)?id:values[0]?.id||'';const selected=values.find(e=>e.id===el('select').value);el('summary').textContent=selected?`${ratio(selected.split)} · ${policy(selected.colored)} · 仅本地模拟`:'尚无实验';}
 el('select').addEventListener('change',()=>{refresh(el('select').value);onChange?.(el('select').value);});
 refresh();return {getValue:()=>el('select').value||null,setValue:id=>refresh(id),refresh};
}

export function mountExperiments({store,flowDraftStore,onChanged}) {
 const root=document.getElementById('experiment-page');
 let route={assetView:'list',experimentId:null,returnTo:null};
 let query='';
 let message='';
 const listLink=()=>routeFor(null,route.returnTo);
 const detailLink=id=>routeFor(id,route.returnTo);
 const newLink=()=>routeFor('new',route.returnTo);
 const notify=error=>{message=error.message||String(error);render(route);};
 const field=(id,label,value)=>`<label class="experiment-field" for="${id}">${label}<input id="${id}" name="${id}" type="number" min="0" max="100" step="1" value="${value}" required></label>`;
 const splitFields=(split,prefix)=>`<div class="experiment-split-fields">${field(`${prefix}-a`,'A 组占比 (%)',split.a)}${field(`${prefix}-b`,'B 组占比 (%)',split.b)}${field(`${prefix}-control`,'对照组占比 (%)',split.control)}</div>`;
 const readSplit=prefix=>{const number=part=>{const value=root.querySelector(`#${prefix}-${part}`)?.value;return value===''||value===undefined?NaN:Number(value);};return {a:number('a'),b:number('b'),control:number('control')};};
 const shell=(title,body,back=false,experiment=null)=>`${back?`<a class="campaign-back experiment-back" href="${listLink()}">← 返回实验列表</a>`:''}<div class="section-heading experiment-page-head"><div><p class="eyebrow">EXPERIMENTATION / LOCAL DEMO${experiment?` · ${escapeHTML(experiment.id)}`:''}</p><h2>${escapeHTML(title)}</h2><p>配置仅保存在本浏览器，使用构造样本验证分流；不会写入 Core 或生产染色。</p></div>${experiment?`<div class="experiment-actions"><span class="experiment-badge ${experiment.runtimeStatus}">${experiment.runtimeStatus==='running'?'运行中':'已停用'}</span><button data-action="toggle" type="button">${experiment.runtimeStatus==='running'?'停用分流':'运行分流'}</button><button data-action="delete" type="button">删除实验</button></div>`:back?'':`<a class="primary-link experiment-primary-link" href="${newLink()}">＋ 新建实验</a>`}</div>${store.warning?`<p class="storage-warning" role="status">${escapeHTML(store.warning)}</p>`:''}${message?`<p class="inline-error" role="status">${escapeHTML(message)}</p>`:''}${body}`;
 function listMarkup(){
  const values=store.experiments();
  const filtered=values.filter(e=>`${e.name} ${e.id}`.toLowerCase().includes(query.toLowerCase()));
  const rows=filtered.map(e=>`<tr class="experiment-row"><th scope="row">${escapeHTML(e.name)} <small>· ${escapeHTML(e.id)}</small></th><td><span class="experiment-badge ${e.runtimeStatus}">${e.runtimeStatus==='running'?'运行中':'已停用'}</span></td><td>${ratio(e.split)}</td><td>${policy(e.colored)}</td><td>${store.bundles().filter(b=>b.experimentId===e.id).length}</td><td><a href="${detailLink(e.id)}">查看详情</a></td></tr>`).join('')||'<tr><td colspan="6" class="experiment-empty">没有匹配的实验。可调整搜索词或新建实验。</td></tr>';
  return `<div class="audience-list-toolbar experiment-list-toolbar"><label for="experiment-search">查找实验<input id="experiment-search" type="search" placeholder="名称或 ID" value="${escapeHTML(query)}"></label><span>共 ${values.length} 项</span></div><div id="experiment-rows" class="campaign-table-scroll" tabindex="0" aria-label="实验列表表格，可横向滚动"><table class="campaign-table experiment-table"><thead><tr><th scope="col">实验名称</th><th scope="col">运行态</th><th scope="col">分流比例</th><th scope="col">染色策略</th><th scope="col">人群包</th><th scope="col">操作</th></tr></thead><tbody>${rows}</tbody></table></div>`;
 }
 function detailMarkup(e){
  const records=store.experimentColoring(e.id),parts=Object.values(records),bundles=store.bundles().filter(b=>b.experimentId===e.id);
  const stopped=e.runtimeStatus!=='running', disabled=stopped?' disabled':'';
  const samples=MOCK_USERS.slice(0,12).map(user=>`<tr><td>${user.uid}</td><td>${groupLabel(store.peekExperimentGroup(e.id,user.uid))}</td><td>${Object.hasOwn(records,user.uid)?'已模拟记录':'只读预测'}</td></tr>`).join('');
  return `<div class="experiment-detail-grid">
   <section class="campaign-detail-block experiment-panel"><h3>基本信息</h3><form data-form="rename"><label for="experiment-rename">实验名称</label><div class="experiment-inline"><input id="experiment-rename" maxlength="48" value="${escapeHTML(e.name)}" required><button type="submit">保存名称</button></div></form><p>稳定 ID：<code>${escapeHTML(e.id)}</code></p><p>染色策略：<strong>${policy(e.colored)}</strong>。创建后不可修改。</p><p>算法与盐仅为浏览器演示，分桶结果不代表 Core 实际结果。白名单、算法版本与盐、实验指标和放量配置尚未接入 Core。</p></section>
   <section class="campaign-detail-block experiment-panel"><h3>分流比例</h3><p>当前 ${ratio(e.split)}。${stopped?'实验停用时不可调整比例。':'运行中调整不会使已发布人群包失效。'}</p><form data-form="split">${splitFields(e.split,'edit')}<button class="primary" type="submit"${disabled}>保存比例</button></form></section>
   <section class="campaign-detail-block experiment-panel"><div class="experiment-panel-head"><div><h3>构造用户分组预览</h3><p>只读预测不写入染色。${stopped?'实验停用时不可模拟分配。':e.colored?'单独点击模拟首次分配才记录，已记录者保持原分组。':'此实验选择不记录分配，模拟操作不会写入染色。'}</p></div><button data-action="assign" type="button"${disabled}>${e.colored?'模拟首次分配':'模拟一次分配'}</button></div><p>本地已记录 ${parts.length} 位：A 组 ${parts.filter(x=>x==='a').length} · B 组 ${parts.filter(x=>x==='b').length} · 对照组 ${parts.filter(x=>x==='control').length}</p><div class="sample-scroll"><table class="sample-table"><thead><tr><th>构造 ID</th><th>当前分组</th><th>来源</th></tr></thead><tbody>${samples}</tbody></table></div></section>
   <section class="campaign-detail-block experiment-panel"><h3>已发布人群包</h3>${bundles.length?bundles.map(b=>`<p>${escapeHTML(b.name)} · ${escapeHTML(b.id)} · ${b.generation===e.generation&&e.runtimeStatus==='running'?'可用':'旧代际或停用，不可用'}</p>`).join(''):'<p>尚无人群包引用此实验。</p>'}</section></div>`;
 }
 function newMarkup(){return `<section class="campaign-detail-block experiment-panel experiment-create"><form data-form="create" novalidate><label for="experiment-name">实验名称</label><input id="experiment-name" maxlength="48" autocomplete="off" placeholder="例如：召回策略实验" required><h3>初始分流比例</h3>${splitFields(DEFAULT_SPLIT,'create')}<fieldset><legend>染色策略（必选）</legend><label><input type="radio" name="colored" value="true">记录首次分配</label><label><input type="radio" name="colored" value="false">不记录分配</label></fieldset><p>新实验初始为停用态；运行后染色策略不可切换。</p><button class="primary" type="submit">创建实验</button></form></section>`;}
 function render(next=route){route=next;const e=store.experiments().find(item=>item.id===route.experimentId);root.innerHTML=route.assetView==='new'?shell('新建实验',newMarkup(),true):route.assetView==='detail'?shell(e?e.name:'未找到实验',e?detailMarkup(e):`<p class="empty-state">该实验 ID 不存在或无效。<a href="${listLink()}">返回实验列表</a></p>`,true,e):shell('实验列表',listMarkup());
  if(e&&route.assetView==='detail')root.querySelector('[data-action="assign"]')?.insertAdjacentHTML?.('beforebegin',`<label class="experiment-sample-picker" for="experiment-sim-id">构造 ID<select id="experiment-sim-id">${MOCK_USERS.slice(0,12).map(user=>`<option value="${user.uid}">${user.uid}</option>`).join('')}</select></label>`);
 }
 root.addEventListener('input',event=>{if(event.target.id==='experiment-search'){query=event.target.value;const fresh=listMarkup();const parsed=document.createElement('div');parsed.innerHTML=fresh;root.querySelector('#experiment-rows').innerHTML=parsed.querySelector('#experiment-rows').innerHTML;}});
 root.addEventListener('submit',event=>{
  const form=event.target.closest('[data-form]');if(!form)return;event.preventDefault();message='';
  try{
   if(form.dataset.form==='create'){
    const colored=form.querySelector('input[name="colored"]:checked');const draft={name:root.querySelector('#experiment-name').value,split:readSplit('create'),colored:colored?colored.value==='true':undefined};
    const errors=validateExperiment(draft);if(errors.length)throw new RangeError(errors.join('；'));
    const saved=store.saveExperiment(draft);onChanged?.();window.location.hash=detailLink(saved.id);return;
   }
   const id=route.experimentId;if(!id||!store.experiments().some(e=>e.id===id))throw new RangeError('实验不存在。');
   if(form.dataset.form==='rename')store.saveExperiment({id,name:root.querySelector('#experiment-rename').value});
   if(form.dataset.form==='split'){const split=readSplit('edit');const errors=validateSplit(split);if(errors.length)throw new RangeError(errors.join('；'));store.updateExperimentSplit(id,split);}
   onChanged?.();message=store.warning?'修改仅保留在当前会话。':'已保存到本浏览器。';render(route);
  }catch(error){notify(error);}
 });
 root.addEventListener('click',event=>{
  const button=event.target.closest('[data-action]');if(!button||!root.contains(button))return;
  const id=route.experimentId;const experiment=store.experiments().find(item=>item.id===id);if(!experiment)return;
  try{
   if(button.dataset.action==='toggle')store.setExperimentStatus(id,experiment.runtimeStatus==='running'?'stopped':'running');
   if(button.dataset.action==='delete'){
    const flowRefs=flowDraftStore?.references(id) || [];
    if(flowRefs.length)throw new RangeError(`该实验被 ${flowRefs.length} 个活动流程节点引用，请先移除节点。`);
    store.deleteExperiment(id);onChanged?.();window.location.hash=listLink();return;
   }
   if(button.dataset.action==='assign'){
    const selected=root.querySelector('#experiment-sim-id')?.value;
    const first=MOCK_USERS.find(user=>user.uid===selected)??MOCK_USERS[0];
    if(!first)throw new RangeError('构造样本均已模拟分配。');
    const part=store.assignExperimentGroup(id,first.uid);message=store.warning?'分配仅保留在当前会话，刷新后可能丢失。':experiment.colored?`已模拟 ${first.uid} 首次分配到${groupLabel(part)}。`:`${first.uid} 本次模拟分配到${groupLabel(part)}；未记录染色。`;
   }else message=store.warning?'状态修改仅保留在当前会话，刷新后可能丢失。':'已更新运行状态。';
   onChanged?.();render(route);
  }catch(error){notify(error);}
 });
 render();return {render,refresh:()=>render(route)};
}
