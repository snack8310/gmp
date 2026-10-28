// Browserless handler checks complement, but do not replace, native browser QA.
import test from 'node:test';
import assert from 'node:assert/strict';
import { BROWSE_ACTIVITIES } from './campaign-activities.mjs';
const elements = new Map(), pending = new Map();
let frameID = 0, captures = 0;
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
globalThis.window = { location:{hash:''}, addEventListener(name,fn) {this.listeners[name]=fn;}, listeners:{} };
globalThis.matchMedia = () => media;
globalThis.requestAnimationFrame = fn => { pending.set(++frameID,fn); return frameID; };
globalThis.cancelAnimationFrame = id => pending.delete(id);
globalThis.fetch = () => { throw new Error('Presentation must not call a network API'); };
await import('./app.mjs');
const fire = (id,event,data={}) => element(id).listeners[event](data);
const pointer = (id, extra={}) => ({pointerType:'mouse',button:0,clientX:100,clientY:100,pointerId:1,target:{closest:()=>id?{dataset:{node:id}}:null},...extra});

test('paused initial frame and exact-end seeking keep displayed counters consistent', () => {
 assert.equal(element('queued').textContent,192);assert.equal(element('active').textContent,0);
 for (const scenario of ['baseline','pressure','recovery']) {
  fire('scenario','change',{target:{value:scenario}});
  assert.equal(element('queued').textContent,192);
  fire('timeline','input',{target:{value:element('timeline').max}});
  assert.equal(element('active').textContent,0);
  assert.equal(Number(element('success').textContent)+Number(element('failed').textContent),192);
 }
 fire('reset','click');assert.equal(element('time').textContent,'00:00.0');
});

test('node identity survives replacement of SVG during a press without capturing a normal click', () => {
 captures=0;
 fire('world','pointerdown',pointer('whatsapp'));
 fire('timeline','input',{target:{value:30}});
 fire('world','pointerup');
 assert.equal(captures,0);assert.equal(element('detail-title').textContent,'WhatsApp');
});

test('a camera drag captures only after movement and does not select a node', () => {
 fire('world','pointerdown',pointer('sms'));
 fire('world','pointermove',pointer('sms',{clientX:140}));
 assert.ok(captures>0);
 fire('world','pointerup');
 assert.equal(element('detail-title').textContent,'WhatsApp');
});

test('touch tap selects, but vertical scrolling does not select or capture', () => {
 const previousCaptures=captures;
 fire('world','pointerdown',pointer('push',{pointerType:'touch'}));fire('world','pointerup');
 assert.equal(element('detail-title').textContent,'Push');
 fire('world','pointerdown',pointer('ivr',{pointerType:'touch'}));
 fire('world','pointermove',pointer('ivr',{pointerType:'touch',clientY:150}));fire('world','pointerup');
 assert.equal(element('detail-title').textContent,'Push');assert.equal(captures,previousCaptures);
});

test('reduced motion cancels playback and removes moving people while keeping solids and metrics', () => {
 fire('reset','click');fire('play','click');assert.equal(pending.size,1);
 media.matches=true;media.change();assert.equal(pending.size,0);
 assert.equal(element('objects').innerHTML.includes('cy="-11"'),false);
 assert.ok(element('objects').innerHTML.includes('polygon'));
 fire('play','click');assert.equal(pending.size,1);
 fire('reset','click');assert.equal(pending.size,0);
 assert.equal(element('queued').textContent,192);
});

test('audience create, cancel, save and reuse reset only when the referenced definition changes', () => {
 fire('new-audience','click');
 assert.equal(element('audience-editor').hidden,false);
 element('audience-name').value='Temporary draft';element('rule-country').value='MX';
 fire('audience-form','input');assert.ok(Number(element('preview-count').textContent)<192);
 fire('cancel-audience','click');assert.equal(element('total').textContent,192);
 fire('new-audience','click');assert.equal(element('audience-name').value,'');
 element('audience-name').value='Saved test';element('rule-country').value='MX';fire('audience-form','input');
 const count=Number(element('preview-count').textContent);
 fire('audience-form','submit',{preventDefault(){}});
 assert.equal(element('audience-editor').hidden,true);
 fire('flow-audience','change',{target:{value:'aud-custom-1'}});
 assert.equal(element('total').textContent,count);
 fire('play','click');assert.equal(pending.size,1);
 fire('edit-flow-audience','click');assert.equal(pending.size,0);
 element('rule-min').value=300; element('rule-max').value=365;fire('audience-form','input');
 assert.equal(element('preview-count').textContent,0);
 fire('audience-form','submit',{preventDefault(){}});
 assert.equal(element('total').textContent,0);assert.equal(element('play').disabled,true);
 fire('play','click');assert.equal(pending.size,0);
 fire('edit-flow-audience','click');element('rule-min').value='';fire('audience-form','input');
 assert.equal(element('save-audience').disabled,true);assert.equal(element('preview-count').textContent,'—');
 fire('cancel-audience','click');assert.equal(element('total').textContent,0);
});

