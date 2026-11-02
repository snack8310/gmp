// Browserless handler checks complement, but do not replace, native browser QA.
import test from 'node:test';
import assert from 'node:assert/strict';
import { ACTIVITIES, DEFAULT_AUDIENCE_CATALOG } from './campaign-activities.mjs';
import { mountAudiences } from './audience-ui.mjs';
import { STORAGE_KEY } from './audience.mjs';
const elements = new Map(), pending = new Map();
let frameID = 0, captures = 0;
const browserStorage = new Map(), storageWrites = [];
function element(id) {
 if (!elements.has(id)) elements.set(id, {
  innerHTML: '', textContent: '', value: 0, style: {}, dataset: {}, listeners: {}, attributes:{},
  classList: { toggle() {}, add() {}, remove() {} }, setAttribute(name,value) {this.attributes[name]=value;},
  addEventListener(name, fn) { this.listeners[name] = fn; },
  setPointerCapture() { captures++; },
 });
 return elements.get(id);
}
const media = { matches: false, addEventListener(name,fn) { this.change=fn; } };
globalThis.document = { getElementById:element, querySelector:element, querySelectorAll:()=>[], addEventListener() {} };
globalThis.window = { location:{hash:'#activities/campaign-branch/campaign',protocol:'http:',origin:'http://127.0.0.1:9191'}, localStorage:{getItem:key=>browserStorage.get(key)??null,setItem:(key,value)=>{storageWrites.push(key);browserStorage.set(key,value);},removeItem:key=>browserStorage.delete(key)}, addEventListener(name,fn) {this.listeners[name]=fn;}, listeners:{} };
globalThis.matchMedia = () => media;
globalThis.requestAnimationFrame = fn => { pending.set(++frameID,fn); return frameID; };
globalThis.cancelAnimationFrame = id => pending.delete(id);
const apiCampaigns = structuredClone(ACTIVITIES).map(activity => ({ ...activity, treatments: activity.treatments.map(({ actionRef, audienceRef, ...item }) => ({ ...item, actionId: actionRef.id, audienceRef: audienceRef.type === 'arm' ? { type: 'arm', bundleId: audienceRef.bundleId, groupId: audienceRef.groupId } : { type: 'group', groupId: audienceRef.groupId } })) }));
const apiActions = ['rcs', 'email', 'sms'].map(id => ({ id: `action-${id}`, name: id, available: true, requiresSlot: false }));
const apiRefs = DEFAULT_AUDIENCE_CATALOG.map(item => ({ type: item.type, groupId: item.groupId, ...(item.bundleId ? { bundleId: item.bundleId, bundleName: item.bundleName } : {}), name: item.armName || item.groupName, available: item.available }));
let nextFailure = false, writes = 0, usageCalls = 0, deferTreatment = false, releaseTreatment, deferCampaignList = false, releaseCampaignList;
const apiAddresses = [];
globalThis.fetch = async (address, options = {}) => {
 apiAddresses.push(address);
 const parsed = new URL(address), path = parsed.pathname;
 const method = options.method || 'GET', body = options.body && JSON.parse(options.body);
 const response = (status, value) => ({ status, ok: status >= 200 && status < 300, json: async () => value });
 if (nextFailure && method !== 'GET') { nextFailure = false; return response(422, { error: { code: 'unknown_audience', message: '服务端拒绝保存' } }); }
 if (path === '/api/v1/campaigns' && method === 'POST') { const created = { id: `campaign-test-${++writes}`, name: body.name, treatments: [] }; apiCampaigns.push(created); return response(201, structuredClone(created)); }
 if (path === '/api/v1/campaigns' && method === 'GET' && deferCampaignList) return new Promise(resolve=>{releaseCampaignList=()=>{deferCampaignList=false;resolve(response(200,{campaigns:structuredClone(apiCampaigns)}));};});
 if (path === '/api/v1/campaigns') return response(200, { campaigns: structuredClone(apiCampaigns) });
 if (path === '/api/v1/actions') return response(200, { actions: apiActions });
 if (path === '/api/v1/audience-references') return response(200, { audienceReferences: apiRefs });
 if (path === '/api/v1/audience-reference-usages') { usageCalls++; return response(200, { usages: apiCampaigns.flatMap(activity => activity.treatments.filter(item => item.audienceRef?.type === parsed.searchParams.get('type') && item.audienceRef.groupId === parsed.searchParams.get('groupId') && (!parsed.searchParams.has('bundleId') || item.audienceRef.bundleId === parsed.searchParams.get('bundleId'))).map(item => ({ campaignId: activity.id, treatmentId: item.id }))) }); }
 if (path.startsWith('/api/v1/campaigns/')) { const parts=path.slice('/api/v1/campaigns/'.length).split('/'); const id = decodeURIComponent(parts[0]); const campaign = apiCampaigns.find(item => item.id === id);
  if (!campaign) return response(404, { error: { code: 'not_found', message: '活动不存在' } });
  if (parts[1] === 'treatments') {
   if (method === 'POST') { if (deferTreatment) return new Promise(resolve=>{releaseTreatment=()=>{deferTreatment=false;const added={...body,id:`treatment-test-${++writes}`};campaign.treatments.push(added);resolve(response(201,structuredClone(added)));};}); const added={...body,id:`treatment-test-${++writes}`}; campaign.treatments.push(added); return response(201,structuredClone(added)); }
   const current=campaign.treatments.find(item=>item.id===decodeURIComponent(parts[2]));
   if (!current) return response(404,{error:{code:'not_found',message:'投放项不存在'}});
   if (method === 'PUT') { writes++;Object.assign(current,body);return response(200,structuredClone(current)); }
   if (method === 'DELETE') { writes++;campaign.treatments.splice(campaign.treatments.indexOf(current),1);return response(204); }
  }
  if (method === 'PUT') { writes++; campaign.name = body.name; return response(200, structuredClone(campaign)); }
  if (method === 'DELETE') { writes++; apiCampaigns.splice(apiCampaigns.indexOf(campaign), 1); return response(204); }
  return response(200, structuredClone(campaign)); }
 throw Error(`Unexpected API route: ${path}`);
};
await import('./app.mjs');
const settle = () => new Promise(resolve => setImmediate(resolve));
const fire = (id,event,data={}) => element(id).listeners[event](data);
const pointer = (id, extra={}) => ({pointerType:'mouse',button:0,clientX:100,clientY:100,pointerId:1,target:{closest:()=>id?{dataset:{node:id}}:null},...extra});
const controllerVersion = id => Number(element(id).innerHTML.match(/data-controller-version="(\d+)"/)?.[1]);
const formTarget = id => ({id,dataset:{controllerVersion:String(controllerVersion('campaign-editor'))},querySelector:()=>({disabled:false})});

