import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { Script } from 'node:vm';

test('standalone export embeds the actual modules, has no external assets, and boots offline', async () => {
 const dir=await mkdtemp(join(tmpdir(),'gmp-export-'));
 try {
  const output=join(dir,'demo.html');
  execFileSync(process.execPath,[new URL('./export.mjs',import.meta.url).pathname,output]);
  const html=await readFile(output,'utf8');
  assert.doesNotMatch(html,/<script[^>]+src=|<link[^>]+stylesheet/i);
  assert.match(html,/\.primary-module-nav/);
  assert.match(html,/活动列表与工作区需要本机管理 API/);
  const script=html.match(/<script>([\s\S]*?)<\/script>/)[1];
  assert.doesNotMatch(script,/^import |^export /m);
  const elements=new Map();
  const element=id=>{if(!elements.has(id))elements.set(id,{value:'',checked:false,style:{},dataset:{},classList:{toggle(){},add(){},remove(){}},setAttribute(){},addEventListener(){}});return elements.get(id);};
  const document={getElementById:element,querySelector:element,querySelectorAll:()=>[],addEventListener(){}};
  const window={location:{hash:'#campaign/activities/campaign-branch',protocol:'file:',origin:'null'},listeners:{},addEventListener(name,fn){this.listeners[name]=fn;}};
  let fetchCalls=0;
  new Script(script).runInNewContext({document,window,matchMedia:()=>({matches:false,addEventListener(){}}),requestAnimationFrame:()=>1,cancelAnimationFrame(){},fetch(){fetchCalls++;throw new Error('Network forbidden');}});
  assert.ok(element('audience-list').innerHTML.includes('墨西哥'));
  assert.ok(element('storage-warning').textContent.includes('存储不可用'));
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(element('activity-not-found-title').textContent,'无法读取活动上下文');
  assert.ok(element('activity-nav-status').textContent.includes('离线单文件没有同源 HTTP 服务'));
  assert.equal(fetchCalls,0);
  window.location.hash='#campaign/activities';window.listeners.hashchange();
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(element('campaign-activities').innerHTML.includes('无法读取活动管理 API'));
  window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(element('activity-not-found-title').textContent,'无法读取活动上下文');
 } finally {await rm(dir,{recursive:true,force:true});}
});

test('entry configuration controls belong to audience management and no standalone sandbox remains', async()=>{
 const html=await readFile(new URL('./index.html',import.meta.url),'utf8');
 for(const id of ['entry-mode','batch-schedule','batch-time','event-type','rule-source']){
  assert.ok(html.includes(`id="${id}"`));
 }
 assert.doesNotMatch(html,/flow-section|flow-audience|COMPOSE \/ SIMULATION|组合沙盘|组合页|触达模拟|开始模拟/);
 assert.doesNotMatch(html,/id="play"|id="timeline"|id="scenario"/);
});

test('global navigation leads directly to activity details and asset lists', async () => {
 const html=await readFile(new URL('./index.html',import.meta.url),'utf8');
 assert.doesNotMatch(html,/domain-nav|nav-campaign|nav-compose/);
 const primary=html.match(/<nav class="primary-module-nav"[\s\S]*?<\/nav>/)?.[0];
 assert.ok(primary);
 assert.deepEqual([...primary.matchAll(/id="module-(activities|audience|experimentation)"/g)].map(match=>match[1]),['activities','audience','experimentation']);
 assert.match(html,/id="activity-list" class="activity-list"/);
 assert.match(html,/id="lab-shell" class="lab-shell" hidden/);
 assert.match(html,/id="activity-route-loading" class="activity-route-loading"/);
 assert.match(html,/id="asset-return-link"/);
 const landing=html.slice(html.indexOf('id="activity-landing"'),html.indexOf('id="activity-not-found"'));
 assert.match(landing,/id="campaign-activities"/);
 assert.ok(html.indexOf('id="experimentation-section"')<html.indexOf('id="campaign-section"'));
 const campaign=html.slice(html.indexOf('id="campaign-section"'));
 assert.match(campaign,/id="campaign-detail"/);
 assert.match(campaign,/id="campaign-editor"/);
 assert.match(html,/模拟数据 · 不会真实触达/);
});
