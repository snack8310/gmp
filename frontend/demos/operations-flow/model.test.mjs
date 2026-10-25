import test from 'node:test';
import assert from 'node:assert/strict';
import { TOTAL, CHANNELS, SCENARIOS, createScenario, snapshot, createClock, personState } from './model.mjs';

test('fixtures are deterministic, complete, mutually exclusive, and fixed-size', () => {
 for (const key of Object.keys(SCENARIOS)) {
  const s = createScenario(key);
  assert.deepEqual(s, createScenario(key));
  assert.equal(s.people.length, TOTAL);
  assert.equal(new Set(s.people.map(p=>p.id)).size, TOTAL);
  for (const c of CHANNELS) assert.equal(s.people.filter(p=>p.channel===c).length, TOTAL/4);
  assert.equal(s.people.filter(p=>p.group==='a').length, TOTAL/2);
  assert.equal(s.people.filter(p=>p.group==='b').length, TOTAL/2);
 }
 assert.throws(()=>createScenario('unknown'), RangeError);
});

test('counts conserve the population at every transition and over each full timeline', () => {
 for (const key of Object.keys(SCENARIOS)) {
  const s = createScenario(key);
  const times = new Set([0, -10, s.duration, s.duration+100]);
  for (let t=0;t<s.duration;t+=.137) times.add(t);
  for(const p of s.people) for(const age of [0,3,7,12,12+p.hold,17+p.hold]) for(const epsilon of [-.00001,0,.00001]) times.add(p.start+age+epsilon);
  for(const t of times) {
   const v = snapshot(s,t);
   assert.equal(v.queued+v.active+v.success+v.failed,TOTAL, `${key} at ${t}`);
   for(const value of [v.queued,v.active,v.success,v.failed]) assert.ok(Number.isInteger(value)&&value>=0);
   assert.equal(Object.values(v.nodes).reduce((n,x)=>n+x.active,0), v.active+v.queued);
   assert.equal(v.nodes.success.arrived,v.success);assert.equal(v.nodes.failed.arrived,v.failed);
   assert.equal(CHANNELS.reduce((n,c)=>n+v.nodes[c].success+v.nodes[c].failed,0),v.success+v.failed);
  }
  const initial=snapshot(s,0), end=snapshot(s,s.duration);
  assert.equal(initial.queued,TOTAL);assert.equal(initial.active,0);
  assert.equal(end.queued,0);assert.equal(end.active,0);assert.equal(end.success+end.failed,TOTAL);
  assert.ok(end.success>0);assert.ok(end.failed>0);
  assert.equal(end.nodes.strategy.arrived,TOTAL);
  assert.equal(end.nodes.a.arrived+end.nodes.b.arrived,TOTAL);
  assert.equal(CHANNELS.reduce((n,c)=>n+end.nodes[c].arrived,0),TOTAL);
 }
});

test('delayed receipts stay in progress and do not create extra failures', () => {
 const base=createScenario('baseline'), delay=createScenario('pressure'), recovery=createScenario('recovery');
 assert.ok(delay.duration>base.duration);
 assert.ok(snapshot(delay,30).nodes.whatsapp.active>snapshot(base,30).nodes.whatsapp.active);
 assert.equal(snapshot(delay,delay.duration).failed,snapshot(base,base.duration).failed);
 assert.ok(snapshot(recovery,recovery.duration).failed<snapshot(base,base.duration).failed);
 const p=delay.people.find(p=>p.channel==='whatsapp');
 assert.equal(personState(p,p.start+20).phase,'waiting');
});

function harness(duration=2) {
 let next=0;const pending=new Map(),renders=[];
 const clock=createClock({duration,render:(t,p)=>renders.push([t,p]),request:fn=>{pending.set(++next,fn);return next;},cancel:id=>pending.delete(id)});
 return {clock,pending,renders,step(stamp){assert.equal(pending.size,1);const [id,fn]=pending.entries().next().value;pending.delete(id);fn(stamp);}};
}

test('play is idempotent, pause freezes, reset cancels, and repeated scene changes own one frame', () => {
 const h=harness();
 for(let i=0;i<30;i++)h.clock.play();assert.equal(h.pending.size,1);
 h.step(0);h.step(200);assert.equal(h.clock.time,.2);
 h.clock.pause();const frozen=h.clock.time;assert.equal(h.pending.size,0);assert.equal(h.clock.time,frozen);
 h.clock.play();h.step(50000);assert.equal(h.clock.time,frozen);h.step(50200);assert.equal(h.clock.time,.4);
 for(let i=0;i<30;i++){h.clock.reset(3);assert.equal(h.clock.time,0);assert.equal(h.pending.size,0);h.clock.play();assert.equal(h.pending.size,1);}
 h.clock.pause();assert.equal(h.pending.size,0);
});

test('clock bounds seeking and speed, stops at completion, and can replay cleanly', () => {
 const h=harness();h.clock.seek(-3);assert.equal(h.clock.time,0);h.clock.setSpeed(4);h.clock.play();h.step(0);h.step(250);assert.equal(h.clock.time,1);h.step(500);assert.equal(h.clock.time,2);assert.equal(h.clock.playing,false);assert.equal(h.pending.size,0);
 h.clock.play();assert.equal(h.pending.size,0);h.clock.reset();h.clock.play();assert.equal(h.pending.size,1);h.clock.seek(999);assert.equal(h.clock.time,2);assert.equal(h.pending.size,0);assert.throws(()=>h.clock.setSpeed(99),RangeError);
});