test('app requests the campaign API from the page origin by default', async () => {
 await settle();
 assert.ok(apiAddresses.length > 0);
 assert.ok(apiAddresses.every(address => new URL(address).origin === window.location.origin));
});

test('audience definitions can be created, previewed, canceled and saved', () => {
 fire('new-audience','click');
 assert.equal(element('audience-editor').hidden,false);
 element('audience-name').value='Temporary draft';element('rule-country').value='MX';fire('audience-form','input');
 assert.ok(Number(element('preview-count').textContent)<192);
 fire('cancel-audience','click');assert.equal(element('audience-editor').hidden,true);
 fire('new-audience','click');assert.equal(element('audience-name').value,'');
 element('audience-name').value='Saved test';element('rule-country').value='MX';fire('audience-form','input');
 const count=Number(element('preview-count').textContent);
 fire('audience-form','submit',{preventDefault(){}});
 assert.equal(element('audience-editor').hidden,true);
 assert.match(element('audience-list').innerHTML,/Saved test/);
 const card=element('audience-list').innerHTML.split('Saved test')[1];
 assert.equal(Number(card.match(/<div class=\"card-count\">(\d+)/)?.[1]),count);
});

test('experiment page creates and renames identities reused by audience definitions',async()=>{
 window.location.hash='#experimentation';window.listeners.hashchange();
 await settle();
 assert.equal(element('experimentation-section').hidden,false);
 assert.equal(element('module-experimentation').attributes['aria-current'],'page');
 assert.equal(element('experiment-audience-link').href,'#audience');
 fire('new-experiment','click');element('experiment-name').value='Reusable demo';
 fire('experiment-form','submit',{preventDefault(){}});
 const experimentId=element('experiment-list').innerHTML.match(/exp-custom-\d+/)?.[0];
 assert.ok(experimentId);
 assert.ok(element('audience-experiment-picker-select').innerHTML.includes(experimentId));
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-edit-experiment]'?{dataset:{editExperiment:experimentId}}:null}});
 element('experiment-name').value='Canceled rename';fire('cancel-experiment','click');
 assert.ok(!element('experiment-list').innerHTML.includes('Canceled rename'));
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-edit-experiment]'?{dataset:{editExperiment:experimentId}}:null}});
 element('experiment-name').value='Renamed shared';fire('experiment-form','submit',{preventDefault(){}});
 assert.ok(element('audience-experiment-picker-select').innerHTML.includes('Renamed shared'));
 fire('new-experiment','click');element('experiment-name').value='Disposable';fire('experiment-form','submit',{preventDefault(){}});
 const disposable=element('experiment-list').innerHTML.match(/exp-custom-\d+/g).at(-1);
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-delete-experiment]'?{dataset:{deleteExperiment:disposable}}:null}});
 assert.ok(!element('experiment-list').innerHTML.includes('Disposable'));
 assert.ok(!element('audience-experiment-picker-select').innerHTML.includes(disposable));
 window.location.hash='#audience';window.listeners.hashchange();await settle();
 fire('new-audience','click');element('audience-experiment-enabled').checked=true;
 element('audience-experiment-picker-select').value=experimentId;fire('audience-form','change');
 element('audience-name').value='引用共享实验';fire('audience-form','submit',{preventDefault(){}});
 assert.ok(element('audience-list').innerHTML.includes('Renamed shared'));
 assert.equal(element('audience-section').hidden,false);
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-delete-experiment]'?{dataset:{deleteExperiment:experimentId}}:null}});
 assert.match(element('experiment-status').textContent,/引用/);
});