test('flow experiment is a selectable path node, separate from read-only inherited audience',()=>{
 fire('flow-audience','change',{target:{value:'aud-app'}});fire('edit-flow-audience','click');
 element('audience-experiment-enabled').checked=true;element('audience-experiment-picker-select').value='exp-recall';fire('audience-form','change');
 fire('audience-form','submit',{preventDefault(){}});
 const inherited=element('inherited-experiment-note').textContent,entry=element('flow-entry-summary').textContent;
 assert.ok(inherited.includes('不可在画布取消'));
 assert.ok(element('labels').innerHTML.includes('data-node="add-a"'));
 assert.ok(!element('labels').innerHTML.includes('data-node="add-control"'));
 fire('world','pointerdown',pointer('add-control'));fire('world','pointerup');
 assert.ok(!element('node-list').innerHTML.includes('data-node="flowExperiment"'));
 fire('world','pointerdown',pointer('add-a'));fire('world','pointerup');
 assert.ok(element('node-list').innerHTML.includes('data-node="flowExperiment"'));
 assert.equal(element('flow-node-editor').hidden,false);assert.equal(element('detail-title').textContent,'流程实验');
 assert.ok(element('flow-node-scope').textContent.includes('A 分支'));
 assert.ok(element('split-summary').textContent.includes('同一实验，复用原分配'));
 assert.ok(!element('labels').innerHTML.includes('data-node="add-a"'));
 element('flow-node-picker-select').value='exp-onboarding';fire('flow-node-picker-select','change');
 assert.equal(element('inherited-experiment-note').textContent,inherited);assert.equal(element('flow-entry-summary').textContent,entry);
 fire('timeline','input',{target:{value:element('timeline').max}});
 assert.equal(Number(element('success').textContent)+Number(element('failed').textContent)+Number(element('control').textContent),192);
 assert.equal(element('active').textContent,0);assert.ok(Number(element('detail-arrived').textContent)>0);
 fire('world','pointerdown',pointer('audience'));fire('world','pointerup');assert.equal(element('flow-node-editor').hidden,true);
 fire('world','pointerdown',pointer('flowExperiment'));fire('world','pointerup');assert.equal(element('flow-node-editor').hidden,false);
 fire('play','click');assert.equal(pending.size,1);fire('remove-flow-node','click');assert.equal(pending.size,0);
 assert.equal(element('inherited-experiment-note').textContent,inherited);assert.equal(element('flow-entry-summary').textContent,entry);
 assert.ok(!element('node-list').innerHTML.includes('data-node="flowExperiment"'));assert.equal(element('flow-node-editor').hidden,true);
 fire('remove-flow-node','click');assert.equal(element('inherited-experiment-note').textContent,inherited);
 fire('flow-audience','change',{target:{value:'aud-whitelist-new'}});
 assert.ok(element('inherited-experiment-name').textContent.includes('未引用'));
 fire('flow-path-actions','click',{target:{closest:selector=>selector==='[data-add-branch]'?{dataset:{addBranch:'direct'}}:null}});
 assert.ok(element('flow-node-scope').textContent.includes('普通出口'));assert.equal(element('flow-node-editor').hidden,false);
 fire('flow-audience','change',{target:{value:'aud-app'}});
 assert.equal(element('flow-node-editor').hidden,true);assert.ok(!element('node-list').innerHTML.includes('data-node="flowExperiment"'));
});

