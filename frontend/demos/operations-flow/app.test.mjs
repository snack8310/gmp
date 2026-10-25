// Browserless handler checks complement, but do not replace, native browser QA.
import test from 'node:test';
import assert from 'node:assert/strict';
const elements = new Map(), pending = new Map();
let frameID = 0, captures = 0;
function element(id) {
 if (!elements.has(id)) elements.set(id, {
  innerHTML: '', textContent: '', value: 0, style: {}, dataset: {}, listeners: {},
  classList: { toggle() {}, add() {}, remove() {} }, setAttribute() {},
  addEventListener(name, fn) { this.listeners[name] = fn; },
  setPointerCapture() { captures++; },
 });
 return elements.get(id);
}
const media = { matches: false, addEventListener(name,fn) { this.change=fn; } };
globalThis.document = { getElementById:element, querySelector:element, querySelectorAll:()=>[], addEventListener() {} };
globalThis.window = { addEventListener() {} };
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

test('audience experiment is inherited read-only and extra canvas experiment cannot change it',()=>{
 fire('flow-audience','change',{target:{value:'aud-app'}});
 fire('edit-flow-audience','click');
 element('audience-experiment-enabled').checked=true;
 element('audience-experiment-picker-select').value='exp-recall';fire('audience-form','change');
 assert.ok(element('preview-assignment').textContent.includes('对照'));
 fire('audience-form','submit',{preventDefault(){}});
 assert.equal(element('total').textContent,192);
 assert.ok(element('inherited-experiment-note').textContent.includes('不可在画布取消'));
 assert.ok(!element('node-list').innerHTML.includes('策略闸机'));
 assert.ok(element('node-list').innerHTML.includes('对照组'));
 const inherited=element('inherited-experiment-note').textContent;
 element('canvas-experiment-enabled').checked=true;
 element('canvas-experiment-picker-select').value='exp-onboarding';
 fire('canvas-experiment-enabled','change');
 assert.equal(element('inherited-experiment-note').textContent,inherited);
 assert.ok(element('split-summary').textContent.includes('A 分支'));
 fire('timeline','input',{target:{value:element('timeline').max}});
 assert.equal(Number(element('success').textContent)+Number(element('failed').textContent)+Number(element('control').textContent),192);
 assert.equal(element('active').textContent,0);
 fire('play','click');assert.equal(pending.size,1);
 element('canvas-experiment-enabled').checked=false;fire('canvas-experiment-enabled','change');assert.equal(pending.size,0);
 assert.equal(element('inherited-experiment-note').textContent,inherited);
 assert.ok(element('node-list').innerHTML.includes('对照组'));
 assert.ok(!element('node-list').innerHTML.includes('副对照'));
 element('canvas-experiment-enabled').checked=true;element('canvas-experiment-picker-select').value='exp-recall';fire('canvas-experiment-enabled','change');
 assert.ok(element('split-summary').textContent.includes('同一实验，复用原分配'));
 fire('flow-audience','change',{target:{value:'aud-whitelist-new'}});
 assert.equal(element('canvas-experiment-enabled').checked,false);
 assert.ok(element('inherited-experiment-name').textContent.includes('未引用'));
 element('canvas-experiment-enabled').checked=true;fire('canvas-experiment-enabled','change');
 assert.ok(element('split-summary').textContent.includes('整个人群'));
});

test('shared experiment picker creates, renames and cancels independently while keeping identity',()=>{
 fire('edit-flow-audience','click');element('audience-experiment-enabled').checked=true;
 fire('audience-experiment-picker-new','click');element('audience-experiment-picker-name').value='Reusable demo';
 fire('audience-experiment-picker-save','click');
 const experimentId=element('audience-experiment-picker-select').value;
 assert.ok(experimentId.startsWith('exp-custom-'));
 fire('audience-form','submit',{preventDefault(){}});
 const before=element('inherited-experiment-note').textContent;
 fire('edit-flow-audience','click');fire('audience-experiment-picker-edit','click');
 element('audience-experiment-picker-name').value='Canceled rename';fire('audience-experiment-picker-cancel','click');
 assert.ok(!element('audience-experiment-picker-select').innerHTML.includes('Canceled rename'));
 fire('audience-experiment-picker-edit','click');element('audience-experiment-picker-name').value='Renamed shared';fire('audience-experiment-picker-save','click');
 assert.equal(element('audience-experiment-picker-select').value,experimentId);
 assert.ok(element('inherited-experiment-name').textContent.includes('Renamed shared'));
 assert.equal(element('inherited-experiment-note').textContent,before);
 fire('cancel-audience','click');
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