test('publish control updates the shared catalog and stopping a split invalidates activity references',async()=>{
 window.location.hash='#activities/campaign-branch/audience';window.listeners.hashchange();
 element('bundle-name').value='界面发布包';
 element('bundle-audience').value='aud-mx-dormant';
 fire('bundle-form','submit',{preventDefault(){}});
 assert.match(element('bundle-status').textContent,/已发布/);
 assert.match(element('bundle-list').innerHTML,/界面发布包/);
 assert.match(element('audience-catalog').innerHTML,/对照臂/);
 window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();
 await settle();
 assert.doesNotMatch(element('campaign-detail').innerHTML,/引用不可用/);
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-toggle-experiment]'?{dataset:{toggleExperiment:'exp-recall'}}:null}});
 assert.match(element('audience-catalog').innerHTML,/不可用/);
 window.listeners.hashchange();
 await settle();
 assert.match(element('campaign-detail').innerHTML,/引用不可用/);
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-toggle-experiment]'?{dataset:{toggleExperiment:'exp-recall'}}:null}});
 window.listeners.hashchange();
 await settle();
 assert.match(element('campaign-detail').innerHTML,/引用不可用/);
 element('bundle-name').value='重新发布包';element('bundle-audience').value='aud-mx-dormant';
 fire('bundle-form','submit',{preventDefault(){}});
 assert.match(element('bundle-status').textContent,/已发布/);
 assert.match(element('bundle-list').innerHTML,/重新发布包.*可用/);
});

