import { filterAudience, summarizeRules, summarizeEntry } from './audience.mjs';
import { mountAudiences } from './audience-ui.mjs';
import { mountExperimentPicker, mountExperiments, escapeHTML } from './experiment-ui.mjs';
import { CHANNELS, SCENARIOS, createScenario, snapshot, createClock } from './model.mjs';

const $ = id => document.getElementById(id);
const icons = {
 audience: '<circle cx="9" cy="8" r="3"/><path d="M3 20v-3a6 6 0 0 1 12 0v3M16 5a3 3 0 0 1 0 6m2 3a5 5 0 0 1 3 5"/>',
 direct: '<path d="M3 12h18m-6-6 6 6-6 6"/>', control: '<path d="M7 5v14M17 5v14"/>',
 a: '<path d="M5 20 12 4l7 16M8 14h8"/>', b: '<path d="M6 4h7a4 4 0 0 1 0 8H6zm0 8h8a4 4 0 0 1 0 8H6z"/>',
 sms: '<path d="M3 5h18v13H8l-5 3zM7 9h10M7 13h6"/>',
 push: '<path d="M5 17h14l-2-4V9a5 5 0 0 0-10 0v4zm4 3h6M12 2v2"/>',
 whatsapp: '<path d="M4 17 2 22l6-2a9 9 0 1 0-4-3zM8 7c0 5 4 9 9 9l1-3-3-1-1 1-3-3 1-1-1-3z"/>',
 ivr: '<path d="m7 3 4 5-3 3 5 5 3-3 5 4-2 4C10 22 2 14 3 5z"/>',
 success: '<circle cx="12" cy="12" r="9"/><path d="m7 12 3 3 7-7"/>', failed: '<circle cx="12" cy="12" r="9"/><path d="m8 8 8 8m0-8-8 8"/>',
};
const icon = key => `<svg viewBox="0 0 24 24" aria-hidden="true">${icons[key]}</svg>`;
const nodes = [
 {id:'audience',name:'人群池',kind:'AUDIENCE SOURCE',x:-370,z:0,w:130,d:125,h:14,desc:'等待进入本次模拟的人群。所有样本仅存在于浏览器内存，不是真实用户。',rules:[['来源','构造样本'],['人群规模','240 人'],['数据连接','无']]},
 {id:'direct',name:'直接触达',kind:'NO EXPERIMENT',x:5,z:0,w:65,d:65,h:32,desc:'A/B 已关闭。符合属性条件的全部模拟用户直接进入渠道，不做随机分组。',rules:[['分组','关闭'],['来源','已保存人群定义']]},
 {id:'control',name:'对照组',kind:'NO DELIVERY CONTROL',x:5,z:290,w:65,d:65,h:32,desc:'符合人群条件、但被稳定分配为对照组的模拟用户。不触达，不计入成功或失败。',rules:[['目标区间','10%（实际人数可偏离）'],['触达','不执行'],['算法','展示专用 v1']]},
 {id:'a',name:'A 组',kind:'SIMULATED SEGMENT A',x:5,z:-155,w:65,d:65,h:32,desc:'通过展示专用稳定哈希分配到 A 组。先符合属性条件，再随机分组；不调用生产分桶，不写染色。',rules:[['目标区间','45%'],['触达渠道','按脚本分配'],['分组算法','展示专用']]},
 {id:'b',name:'B 组',kind:'SIMULATED SEGMENT B',x:5,z:155,w:65,d:65,h:32,desc:'展示专用 B 组，与 A 组及对照组互斥。目标占比是分桶区间，不保证小样本人数精确相等。',rules:[['目标区间','45%'],['触达渠道','按脚本分配'],['生产分桶','未调用']]},
 {id:'sms',name:'短信',kind:'SMS CHANNEL',x:200,z:-245,w:60,d:56,h:65,desc:'模拟短信通道的发送与回执。等待阶段仍为处理中，到终点才记成功或失败。',rules:[['预设分配','25%'],['服务连接','无'],['回执来源','预设脚本']]},
 {id:'push',name:'Push',kind:'PUSH CHANNEL',x:200,z:-80,w:60,d:56,h:65,desc:'模拟 App 推送通道。画面中的成功仅指回执成功，不代表用户已阅读。',rules:[['预设分配','25%'],['设备数据','无'],['回执来源','预设脚本']]},
 {id:'whatsapp',name:'WhatsApp',kind:'WHATSAPP CHANNEL',x:200,z:80,w:60,d:56,h:65,desc:'模拟跨境消息通道。在回执延迟场景下停留更久，等待中的用户不会被提前判成失败。',rules:[['预设分配','25%'],['服务连接','无'],['等待时长','随场景变化']]},
 {id:'ivr',name:'IVR',kind:'VOICE CHANNEL',x:200,z:245,w:60,d:56,h:65,desc:'模拟语音触达流程。不拨打电话，不使用号码，也不连接呼叫服务。',rules:[['预设分配','25%'],['真实拨号','无'],['回执来源','预设脚本']]},
 {id:'success',name:'触达成功',kind:'SUCCESS TERMINAL',x:410,z:-135,w:92,d:84,h:34,desc:'已抵达终点并获得模拟成功回执的虚拟用户。它不是阅读率、下单率或业务转化。',rules:[['状态','终态'],['口径','模拟回执成功'],['业务转化','未模拟']]},
 {id:'failed',name:'触达失败',kind:'FAILURE TERMINAL',x:410,z:145,w:92,d:84,h:24,desc:'预设脚本产生的终态失败。延迟回执与尚未进入的人不计入这里；不自动补发。',rules:[['状态','终态'],['失败原因','模拟渠道拒绝'],['真实重试','无']]},
];
icons.flowExperiment='<path d="M5 4h14v16H5zM8 8h8m-8 4h8m-8 4h5"/>';
icons.a2=icons.a;icons.b2=icons.b;icons.control2=icons.control;
nodes.push({id:'flowExperiment',name:'流程实验',kind:'FLOW EXPERIMENT NODE',x:-60,z:0,w:62,d:62,h:50,desc:'进入后的独立流程实验节点。仅对从指定路径到达本节点的模拟用户分组，不改入口人群或继承实验。',rules:[]},{id:'a2',name:'A1 组',kind:'ADDITIONAL EXPERIMENT A',x:70,z:-245,w:53,d:53,h:28,desc:'画布额外实验的 A 出口，只从所选分支接收用户，不改变继承实验分组。',rules:[]},{id:'b2',name:'A2 组',kind:'ADDITIONAL EXPERIMENT B',x:70,z:-85,w:53,d:53,h:28,desc:'画布额外实验的 B 出口。分母是所选分支，不是整个人群。',rules:[]},{id:'control2',name:'副对照',kind:'ADDITIONAL NO DELIVERY',x:70,z:90,w:53,d:53,h:28,desc:'画布额外实验的对照出口，不进入触达渠道，不属于失败。',rules:[]});
const byId = Object.fromEntries(nodes.map(n => [n.id,n]));
let currentAudience = null;let extraConfig=null;let canvasPicker;
const visibleNodes = () => nodes.filter(n => ['flowExperiment','a2','b2','control2'].includes(n.id)?Boolean(scenario.extra):scenario.experimentId?n.id!=='direct':!['a','b','control'].includes(n.id));
let selected = 'audience', scenario = createScenario(), data = snapshot(scenario,0);
let yaw = -.28, pitch = .55, zoom = 1, reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
const motionQuery = matchMedia('(prefers-reduced-motion: reduce)');
let dragging = false, dragStart = null, ignoreClick = false, pressedNode = null;
const colors = {blue:'#2c4a7c',paper:'#fbfaf7',white:'#ffffff',line:'#cfc9be',side:'#e4e0d8',light:'#f5f3ee',ink:'#55504a'};
function project(x,y,z) {
 const rx = x * Math.cos(yaw) - z * Math.sin(yaw), rz = x * Math.sin(yaw) + z * Math.cos(yaw);
 return [550 + rx * zoom, 345 + (rz * pitch - y * Math.sqrt(1-pitch*pitch)) * zoom];
}
const point = p => p.map(v=>v.toFixed(1)).join(',');
const points = vertices => vertices.map(v=>point(project(...v))).join(' ');
function poly(vertices,fill,stroke=colors.line,width=.8,extra='') {return `<polygon points="${points(vertices)}" fill="${fill}" stroke="${stroke}" stroke-width="${width}" ${extra}/>`;}
function cube(x,z,w,d,h,bottom=0,accent=false) {
 const a=[x-w/2,bottom,z-d/2], b=[x+w/2,bottom,z-d/2], c=[x+w/2,bottom,z+d/2], e=[x-w/2,bottom,z+d/2];
 const top = v => [v[0],bottom+h,v[2]];
 const sides=[[a,b,top(b),top(a)],[b,c,top(c),top(b)],[c,e,top(e),top(c)],[e,a,top(a),top(e)]];
 sides.sort((f,g)=>depth(f.reduce((s,v)=>s+v[0],0)/4,f.reduce((s,v)=>s+v[2],0)/4)-depth(g.reduce((s,v)=>s+v[0],0)/4,g.reduce((s,v)=>s+v[2],0)/4));
 return sides.map((face,i)=>poly(face,accent?(i%2?'#2c4a7c':'#1f365b'):(i%2?colors.side:'#ece8df'),accent?colors.blue:colors.line,.7)).join('')+poly([top(a),top(b),top(c),top(e)],accent?colors.blue:colors.white,accent?colors.blue:colors.line);
}
function depth(x,z){return x*Math.sin(yaw)+z*Math.cos(yaw);}
function line(vertices,stroke,width=1,dash='') {return `<polyline points="${points(vertices)}" fill="none" stroke="${stroke}" stroke-width="${width}" stroke-linecap="round" stroke-linejoin="round" ${dash?'stroke-dasharray="'+dash+'"':''}/>`;}
function route(from,to) {
 const a=byId[from],b=byId[to];
 return [[a.x,17,a.z],[a.x+(b.x-a.x)*.46,17,a.z],[a.x+(b.x-a.x)*.46,17,b.z],[b.x,17,b.z]];
}
function pathPosition(vertices,t){
 const distances=vertices.slice(1).map((v,i)=>Math.hypot(...v.map((n,j)=>n-vertices[i][j])));
 let remaining=t*distances.reduce((a,b)=>a+b,0);
 for(let i=0;i<distances.length;i++){if(remaining<=distances[i]||i===distances.length-1){const p=Math.min(1,remaining/(distances[i]||1));return vertices[i].map((n,j)=>n+(vertices[i+1][j]-n)*p);}remaining-=distances[i];}
 return vertices.at(-1);
}
function personPosition(state){
 const {person:p,phase,progress}=state;
 if(phase==='queued')return [-428+(p.id%20)*6,19,-36+Math.floor(p.id/20)*6];
 if(phase==='entry')return pathPosition(route('audience',p.group),progress);
 if(phase==='extraEntry')return pathPosition(route(p.group,'flowExperiment'),progress);
 if(phase==='extraSplit')return pathPosition(route('flowExperiment',p.extraGroup),progress);
 if(phase==='dispatch')return pathPosition(route(p.extraGroup||p.group,p.channel),progress);
 if(phase==='waiting'){const n=byId[p.channel];return [n.x-40-(p.id%4)*3,17,n.z-22+Math.floor(p.id/4)%12*4];}
 if(phase==='result')return pathPosition(route(p.channel,p.failed?'failed':'success'),progress);
 const n=byId[state.node||phase];return [n.x+58+(Math.floor(p.id/4)%8)*4,17,n.z-30+(p.id%16)*4];
}
function personFigure(state){
 const p=personPosition(state), [x,y]=project(...p), scale=zoom*(state.phase==='queued'?.63:.82);
 const fill=state.person.group==='a'?colors.blue:colors.ink;
 return {z:depth(p[0],p[2])+4,svg:`<g transform="translate(${x.toFixed(1)} ${y.toFixed(1)}) scale(${scale})" aria-hidden="true"><ellipse cy="1" rx="4.8" ry="2" fill="#1c1a17" opacity=".12"/><path d="M-2-2v-5q2-2 4 0v5M-1-2v3M1-2v3" fill="${fill}" stroke="${fill}" stroke-width="1.8" stroke-linecap="round"/><circle cy="-11" r="2.5" fill="${fill}"/></g>`};
}
function building(n){
 let svg=cube(n.x,n.z,n.w+16,n.d+16,8,0)+cube(n.x,n.z,n.w,n.d,7,8);
 if(n.id==='audience'){
  svg+=cube(n.x-56,n.z-63,6,8,34,15)+cube(n.x+56,n.z-63,6,8,34,15)+cube(n.x,n.z-63,118,8,9,49,true);
 } else if(n.id==='flowExperiment'){
  svg+=cube(n.x,n.z,44,44,34,15)+cube(n.x,n.z,48,48,5,49,true);
  const [x,y]=project(n.x,65,n.z);svg+=`<text x="${x}" y="${y}" text-anchor="middle" font-size="22" fill="${colors.blue}">E</text>`;
 } else if(['a','b','direct','control','a2','b2','control2'].includes(n.id)){
  svg+=cube(n.x,n.z,43,43,20,15)+cube(n.x,n.z,43,43,3,35,true);
  const [x,y]=project(n.x,46,n.z);svg+=`<text x="${x}" y="${y}" text-anchor="middle" font-family="Georgia,serif" font-size="24" fill="${colors.blue}">${n.id==='direct'?'→':n.id.startsWith('control')?'Ⅱ':n.id==='a2'?'1':n.id==='b2'?'2':n.id.toUpperCase()}</text>`;
 } else if(CHANNELS.includes(n.id)){
  svg+=cube(n.x,n.z,40,35,51,15)+cube(n.x,n.z,44,39,6,66,true);
  svg+=cube(n.x-15,n.z+19,9,8,34,15,true);
  const [x,y]=project(n.x,53,n.z+21);svg+=`<g transform="translate(${x-10} ${y-12}) scale(.8)" fill="none" stroke="${colors.blue}" stroke-width="1.6">${icons[n.id]}</g>`;
 }else{
  for(let i=0;i<3;i++)svg+=cube(n.x-24+i*24,n.z,19,36,n.id==='success'?20+i*10:12,15,n.id==='success');
 }
 if(n.id===selected)svg+=line([[n.x-n.w/2-12,9,n.z-n.d/2-12],[n.x+n.w/2+12,9,n.z-n.d/2-12],[n.x+n.w/2+12,9,n.z+n.d/2+12],[n.x-n.w/2-12,9,n.z+n.d/2+12],[n.x-n.w/2-12,9,n.z-n.d/2-12]],colors.blue,2);
 return {z:depth(n.x,n.z),svg:`<g class="scene-node" data-node="${n.id}" aria-label="${n.name}"><title>${n.name}：点击查看节点详情</title>${svg}</g>`};
}
function drawScene(){
 $('ground').innerHTML=cube(0,0,1040,680,9,-9)+poly([[-520,.2,-340],[520,.2,-340],[520,.2,340],[-520,.2,340]],colors.paper,colors.line)+Array.from({length:18},(_,i)=>line([[-500,1,-320+i*38],[500,1,-320+i*38]],colors.line,.35)).join('')+Array.from({length:27},(_,i)=>line([[-500+i*38,1,-320],[-500+i*38,1,320]],colors.line,.35)).join('');
 const groups=scenario.experimentId?['a','b']:['direct'];
 const routed=groups.filter(g=>g!==scenario.extra?.branch);
 const connections=[...groups.map(g=>['audience',g]),...(scenario.experimentId?[['audience','control']]:[]),...routed.flatMap(g=>CHANNELS.map(c=>[g,c])),...(scenario.extra?[[scenario.extra.branch,'flowExperiment'],...['a2','b2','control2'].map(g=>['flowExperiment',g]),...['a2','b2'].flatMap(g=>CHANNELS.map(c=>[g,c]))]:[]),...CHANNELS.flatMap(c=>[[c,'success'],[c,'failed']])];
 $('routes').innerHTML=connections.map(([a,b])=>line(route(a,b),colors.white,9)+line(route(a,b),b==='failed'?colors.ink:colors.blue,1.25,b==='failed'?'4 5':'')).join('');
 const figures = reduced ? [] : data.states.map(personFigure);
 // Painter ordering uses camera depth for solids and people.
 $('objects').innerHTML=[...visibleNodes().map(building),...figures].sort((a,b)=>a.z-b.z).map(o=>o.svg).join('');
 $('labels').innerHTML=visibleNodes().map(n=>{
  const [x,y]=CHANNELS.includes(n.id)?project(n.x-(scenario.extra?0:84),scenario.extra?100:24,n.z):project(n.x,0,n.z+n.d/2+22), width=n.id==='flowExperiment'?126:n.id==='whatsapp'?100:86;
  const count=data.nodes[n.id].arrived;
  if(n.id==='flowExperiment')return `<g class="scene-node ${selected===n.id?'selected-label':''}" data-node="flowExperiment" transform="translate(${x.toFixed(1)},${y.toFixed(1)})"><title>${escapeHTML(n.experimentName||'流程实验节点')}</title><rect class="node-label-back node-outline" x="-63" y="-2" width="126" height="52" rx="3"/><text class="scene-label" text-anchor="middle" y="13">流程实验节点</text><text class="scene-sub" text-anchor="middle" y="28">${escapeHTML((n.experimentName||'').slice(0,9))}${(n.experimentName||'').length>9?'…':''}</text><text class="scene-sub" text-anchor="middle" y="42">${count} 累计到达</text></g>`;
  return `<g class="scene-node ${selected===n.id?'selected-label':''}" data-node="${n.id}" transform="translate(${x.toFixed(1)},${y.toFixed(1)})"><rect class="node-label-back node-outline" x="${-width/2}" y="-2" width="${width}" height="38" rx="3"/><text class="scene-label" text-anchor="middle" y="13">${escapeHTML(n.name)}</text><text class="scene-sub" text-anchor="middle" y="28">${count} ${n.id==='audience'?'样本':'累计'}</text></g>`;
 }).join('');
 if(currentAudience&&!extraConfig&&audienceUI.store.experiments().length){
  $('labels').innerHTML+=(scenario.experimentId?['a','b']:['direct']).map(branch=>{
   const n=byId[branch],[x,y]=project(n.x+105,8,n.z+35);
   return `<g class="scene-node add-flow-node" data-node="add-${branch}" transform="translate(${x},${y})"><rect x="-53" y="-13" width="106" height="26" rx="4" fill="#ebf0f7" stroke="#2c4a7c" stroke-dasharray="3 2"/><text text-anchor="middle" y="4" font-size="11" fill="#2c4a7c">＋ 添加实验节点</text></g>`;
  }).join('');
 }
}
function renderDetails(){
 const n=byId[selected],s=data.nodes[selected];
 $('flow-node-editor').hidden=selected!=='flowExperiment'||!extraConfig;
 $('detail-index').textContent=String(nodes.indexOf(n)+1).padStart(2,'0');
 $('detail-icon').innerHTML=icon(n.id);$('detail-kind').textContent=n.kind;$('detail-title').textContent=n.name;$('detail-desc').textContent=n.desc;
 $('arrived-label').textContent=n.id==='audience'?'固定样本':'累计到达';$('detail-arrived').textContent=s.arrived;$('detail-active').textContent=s.active;
 $('detail-progress').style.width=(data.total?s.arrived/data.total*100:0)+'%';
 $('detail-rules').innerHTML=n.rules.map(([a,b])=>`<div><span>${escapeHTML(a)}</span><strong>${escapeHTML(b)}</strong></div>`).join('')+(CHANNELS.includes(n.id)?`<div><span>终态成功 / 失败</span><strong>${s.success} / ${s.failed}</strong></div>`:'');
}
const timeFormat = t => `${String(Math.floor(t/60)).padStart(2,'0')}:${(t%60).toFixed(1).padStart(4,'0')}`;
let lastTime=-1,lastPlaying=false;
function render(time,playing){
 if(playing===lastPlaying&&playing&&Math.abs(time-lastTime)<.04)return;
 lastTime=time;lastPlaying=playing;data=snapshot(scenario,time);
 for(const k of ['total','queued','active','success','failed','control'])$(k).textContent=data[k];
 $('balance').textContent=`${data.queued+data.active+data.success+data.failed+data.control} / ${data.total}`;
 $('time').textContent=timeFormat(time);$('timeline').value=time;
 $('play').disabled=data.total===0;
 $('play').textContent=data.total===0?'无人可模拟':playing?'暂停模拟':time>=scenario.duration?'重新播放':time===0?'开始模拟':'继续模拟';
 $('play-state').textContent=playing?'模拟运行中':time>=scenario.duration?'模拟结束':'已暂停';
 for(const n of visibleNodes())document.querySelector(`[data-list-node="${n.id}"] .count`).textContent=data.nodes[n.id].arrived;
 renderDetails();drawScene();
}
const clock=createClock({duration:scenario.duration,render,request:cb=>requestAnimationFrame(cb),cancel:id=>cancelAnimationFrame(id)});
function rebuildNodeList(){ $('node-list').innerHTML=visibleNodes().map(n=>`<button class="node-button ${n.id===selected?'active':''}" data-node="${n.id}" data-list-node="${n.id}" aria-pressed="${n.id===selected}">${icon(n.id)}<span>${n.name}</span><span class="count">0</span></button>`).join('');$('node-total').textContent=visibleNodes().length; }
rebuildNodeList();
function selectNode(id){
 if(id?.startsWith('add-')){addFlowNode(id.slice(4));return;}
 if(!byId[id])return;selected=id;
 document.querySelectorAll('[data-list-node]').forEach(b=>{const on=b.dataset.node===id;b.classList.toggle('active',on);b.setAttribute('aria-pressed',on);});
 renderDetails();drawScene();$('status-message').textContent=`已选择${byId[id].name}，详情已更新。`;
}
$('node-list').addEventListener('click',e=>{const b=e.target.closest('[data-node]');if(b)selectNode(b.dataset.node);});
$('play').addEventListener('click',()=>{if(!data.total)return;if(clock.playing)clock.pause();else{if(clock.time>=scenario.duration)clock.reset();clock.play();}$('status-message').textContent=clock.playing?'模拟已开始，仅使用构造数据。':'模拟已暂停。';});
$('reset').addEventListener('click',()=>{clock.reset();$('status-message').textContent='模拟已重置并暂停。';});
$('speed').addEventListener('change',e=>clock.setSpeed(Number(e.target.value)));
$('timeline').addEventListener('input',e=>clock.seek(e.target.value));
$('scenario').addEventListener('change',()=>applyAudience(currentAudience,true));

