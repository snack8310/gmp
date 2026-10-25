import test from 'node:test';
import assert from 'node:assert/strict';
import { MOCK_USERS, assignGroup } from './audience.mjs';
import { TOTAL, CHANNELS, SCENARIOS, createScenario, snapshot, createClock, personState } from './model.mjs';

test('fixtures are deterministic, complete, mutually exclusive, and fixed-size', () => {
 for (const key of Object.keys(SCENARIOS)) {
  const s = createScenario(key);
  assert.deepEqual(s, createScenario(key));
  assert.equal(s.people.length, TOTAL);
  assert.equal(new Set(s.people.map(p=>p.id)).size, TOTAL);
  for (const c of CHANNELS) assert.equal(s.people.filter(p=>p.channel===c).length, TOTAL/4);
  assert.equal(s.people.filter(p=>p.group==='direct').length, TOTAL);
 }
 assert.throws(()=>createScenario('unknown'), RangeError);
});

test('counts conserve the population at every transition and over each full timeline', () => {
 for (const key of Object.keys(SCENARIOS)) {
  const s = createScenario(key);
  const times = new Set([0, -10, s.duration, s.duration+100]);
  for (let t=0;t<s.duration;t+=.137) times.add(t);
  for(const p of s.people) for(const age of [0,4,9,9+p.hold,14+p.hold]) for(const epsilon of [-.00001,0,.00001]) times.add(p.start+age+epsilon);
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
  assert.equal(Object.hasOwn(end.nodes,'strategy'),false);
  assert.equal(end.nodes.direct.arrived,TOTAL);
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

test('filtered experimental populations conserve control separately, including empty and single-user cases', () => {
 for (const users of [[],[{uid:'SIM-0001'}],Array.from({length:80},(_,i)=>({uid:`SIM-${String(i+1).padStart(4,'0')}`}))]) {
  for (const experiment of [null,'exp-entry']) for(const key of Object.keys(SCENARIOS)) {
   const scenario=createScenario(key,users,experiment);
   for(let time=0;time<=scenario.duration+1;time+=.17){
    const s=snapshot(scenario,time);
    assert.equal(s.queued+s.active+s.success+s.failed+s.control,users.length);
    assert.equal(s.nodes.control.arrived,s.control);
    assert.equal(Object.values(s.nodes).reduce((n,x)=>n+x.active,0),s.active+s.queued);
   }
   const end=snapshot(scenario,scenario.duration);
   assert.equal(end.active,0);assert.equal(end.queued,0);
   assert.equal(end.control,scenario.people.filter(p=>p.group==='control').length);
   assert.equal(CHANNELS.reduce((n,c)=>n+end.nodes[c].arrived,0),users.length-end.control);
  }
 }
});


test('downstream experiments conserve every boundary and never deliver either control cohort', () => {
 const cases = [
  [null, {experimentId:'exp-extra',branch:'direct'}],
  ['exp-entry', {experimentId:'exp-extra',branch:'a'}],
  ['exp-entry', {experimentId:'exp-extra',branch:'b'}],
  ['exp-entry', {experimentId:'exp-entry',branch:'a'}],
 ];
 for (const users of [[],MOCK_USERS.slice(0,1),MOCK_USERS]) for (const [entry, extra] of cases) {
  const scenario=createScenario('baseline',users,entry,extra);
  const times=new Set([0,-2,scenario.duration,scenario.duration+20]);
  for(const p of scenario.people) for(const age of [0,4,6,8,9,13,14+p.hold,18+p.hold]) for(const epsilon of [-.00001,0,.00001]) times.add(p.start+age+epsilon);
  for(const time of times){
   const state=snapshot(scenario,time);
   assert.equal(state.queued+state.active+state.success+state.failed+state.control,users.length);
   assert.equal(state.nodes.control.arrived+state.nodes.control2.arrived,state.control);
   assert.equal(Object.values(state.nodes).reduce((sum,n)=>sum+n.active,0),state.queued+state.active);
   for(const node of Object.values(state.nodes))assert.ok(node.active>=0&&node.arrived>=0);
  }
  const end=snapshot(scenario,scenario.duration);
  assert.equal(end.active,0);assert.equal(end.queued,0);
  assert.equal(end.control,scenario.people.filter(p=>p.group==='control'||p.extraGroup==='control2').length);
  assert.equal(CHANNELS.reduce((sum,key)=>sum+end.nodes[key].arrived,0),users.length-end.control);
  assert.ok(scenario.people.filter(p=>p.group==='control').every(p=>p.extraGroup===null));
 }
});

test('the extra experiment only partitions its eligible branch and never changes inherited assignments',()=>{
 const base=createScenario('baseline',MOCK_USERS,'exp-entry');
 const extra=createScenario('baseline',MOCK_USERS,'exp-entry',{experimentId:'exp-extra',branch:'a'});
 const reused=createScenario('baseline',MOCK_USERS,'exp-entry',{experimentId:'exp-entry',branch:'a'});
 for(const person of extra.people){
  assert.equal(person.group,base.people[person.id].group);
  if(person.group==='a')assert.equal(person.extraGroup,`${assignGroup(person.uid,'exp-extra')}2`);
  else assert.equal(person.extraGroup,null);
 }
 assert.ok(reused.people.filter(p=>p.group==='a').every(p=>p.extraGroup==='a2'));
 assert.ok(reused.people.every(p=>p.extraGroup!=='control2'&&p.extraGroup!=='b2'));
 const selected=MOCK_USERS.filter((_,i)=>i%3===0).reverse();
 const filtered=createScenario('baseline',selected,'exp-entry',{experimentId:'exp-extra',branch:'a'});
 for(const p of filtered.people){const original=extra.people.find(x=>x.uid===p.uid);assert.equal(p.group,original.group);assert.equal(p.extraGroup,original.extraGroup);}
});

test('invalid experiment identities and branches fail closed',()=>{
 for(const inherited of [false,true,'',42,'<bad>'])assert.throws(()=>createScenario('baseline',[],inherited),RangeError);
 for(const extra of [{experimentId:'',branch:'a'},{experimentId:'exp',branch:'control'},{experimentId:'exp',branch:'direct'},{}])assert.throws(()=>createScenario('baseline',[],'exp-entry',extra),RangeError);
 assert.throws(()=>createScenario('baseline',[],null,{experimentId:'exp',branch:'a'}),RangeError);
});


test('exact-end seeking finishes every cohort length despite floating-point subtraction',()=>{
 for(let length=1;length<=MOCK_USERS.length;length++)for(const key of Object.keys(SCENARIOS))for(const [entry,extra] of [[null,null],['exp-entry',null],['exp-entry',{experimentId:'exp-extra',branch:'a'}],[null,{experimentId:'exp-extra',branch:'direct'}]]){
  const scenario=createScenario(key,MOCK_USERS.slice(0,length),entry,extra);
  const end=snapshot(scenario,scenario.duration);
  assert.equal(end.active,0,`${length} ${key} ${entry}`);assert.equal(end.queued,0);
  assert.equal(end.success+end.failed+end.control,length);
 }
});


test('the explicit canvas experiment node receives only its selected branch and owns its outgoing split',()=>{
 for (const [entry,extra] of [[null,{experimentId:'exp-extra',branch:'direct'}],['exp-entry',{experimentId:'exp-extra',branch:'a'}],['exp-entry',{experimentId:'exp-entry',branch:'b'}]]) {
  const scenario=createScenario('baseline',MOCK_USERS,entry,extra);
  const branch=scenario.people.filter(p=>p.group===extra.branch);
  assert.ok(branch.length>0);
  const end=snapshot(scenario,scenario.duration);
  assert.equal(end.nodes.flowExperiment.arrived,branch.length);
  assert.equal(end.nodes.flowExperiment.active,0);
  assert.equal(end.nodes.a2.arrived+end.nodes.b2.arrived+end.nodes.control2.arrived,branch.length);
  for(const person of branch){
   assert.equal(personState(person,person.start+4).phase,'extraEntry');
   assert.ok(Math.abs(personState(person,person.start+5).progress-.5)<1e-12);
   assert.equal(personState(person,person.start+6).phase,'extraSplit');
   assert.ok(Math.abs(personState(person,person.start+7).progress-.5)<1e-12);
   assert.notEqual(personState(person,person.start+8).phase,'extraSplit');
  }
  for(let time=0;time<=scenario.duration;time+=.113){
   const state=snapshot(scenario,time);
   const expected=state.states.filter(p=>p.phase==='extraSplit').length;
   assert.equal(state.nodes.flowExperiment.active,expected);
   assert.equal(state.nodes.flowExperiment.arrived,branch.filter(p=>time>=p.start+6).length);
   assert.equal(Object.values(state.nodes).reduce((sum,node)=>sum+node.active,0),state.active+state.queued);
   assert.equal(state.queued+state.active+state.success+state.failed+state.control,MOCK_USERS.length);
  }
 }
 const plain=createScenario();
 assert.deepEqual(snapshot(plain,plain.duration).nodes.flowExperiment,{arrived:0,active:0,success:0,failed:0});
});