test('activity details directly show treatment content and legacy compose paths return to the activity',async()=>{
 window.location.hash='#experimentation';window.listeners.hashchange();await settle();
 assert.equal(element('experimentation-section').hidden,false);
 assert.equal(element('module-experimentation').attributes['aria-current'],'page');
 assert.equal(element('activity-context').hidden,true);
 assert.equal(element('activity-menu-list').hidden,true);
 window.location.hash='#activities/campaign-branch/compose';window.listeners.hashchange();await settle();
 assert.equal(element('campaign-section').hidden,false);
 assert.match(element('campaign-detail').innerHTML,/treatment-rcs/);
 assert.equal(elements.has('flow-section'),false);
 assert.equal(pending.size,0);
 window.location.hash='#compose';window.listeners.hashchange();await settle();
 assert.equal(element('activity-landing').hidden,false);
 window.location.hash='#activities/missing/campaign';window.listeners.hashchange();await settle();
 assert.equal(element('activity-not-found-title').textContent,'未找到该活动');
 assert.equal(element('audience-section').hidden,true);
});

test('campaign list and detail deep links follow history without a second workspace tab',async()=>{
 window.location.hash='#activities';window.listeners.hashchange();
 await settle();
 assert.equal(element('campaign-activities').hidden,false);
 assert.match(element('campaign-activities').innerHTML,/campaign-branch/);
 window.location.hash='#activities/campaign-branch/campaign';window.listeners.hashchange();
 await settle();
 assert.equal(element('campaign-detail').hidden,false);
 assert.match(element('campaign-detail').innerHTML,/treatment-rcs/);
 window.location.hash='#activities/unknown/campaign';window.listeners.hashchange();
 await settle();
 assert.equal(element('activity-not-found-title').textContent,'未找到该活动');
 window.location.hash='#activities/campaign-branch/campaign';window.listeners.hashchange();
 await settle();
 assert.match(element('campaign-detail').innerHTML,/跨境召回/);
 window.location.hash='#activities';window.listeners.hashchange();
 await settle();
 assert.equal(element('campaign-activities').hidden,false);
 window.location.hash='#activities/campaign-branch/compose';window.listeners.hashchange();await settle();
 assert.equal(element('campaign-detail').hidden,false);
 assert.equal(elements.has('flow-section'),false);
 assert.equal(pending.size,0);
 window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();await settle();
 assert.equal(element('campaign-detail').hidden,false);
 assert.equal(element('activity-context-name').textContent,'跨境召回');
});

test('global module navigation separates shared assets from activity workspaces',async()=>{
 window.location.hash='#activities';window.listeners.hashchange();await settle();
 assert.match(element('activity-list').innerHTML,/campaign-branch/);
 assert.match(element('activity-list').innerHTML,/campaign-empty/);
 const storageKeys=[...browserStorage.keys()],writesBefore=storageWrites.length;
 window.location.hash='#audience';window.listeners.hashchange();await settle();
 assert.equal(element('audience-section').hidden,false);
 assert.equal(element('module-audience').attributes['aria-current'],'page');
 assert.equal(element('activity-context').hidden,true);
 assert.equal(element('activity-menu-list').hidden,true);
 window.location.hash='#experimentation';window.listeners.hashchange();await settle();
 assert.equal(element('experimentation-section').hidden,false);
 assert.equal(element('module-experimentation').attributes['aria-current'],'page');
 window.location.hash='#activities/campaign-empty/campaign';window.listeners.hashchange();await settle();
 assert.equal(element('activity-context-name').textContent,'待装配活动');
 assert.equal(element('campaign-detail').hidden,false);
 assert.match(element('activity-list').innerHTML,/aria-current="location"/);
 assert.equal(element('activity-menu-list').hidden,false);
 window.location.hash='#activities/campaign-branch/audience';window.listeners.hashchange();await settle();
 assert.equal(element('audience-section').hidden,false);
 assert.equal(element('asset-return').hidden,false);
 assert.equal(element('asset-return-link').href,'#activities/campaign-branch/campaign');
 assert.equal(storageWrites.length,writesBefore);
 assert.deepEqual([...browserStorage.keys()],storageKeys);
 assert.ok([...browserStorage.keys()].every(key=>key===STORAGE_KEY));
});