function view(mode){pitch=mode==='top'?.96:.55;yaw=mode==='top'?0:-.28;zoom=1;$('isometric').classList.toggle('active',mode!=='top');$('top').classList.toggle('active',mode==='top');$('isometric').setAttribute('aria-pressed',mode!=='top');$('top').setAttribute('aria-pressed',mode==='top');drawScene();}
$('isometric').addEventListener('click',()=>view('iso'));$('top').addEventListener('click',()=>view('top'));
$('view-reset').addEventListener('click',()=>view('iso'));
$('zoom-in').addEventListener('click',()=>{zoom=Math.min(1.25,zoom+.1);drawScene();});$('zoom-out').addEventListener('click',()=>{zoom=Math.max(.7,zoom-.1);drawScene();});
// Keep the pressed node ID across frame redraws; the SVG element itself may disappear.
$('world').addEventListener('pointerdown',e=>{
 if(e.pointerType!=='touch'&&e.button!==0)return;
 dragStart={x:e.clientX,y:e.clientY,yaw,touch:e.pointerType==='touch'};
 pressedNode=e.target.closest('[data-node]')?.dataset.node||null;
 dragging=true;ignoreClick=false;if(!dragStart.touch)$('world').classList.add('dragging');
});
$('world').addEventListener('pointermove',e=>{
 if(!dragging)return;const dx=e.clientX-dragStart.x,dy=e.clientY-dragStart.y;
 if(Math.hypot(dx,dy)>4&&!ignoreClick){ignoreClick=true;if(!dragStart.touch)$('world').setPointerCapture(e.pointerId);}
 if(!ignoreClick||dragStart.touch)return;
 yaw=Math.max(-.7,Math.min(.35,dragStart.yaw+dx*.002));drawScene();
});
function endDrag(){dragging=false;pressedNode=null;$('world').classList.remove('dragging');}
$('world').addEventListener('pointerup',()=>{const id=dragging&&!ignoreClick?pressedNode:null;endDrag();if(id)selectNode(id);});
$('world').addEventListener('pointercancel',endDrag);
$('world').addEventListener('lostpointercapture',endDrag);
$('world').addEventListener('pointerleave',()=>{if(!ignoreClick)endDrag();});
function updateMotion(){reduced=motionQuery.matches;if(reduced)clock.pause();$('motion-note').innerHTML=reduced?'减少动态效果<br>仅更新数值':'SVG 立体投影<br>无需 WebGL';drawScene();}
motionQuery.addEventListener('change',updateMotion);
document.addEventListener('visibilitychange',()=>{if(document.hidden)clock.pause();});
window.addEventListener('pagehide',()=>clock.pause(),{once:true});
function applyAudience(item,preserveExtra=false){
 clock.pause();currentAudience=item;
 if(!preserveExtra)extraConfig=null;
 const users=item?filterAudience(item.rules):[];
 const experimentId=item?.experimentId||null;
 scenario=createScenario($('scenario').value||'baseline',users,experimentId,extraConfig);
 selected='audience';
 const experiments=audienceUI.store.experiments();
 const inherited=experiments.find(e=>e.id===experimentId),additional=experiments.find(e=>e.id===extraConfig?.experimentId);
 byId.audience.rules=[['引用人群',item?item.name:'尚未选择'],['入口配置',item?summarizeEntry(item):'无'],['匹配人数',`${users.length} 位模拟用户`],['继承实验',inherited?.name||'无 · 单一出口']];
 const branchPrefix=extraConfig?.branch==='b'?'B':extraConfig?.branch==='a'?'A':'分支';
 byId.a2.name=`${branchPrefix}1 组`;byId.b2.name=`${branchPrefix}2 组`;
 // Main exits move left only when an additional stage needs canvas space.
 for(const id of ['a','b','direct','control'])byId[id].x=extraConfig?-200:-75;
 for(const id of ['a','b','control'])byId[id].rules=[['继承实验',inherited?.name||'无'],['ID',experimentId||'无'],['作用分母',`${users.length} 位符合条件用户`],['生产分桶','未接入']];
 if(extraConfig){byId.flowExperiment.z=byId[extraConfig.branch].z;const base=extraConfig.branch==='a'?-110:extraConfig.branch==='b'?120:0;byId.a2.z=base-140;byId.b2.z=base;byId.control2.z=base+140;}
 const branchCount=scenario.people.filter(p=>p.group===extraConfig?.branch).length;
 for(const id of ['a2','b2','control2'])byId[id].rules=[['流程节点实验',additional?.name||'无'],['ID',additional?.id||'无'],['分支分母',`${branchCount} 位`],['分配',extraConfig?.experimentId===experimentId?'复用同一结果 · 不重新随机':'独立展示实验']];
 byId.flowExperiment.experimentName=additional?.name||'';
 byId.flowExperiment.rules=[['引用实验',additional?.name||'无'],['ID',additional?.id||'无'],['到达路径',extraConfig?.branch==='direct'?'普通出口':`${extraConfig?.branch?.toUpperCase()||''} 分支`],['路径分母',`${branchCount} 位`]];
 byId.flowExperiment.desc=additional?`引用「${additional.name}」。只对到达本节点的人分组，不改变入口绑定。`:'独立流程实验节点';
 $('flow-node-scope').textContent=extraConfig?`${extraConfig.branch==='direct'?'普通出口':extraConfig.branch.toUpperCase()+' 分支'} → 本节点；分母 ${branchCount} 位。`:'尚未添加流程节点';
 $('flow-path-actions').innerHTML=!item?'请先选择人群':extraConfig?'<button data-inspect-flow-node="true">编辑流程实验节点 ↗</button>':!experiments.length?'暂无可引用实验，请先在实验域新建。':(experimentId?['a','b']:['direct']).map(branch=>`<button data-add-branch="${branch}">${branch==='direct'?'普通出口':branch.toUpperCase()+' 分支'} → ＋ 添加实验节点</button>`).join('');
 rebuildNodeList();
 $('flow-entry-title').textContent=item?(item.entry.mode==='event'?'事件人群 · 已保存触发配置':'批量人群 · 已保存来源与排期'):'请先创建人群';
 $('flow-entry-summary').textContent=item?summarizeEntry(item):'画布入口只引用受众中的已保存定义。';
 $('flow-entry-boundary').textContent=item?.entry.mode==='event'?'仅模拟符合条件样本依次进入；不是收到的事件数。不订阅真实事件，多画布事件分发与运行时投递尚未定义。':'仅模拟符合条件样本进入；不会执行已配置的排期。修改模式、来源或排期，请回到「编辑此人群」。';
 $('flow-reference').textContent=item?`引用：${item.name} · ${summarizeRules(item.rules)}`:'暂无人群定义，请先到受众新建。';
 const count=g=>scenario.people.filter(p=>p.group===g).length,count2=g=>scenario.people.filter(p=>p.extraGroup===g).length;
 $('inherited-experiment-name').textContent=inherited?`${inherited.name} · ${inherited.id}`:'此人群未引用实验 · 单一出口';
 $('inherited-experiment-note').textContent=inherited?`随人群继承，不可在画布取消或替换。分母 ${users.length}：A ${count('a')} / B ${count('b')} / 对照 ${count('control')}（不触达）。请通过「编辑此人群」修改引用。`:'圈选条件决定谁符合。可以编辑人群引用实验，进入后的路径可以添加独立流程实验节点。';
 $('split-summary').textContent=additional?`流程节点：${additional.name} · ${extraConfig.branch==='direct'?'整个人群':extraConfig.branch.toUpperCase()+' 分支'}分母 ${branchCount} → ${byId.a2.name} ${count2('a2')} / ${byId.b2.name} ${count2('b2')} / 副对照 ${count2('control2')}${extraConfig.experimentId===experimentId?' · 同一实验，复用原分配':''}`:'进入后普通流程 · 可在路径添加实验节点';
 $('flow-audience').value=item?.id||'';$('edit-flow-audience').disabled=!item;
 $('timeline').max=scenario.duration;$('duration').textContent=timeFormat(scenario.duration);$('scenario-note').textContent=scenario.config.note+(users.length?'':' 当前匹配为零，请修改人群条件。');
 clock.reset(scenario.duration);$('status-message').textContent='人群或配置已更新，沙盘重置并暂停。';
}
function refreshAudienceOptions(items){$('flow-audience').innerHTML=items.map(a=>`<option value="${escapeHTML(a.id)}">${escapeHTML(a.name)}</option>`).join('')||'<option value="">暂无人群</option>';}
function refreshExperiments(){audienceUI.refreshExperiments();canvasPicker?.refresh();if(extraConfig&&!audienceUI.store.experiments().some(item=>item.id===extraConfig.experimentId))extraConfig=null;if(currentAudience)applyAudience(currentAudience,true);}
const domains={audience:'audience-section',experimentation:'experimentation-section',campaign:'campaign-section',compose:'flow-section'};
function renderDomain(){
 const domain=Object.hasOwn(domains,window.location.hash.slice(1))?window.location.hash.slice(1):'audience';
 for(const [key,section] of Object.entries(domains)){$(section).hidden=key!==domain;$(`nav-${key}`).setAttribute('aria-current',key===domain?'page':'false');}
 if(domain!=='compose')clock.pause();
}
function navigate(domain){window.location.hash=`#${domain}`;renderDomain();}
window.addEventListener('hashchange',renderDomain);
const audienceUI=mountAudiences({onUse:item=>applyAudience(item),onSaved:(saved,items)=>{refreshAudienceOptions(items);if(!currentAudience||saved.id===currentAudience.id)applyAudience(saved);else $('flow-audience').value=currentAudience.id;},onDeleted:(id,items)=>{refreshAudienceOptions(items);if(currentAudience?.id===id)applyAudience(items[0]||null);else $('flow-audience').value=currentAudience?.id||'';},onNavigate:navigate});
mountExperiments({store:audienceUI.store,onChanged:refreshExperiments});
function addFlowNode(branch){
 if(!currentAudience||extraConfig||!(currentAudience.experimentId?['a','b']:['direct']).includes(branch))return;
 const experiment=audienceUI.store.experiments()[0];if(!experiment){$('status-message').textContent='请先在实验域新建可引用实验。';return;}
 extraConfig={experimentId:experiment.id,branch};canvasPicker.setValue(experiment.id);applyAudience(currentAudience,true);selectNode('flowExperiment');
 $('status-message').textContent=`已在${branch==='direct'?'普通出口':branch.toUpperCase()+' 分支'}后插入流程实验节点，可在检查器更换引用；入口未改变。`;
}
function changeFlowNode(){if(!extraConfig)return;const experimentId=canvasPicker.getValue();if(!experimentId)return;extraConfig={...extraConfig,experimentId};applyAudience(currentAudience,true);selectNode('flowExperiment');}
canvasPicker=mountExperimentPicker({rootId:'flow-node-picker',store:audienceUI.store,onChange:changeFlowNode});
$('flow-path-actions').addEventListener('click',event=>{const add=event.target.closest('[data-add-branch]');if(add)addFlowNode(add.dataset.addBranch);else if(event.target.closest('[data-inspect-flow-node]'))selectNode('flowExperiment');});
$('remove-flow-node').addEventListener('click',()=>{if(!extraConfig)return;extraConfig=null;applyAudience(currentAudience,true);$('status-message').textContent='已删除进入后的流程实验节点，入口人群和继承实验未改变。';});
refreshAudienceOptions(audienceUI.list());applyAudience(audienceUI.list()[0]||null);
$('flow-audience').addEventListener('change',e=>applyAudience(audienceUI.list().find(a=>a.id===e.target.value)||null));
$('edit-flow-audience').addEventListener('click',()=>{if(currentAudience)audienceUI.edit(currentAudience.id);});
updateMotion();
renderDomain();