test('experiment page creates and renames identities reused by audience and composition',()=>{
 window.location.hash='#experimentation';window.listeners.hashchange();
 assert.equal(element('experimentation-section').hidden,false);
 assert.equal(element('nav-experimentation').attributes['aria-current'],'page');
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
 fire('edit-flow-audience','click');element('audience-experiment-enabled').checked=true;
 element('audience-experiment-picker-select').value=experimentId;fire('audience-form','change');
 fire('audience-form','submit',{preventDefault(){}});
 assert.ok(element('inherited-experiment-name').textContent.includes('Renamed shared'));
 assert.equal(element('audience-section').hidden,false);
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-delete-experiment]'?{dataset:{deleteExperiment:experimentId}}:null}});
 assert.match(element('experiment-status').textContent,/引用/);
});

test('publish control updates the shared catalog and stopping a split invalidates activity references',()=>{
 window.location.hash='#audience';window.listeners.hashchange();
 element('bundle-name').value='界面发布包';
 element('bundle-audience').value='aud-mx-dormant';
 fire('bundle-form','submit',{preventDefault(){}});
 assert.match(element('bundle-status').textContent,/已发布/);
 assert.match(element('bundle-list').innerHTML,/界面发布包/);
 assert.match(element('audience-catalog').innerHTML,/对照臂/);
 window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();
 assert.doesNotMatch(element('campaign-detail').innerHTML,/引用不可用/);
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-toggle-experiment]'?{dataset:{toggleExperiment:'exp-recall'}}:null}});
 assert.match(element('audience-catalog').innerHTML,/不可用/);
 window.listeners.hashchange();
 assert.match(element('campaign-detail').innerHTML,/引用不可用/);
 fire('experiment-list','click',{target:{closest:selector=>selector==='[data-toggle-experiment]'?{dataset:{toggleExperiment:'exp-recall'}}:null}});
 window.listeners.hashchange();
 assert.match(element('campaign-detail').innerHTML,/引用不可用/);
 element('bundle-name').value='重新发布包';element('bundle-audience').value='aud-mx-dormant';
 fire('bundle-form','submit',{preventDefault(){}});
 assert.match(element('bundle-status').textContent,/已发布/);
 assert.match(element('bundle-list').innerHTML,/重新发布包.*可用/);
});

test('domain deep links expose only their section and pause composition on exit',()=>{
 window.location.hash='#compose';window.listeners.hashchange();
 assert.equal(element('flow-section').hidden,false);
 fire('play','click');assert.equal(pending.size,1);
 window.location.hash='#campaign';window.listeners.hashchange();
 assert.equal(element('campaign-section').hidden,false);
 assert.equal(element('flow-section').hidden,true);
 assert.equal(element('nav-campaign').attributes['aria-current'],'page');
 assert.equal(pending.size,0);
 window.location.hash='#missing';window.listeners.hashchange();
 assert.equal(element('audience-section').hidden,false);
});

test('campaign list and detail deep links follow history and pause composition',()=>{
 window.location.hash='#campaign';window.listeners.hashchange();
 assert.equal(element('campaign-overview').hidden,false);
 assert.equal(element('nav-campaign').attributes['aria-current'],'page');
 window.location.hash='#campaign/activities';window.listeners.hashchange();
 assert.equal(element('campaign-activities').hidden,false);
 assert.match(element('campaign-activities').innerHTML,/campaign-branch/);
 window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();
 assert.equal(element('campaign-detail').hidden,false);
 assert.match(element('campaign-detail').innerHTML,/treatment-rcs/);
 window.location.hash='#campaign/activities/unknown';window.listeners.hashchange();
 assert.match(element('campaign-detail').innerHTML,/未找到该演示活动/);
 window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();
 assert.match(element('campaign-detail').innerHTML,/跨境召回/);
 window.location.hash='#campaign/activities';window.listeners.hashchange();
 assert.equal(element('campaign-activities').hidden,false);
 window.location.hash='#compose';window.listeners.hashchange();
 fire('play','click');assert.equal(pending.size,1);
 window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();
 assert.equal(pending.size,0);
 assert.equal(element('nav-campaign').attributes['aria-current'],'page');
 assert.equal(element('nav-compose').attributes['aria-current'],'false');
 window.location.hash='#campaign/other';window.listeners.hashchange();
 assert.equal(element('nav-campaign').attributes['aria-current'],'page');
 assert.equal(element('campaign-overview').hidden,false);
});