test('activity changes invalidate old operation forms before the next activity list request completes',async()=>{
 window.location.hash='#activities/campaign-branch/campaign';window.listeners.hashchange();await settle();
 assert.equal(element('campaign-detail').hidden,false);
 window.location.hash='#activities/campaign-branch/campaign/edit';window.listeners.hashchange();await settle();
 element('campaign-name').value='迟到的旧表单';
 const staleForm=formTarget('campaign-form');
 const beforeWrites=writes, beforeActivities=apiCampaigns.length;
 deferCampaignList=true;
 window.location.hash='#activities/campaign-empty/campaign';window.listeners.hashchange();
 assert.equal(element('campaign-section').hidden,true);
 assert.equal(element('campaign-detail').hidden,true);
 assert.equal(element('activity-route-loading').hidden,false);
 await fire('campaign-section','submit',{target:staleForm,preventDefault(){}});
 assert.equal(writes,beforeWrites);
 assert.equal(apiCampaigns.length,beforeActivities);
 assert.equal(window.location.hash,'#activities/campaign-empty/campaign');
 releaseCampaignList();await settle();
 assert.equal(element('activity-context-name').textContent,'待装配活动');
 assert.equal(element('campaign-section').hidden,false);
 assert.equal(element('module-activities').attributes['aria-current'],'page');
});

test('a stale activity delete control cannot delete after its controller is cleared',async()=>{
 window.location.hash='#activities/campaign-branch/campaign';window.listeners.hashchange();await settle();
 const staleDelete={dataset:{controllerVersion:String(controllerVersion('campaign-detail')),deleteActivity:'campaign-branch'},hasAttribute:()=>false};
 const before=apiCampaigns.length;
 window.confirm=()=>true;
 window.location.hash='#activities/campaign-empty/campaign';window.listeners.hashchange();await settle();
 await fire('campaign-section','click',{target:{closest:selector=>selector==='button'?staleDelete:null}});
 assert.equal(apiCampaigns.length,before);
 assert.ok(apiCampaigns.some(item=>item.id==='campaign-branch'));
 assert.equal(window.location.hash,'#activities/campaign-empty/campaign');
});

test('encoded activity IDs round trip through list links and deep routes',async()=>{
 const activity={id:'campaign alpha/β',name:'特殊 ID 活动',treatments:[]};
 apiCampaigns.push(activity);
 try {
  window.location.hash='#activities';window.listeners.hashchange();
  await settle();
  assert.match(element('campaign-activities').innerHTML,/href="#activities\/campaign%20alpha%2F%CE%B2\/campaign"/);
  window.location.hash='#activities/campaign%20alpha%2F%CE%B2/campaign';window.listeners.hashchange();
  await settle();
  assert.match(element('campaign-detail').innerHTML,/特殊 ID 活动/);
  window.location.hash='#activities/%ZZ/audience';window.listeners.hashchange();
  await settle();
  assert.equal(element('activity-not-found-title').textContent,'未找到该活动');
 } finally {apiCampaigns.pop();}
});

