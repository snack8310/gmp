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
  assert.match(html,/离线单文件.*不能读取或保存活动/);
  const script=html.match(/<script>([\s\S]*?)<\/script>/)[1];
  assert.doesNotMatch(script,/^import |^export /m);
  const elements=new Map();
  const element=id=>{if(!elements.has(id))elements.set(id,{value:'',checked:false,style:{},dataset:{},classList:{toggle(){},add(){},remove(){}},setAttribute(){},addEventListener(){}});return elements.get(id);};
  const document={getElementById:element,querySelector:element,querySelectorAll:()=>[],addEventListener(){}};
  const window={location:{hash:'#campaign/activities/campaign-branch',protocol:'file:',origin:'null'},listeners:{},addEventListener(name,fn){this.listeners[name]=fn;}};
  let fetchCalls=0;
  new Script(script).runInNewContext({document,window,matchMedia:()=>({matches:false,addEventListener(){}}),requestAnimationFrame:()=>1,cancelAnimationFrame(){},fetch(){fetchCalls++;throw new Error('Network forbidden');}});
  assert.equal(element('total').textContent,192);
  assert.ok(element('audience-list').innerHTML.includes('墨西哥'));
  assert.ok(element('storage-warning').textContent.includes('存储不可用'));
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(element('campaign-detail').innerHTML.includes('离线单文件没有同源 HTTP 服务'));
  assert.equal(fetchCalls,0);
  window.location.hash='#campaign/activities';window.listeners.hashchange();
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(element('campaign-activities').innerHTML.includes('无法读取活动管理 API'));
  window.location.hash='#campaign/activities/campaign-branch';window.listeners.hashchange();
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(element('campaign-detail').innerHTML.includes('无法读取活动管理 API'));
 } finally {await rm(dir,{recursive:true,force:true});}
});

test('entry configuration controls are confined to audience management, canvas only selects an audience', async()=>{
 const html=await readFile(new URL('./index.html',import.meta.url),'utf8');
 const start=html.indexOf('<section id="flow-section"');
 const audience=html.slice(0,start),canvas=html.slice(start);
 for(const id of ['entry-mode','batch-schedule','batch-time','event-type','rule-source']){
  assert.ok(audience.includes(`id="${id}"`));assert.ok(!canvas.includes(`id="${id}"`));
 }
 assert.ok(canvas.includes('id="flow-audience"'));
 for(const id of ['flow-entry-title','flow-entry-summary','flow-entry-boundary'])assert.ok(canvas.includes(`id="${id}"`));
});

test('flow node tools are inside canvas paths and the inspector, not audience entry controls',async()=>{
 const html=await readFile(new URL('./index.html',import.meta.url),'utf8');
 assert.doesNotMatch(html,/canvas-experiment-enabled|canvas-experiment-branch|画布额外增加实验/);
 const workspace=html.indexOf('<section class="workspace">');
 assert.ok(html.indexOf('id="flow-path-actions"')>workspace);
 assert.ok(html.indexOf('id="flow-node-picker"')>html.indexOf('<aside class="detail-panel"'));
 assert.ok(html.indexOf('id="remove-flow-node"')>workspace);
});

test('domain navigation and sections are ordered and management stays in its domain', async () => {
 const html=await readFile(new URL('./index.html',import.meta.url),'utf8');
 const nav=html.match(/<nav class="domain-nav"[\s\S]*?<\/nav>/)?.[0];
 assert.ok(nav);
 assert.deepEqual([...nav.matchAll(/href="#(audience|experimentation|campaign|compose)"/g)].map(match=>match[1]),['audience','experimentation','campaign','compose']);
 assert.ok(html.indexOf('id="experimentation-section"')<html.indexOf('id="campaign-section"'));
 assert.ok(html.indexOf('id="campaign-section"')<html.indexOf('id="flow-section"'));
 const campaign=html.slice(html.indexOf('id="campaign-section"'),html.indexOf('id="flow-section"'));
 assert.match(campaign,/href="#campaign\/activities"/);
 assert.match(campaign,/id="campaign-activities"/);
 assert.match(campaign,/id="campaign-detail"/);
 assert.doesNotMatch(campaign,/id="play"|开始模拟/);
 assert.match(html,/模拟数据 · 不会真实触达/);
});
