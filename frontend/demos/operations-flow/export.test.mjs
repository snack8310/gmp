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
  assert.doesNotMatch(html,/<script[^>]+src=|<link[^>]+stylesheet|https?:\/\//i);
  const script=html.match(/<script>([\s\S]*?)<\/script>/)[1];
  assert.doesNotMatch(script,/^import |^export /m);
  const elements=new Map();
  const element=id=>{if(!elements.has(id))elements.set(id,{value:'',checked:false,style:{},dataset:{},classList:{toggle(){},add(){},remove(){}},setAttribute(){},addEventListener(){}});return elements.get(id);};
  const document={getElementById:element,querySelector:element,querySelectorAll:()=>[],addEventListener(){}};
  const window={addEventListener(){}};
  new Script(script).runInNewContext({document,window,matchMedia:()=>({matches:false,addEventListener(){}}),requestAnimationFrame:()=>1,cancelAnimationFrame(){},fetch(){throw new Error('Network forbidden');}});
  assert.equal(element('total').textContent,192);
  assert.ok(element('audience-list').innerHTML.includes('墨西哥'));
  assert.ok(element('storage-warning').textContent.includes('存储不可用'));
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