test('audience entry mode, batch schedule and event trigger are saved with the shared definition',()=>{
 fire('new-audience','click');
 element('audience-name').value='定时活动受众';
 element('batch-schedule').value='daily';element('batch-time').value='18:30';fire('audience-form','change');
 assert.equal(element('batch-time-fields').hidden,false);
 fire('audience-form','submit',{preventDefault(){}});
 const savedId=[...element('audience-list').innerHTML.matchAll(/data-edit-audience=\"([^\"]+)/g)].at(-1)?.[1];
 assert.ok(savedId);
 fire('audience-list','click',{target:{closest:selector=>selector==='[data-edit-audience]'?{dataset:{editAudience:savedId}}:null}});
 assert.equal(element('entry-mode').value,'batch');assert.equal(element('batch-time').value,'18:30');
 element('batch-time').value='';fire('audience-form','input');assert.equal(element('save-audience').disabled,true);
 element('entry-mode').value='event';element('event-type').value='order_paid';fire('audience-form','change');
 assert.equal(element('batch-entry-fields').hidden,true);assert.equal(element('event-entry-fields').hidden,false);
 assert.equal(element('save-audience').disabled,false);
 assert.ok(element('preview-message').textContent.includes('不是已收到'));
 fire('audience-form','submit',{preventDefault(){}});
 assert.match(element('audience-list').innerHTML,/定时活动受众/);
});

test('deleting an unreferenced audience succeeds and an API-referenced audience stays protected',async()=>{
 const before=Number(element('audience-count').textContent);
 fire('audience-list','click',{target:{closest:selector=>selector==='[data-delete-audience]'?{dataset:{deleteAudience:'aud-app'}}:null}});
 await settle();
 assert.equal(Number(element('audience-count').textContent),before-1);
 assert.doesNotMatch(element('audience-list').innerHTML,/data-edit-audience=\"aud-app\"/);
 assert.match(element('audience-status').textContent,/已删除/);
 fire('audience-list','click',{target:{closest:selector=>selector==='[data-delete-audience]'?{dataset:{deleteAudience:'aud-mx-dormant'}}:null}});
 await settle();
 assert.match(element('audience-status').textContent,/不能删除/);
});

test('campaign writes follow API success, while 4xx and cancel keep server state',async()=>{
 window.location.hash='#activities/new';window.listeners.hashchange();await settle();
 element('campaign-name').value='接口创建活动';
 const form=formTarget('campaign-form');
 const before=writes;
 nextFailure=true;
 await fire('campaign-section','submit',{target:form,preventDefault(){}});
 assert.match(element('campaign-error').textContent,/服务端拒绝保存/);
 assert.equal(writes,before);
 assert.equal(window.location.hash,'#activities/new');
 await fire('campaign-section','submit',{target:form,preventDefault(){}});await settle();
 const id=window.location.hash.slice('#activities/'.length).split('/')[0];
 assert.match(id,/^campaign-test-/);
 assert.match(element('campaign-detail').innerHTML,/接口创建活动/);
 window.location.hash+= '/edit';window.listeners.hashchange();await settle();
 const editForm=formTarget('campaign-form');
 nextFailure=true;
 await fire('campaign-section','submit',{target:editForm,preventDefault(){}});
 assert.doesNotMatch(element('campaign-status').textContent,/已保存/);
 element('campaign-name').value='未保存改名';
 const savedWrites=writes;
 fire('campaign-section','click',{target:{closest:selector=>selector==='button'?{dataset:{controllerVersion:String(controllerVersion('campaign-editor'))},hasAttribute:name=>name==='data-cancel-campaign'}:null}});
 assert.equal(writes,savedWrites);
 assert.equal(apiCampaigns.find(item=>item.id===id).name,'接口创建活动');
 window.location.hash=`#campaign/activities/${id}/edit`;window.listeners.hashchange();await settle();
 element('campaign-name').value='接口已改名';
 await fire('campaign-section','submit',{target:formTarget('campaign-form'),preventDefault(){}});await settle();
 assert.equal(apiCampaigns.find(item=>item.id===id).name,'接口已改名');
 window.confirm=()=>true;
 await fire('campaign-section','click',{target:{closest:selector=>selector==='button'?{dataset:{controllerVersion:String(controllerVersion('campaign-detail')),deleteActivity:id},hasAttribute:()=>false}:null}});await settle();
 assert.ok(!apiCampaigns.some(item=>item.id===id));
 window.location.hash=`#campaign/activities/${id}`;window.listeners.hashchange();await settle();
 assert.equal(element('activity-not-found-title').textContent,'未找到该活动');
});

test('treatment add, edit and delete use API state and reject failed writes',async()=>{
 window.location.hash='#campaign/activities/campaign-empty/treatments/new';window.listeners.hashchange();await settle();
 assert.match(element('campaign-editor').innerHTML,/新增投放项/);
 assert.match(element('campaign-editor').innerHTML,/引用人群组或实验包臂/);
 assert.match(element('campaign-editor').innerHTML,/新标签打开受众库/);
 assert.match(element('campaign-editor').innerHTML,/target="_blank" rel="noopener"/);
 assert.match(element('campaign-editor').innerHTML,/管理 API 当前开放的引用/);
 element('treatment-name').value='短信';element('treatment-action').value='action-sms';element('treatment-audience').value='group:group-new';element('treatment-slot').value='';
 const form=formTarget('treatment-form');
 nextFailure=true;
 await fire('campaign-section','submit',{target:form,preventDefault(){}});
 assert.match(element('campaign-error').textContent,/服务端拒绝保存/);
 assert.equal(apiCampaigns.find(item=>item.id==='campaign-empty').treatments.length,0);
 await fire('campaign-section','submit',{target:form,preventDefault(){}});await settle();
 const added=apiCampaigns.find(item=>item.id==='campaign-empty').treatments[0];
 assert.equal(added.actionId,'action-sms');
 assert.match(element('campaign-detail').innerHTML,/短信/);
 window.location.hash=`#campaign/activities/campaign-empty/treatments/${added.id}/edit`;window.listeners.hashchange();await settle();
 element('treatment-name').value='短信已改名';element('treatment-action').value='action-sms';element('treatment-audience').value='group:group-new';element('treatment-slot').value='';
 await fire('campaign-section','submit',{target:formTarget('treatment-form'),preventDefault(){}});await settle();
 assert.equal(apiCampaigns.find(item=>item.id==='campaign-empty').treatments[0].name,'短信已改名');
 window.confirm=()=>true;
 await fire('campaign-section','click',{target:{closest:selector=>selector==='button'?{dataset:{controllerVersion:String(controllerVersion('campaign-detail')),deleteTreatment:added.id},hasAttribute:()=>false}:null}});await settle();
 assert.equal(apiCampaigns.find(item=>item.id==='campaign-empty').treatments.length,0);
 assert.match(element('campaign-detail').innerHTML,/暂无投放项/);
});

test('older usage refresh cannot replace newer audience protection',async()=>{
 let calls=0, release;
 const api={
  list:()=>{calls++;return calls===2 ? new Promise(resolve=>{release=resolve;}) : Promise.resolve({campaigns:[]});},
  usages:async()=>({usages:[]}),
 };
 const ui=mountAudiences({api,onSaved(){},onDeleted(){},onNavigate(){}});
 await settle();
 const older=ui.refreshProtection();
 const newer=ui.refreshProtection();
 await newer;
 release({campaigns:[{treatments:[{audienceRef:{type:'group',groupId:'group-new'}}]}]});
 await older;
 assert.doesNotMatch(element('audience-list').innerHTML,/活动占用/);
});

test('treatment completion refreshes usages after navigating away during HTTP write',async()=>{
 window.location.hash='#campaign/activities/campaign-empty/treatments/new';window.listeners.hashchange();await settle();
 element('treatment-name').value='慢请求短信';element('treatment-action').value='action-sms';element('treatment-audience').value='group:group-new';element('treatment-slot').value='';
 const before=usageCalls;
 deferTreatment=true;
 const saving=fire('campaign-section','submit',{target:formTarget('treatment-form'),preventDefault(){}});
 await settle();
 window.location.hash='#campaign/activities';window.listeners.hashchange();await settle();
 releaseTreatment();
 await saving;await settle();
 assert.ok(apiCampaigns.find(item=>item.id==='campaign-empty').treatments.some(item=>item.name==='慢请求短信'));
 assert.ok(usageCalls>before);
 assert.equal(window.location.hash,'#campaign/activities');
});

test('treatment completion keeps another domain open after HTTP write',async()=>{
 window.location.hash='#campaign/activities/campaign-empty/treatments/new';window.listeners.hashchange();await settle();
 element('treatment-name').value='跨域切换短信';element('treatment-action').value='action-sms';element('treatment-audience').value='group:group-new';element('treatment-slot').value='';
 const before=usageCalls;
 deferTreatment=true;
 const saving=fire('campaign-section','submit',{target:formTarget('treatment-form'),preventDefault(){}});
 await settle();
 window.location.hash='#activities/campaign-branch/audience';window.listeners.hashchange();await settle();
 releaseTreatment();
 await saving;await settle();
 assert.ok(apiCampaigns.find(item=>item.id==='campaign-empty').treatments.some(item=>item.name==='跨域切换短信'));
 assert.ok(usageCalls>before);
 assert.equal(window.location.hash,'#activities/campaign-branch/audience');
});
