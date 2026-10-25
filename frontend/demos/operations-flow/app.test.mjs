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
 assert.equal(element('queued').textContent,240);assert.equal(element('active').textContent,0);
 for (const scenario of ['baseline','pressure','recovery']) {
  fire('scenario','change',{target:{value:scenario}});
  assert.equal(element('queued').textContent,240);
  fire('timeline','input',{target:{value:element('timeline').max}});
  assert.equal(element('active').textContent,0);
  assert.equal(Number(element('success').textContent)+Number(element('failed').textContent),240);
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
 assert.equal(element('queued').textContent,240);
});