test('encoded activity IDs round trip through list links and deep routes',()=>{
 const activity={id:'campaign alpha/β',name:'特殊 ID 活动',treatments:[]};
 BROWSE_ACTIVITIES.push(activity);
 try {
  window.location.hash='#campaign/activities';window.listeners.hashchange();
  assert.match(element('campaign-activities').innerHTML,/href="#campaign\/activities\/campaign%20alpha%2F%CE%B2"/);
  window.location.hash='#campaign/activities/campaign%20alpha%2F%CE%B2';window.listeners.hashchange();
  assert.match(element('campaign-detail').innerHTML,/特殊 ID 活动/);
  window.location.hash='#campaign/activities/%ZZ';window.listeners.hashchange();
  assert.match(element('campaign-detail').innerHTML,/未找到该演示活动/);
 } finally {BROWSE_ACTIVITIES.pop();}
});

test('entry mode, batch schedule and event trigger are authored in audience management only',()=>{
 fire('flow-audience','change',{target:{value:'aud-app'}});
 fire('edit-flow-audience','click');
 assert.equal(element('entry-mode').value,'batch');
 element('batch-schedule').value='daily';element('batch-time').value='18:30';fire('audience-form','change');
 assert.equal(element('batch-time-fields').hidden,false);
 fire('audience-form','submit',{preventDefault(){}});
 assert.ok(element('flow-entry-summary').textContent.includes('18:30'));
 assert.ok(element('flow-entry-boundary').textContent.includes('不会执行'));
 fire('edit-flow-audience','click');element('batch-time').value='';fire('audience-form','input');
 assert.equal(element('save-audience').disabled,true);
 element('entry-mode').value='event';element('event-type').value='order_paid';fire('audience-form','change');
 assert.equal(element('batch-entry-fields').hidden,true);assert.equal(element('event-entry-fields').hidden,false);
 assert.equal(element('save-audience').disabled,false);
 assert.ok(element('preview-message').textContent.includes('不是已收到'));
 fire('audience-form','submit',{preventDefault(){}});
 assert.ok(element('flow-entry-title').textContent.includes('事件人群'));
 assert.ok(element('flow-entry-summary').textContent.includes('order_paid'));
 assert.ok(!element('flow-entry-summary').textContent.includes('18:30'));
 assert.ok(element('flow-entry-boundary').textContent.includes('多画布'));
 fire('edit-flow-audience','click');assert.equal(element('entry-mode').value,'event');assert.equal(element('event-type').value,'order_paid');
 element('entry-mode').value='batch';element('batch-schedule').value='manual';fire('audience-form','change');
 fire('cancel-audience','click');assert.ok(element('flow-entry-title').textContent.includes('事件人群'));
 fire('edit-flow-audience','click');assert.equal(element('entry-mode').value,'event');
 element('entry-mode').value='batch';element('batch-schedule').value='manual';element('event-type').value='invalid-hidden';fire('audience-form','change');
 assert.equal(element('save-audience').disabled,false);fire('audience-form','submit',{preventDefault(){}});
 assert.ok(element('flow-entry-title').textContent.includes('批量人群'));
 assert.ok(!element('flow-entry-summary').textContent.includes('order_paid'));
});

test('deleting a free audience updates composition while referenced groups stay protected',()=>{
 fire('flow-audience','change',{target:{value:'aud-app'}});
 const before=Number(element('audience-count').textContent);
 fire('audience-list','click',{target:{closest:selector=>selector==='[data-delete-audience]'?{dataset:{deleteAudience:'aud-app'}}:null}});
 assert.equal(Number(element('audience-count').textContent),before-1);
 assert.notEqual(element('flow-audience').value,'aud-app');
 assert.equal(element('total').textContent,Number(element('queued').textContent));
 assert.match(element('audience-status').textContent,/已删除/);
 fire('audience-list','click',{target:{closest:selector=>selector==='[data-delete-audience]'?{dataset:{deleteAudience:'aud-mx-dormant'}}:null}});
 assert.match(element('audience-status').textContent,/不能删除/);
 const remaining=[...element('audience-list').innerHTML.matchAll(/data-delete-audience="([^"]+)"/g)].map(match=>match[1]);
 for(const id of remaining)fire('audience-list','click',{target:{closest:selector=>selector==='[data-delete-audience]'?{dataset:{deleteAudience:id}}:null}});
 assert.equal(element('audience-count').textContent,2);
 assert.ok(Number(element('total').textContent)>0);
});
