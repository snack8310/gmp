import { CHANNELS, SCENARIOS, createScenario, snapshot, createClock } from './model.mjs';

const $ = id => document.getElementById(id);
const icons = {
 audience: '<circle cx="9" cy="8" r="3"/><path d="M3 20v-3a6 6 0 0 1 12 0v3M16 5a3 3 0 0 1 0 6m2 3a5 5 0 0 1 3 5"/>',
 strategy: '<path d="M4 4h16l-6 7v8l-4 2V11z"/>',
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
 {id:'strategy',name:'策略闸机',kind:'STRATEGY GATE',x:-170,z:0,w:90,d:74,h:70,desc:'让人群沿固定时间轴进入分流。这是展示闸机，不执行生产资格或决策表规则。',rules:[['策略模式','固定演示脚本'],['生产决策','未接入'],['后续流向','A / B 组']]},
 {id:'a',name:'A 组',kind:'SIMULATED SEGMENT A',x:5,z:-155,w:65,d:65,h:32,desc:'固定脚本分配的 A 组，与 B 组互斥。这里不使用产品分桶、染色或对照组语义。',rules:[['预设占比','50%'],['触达渠道','按脚本分配'],['分组算法','展示专用']]},
 {id:'b',name:'B 组',kind:'SIMULATED SEGMENT B',x:5,z:155,w:65,d:65,h:32,desc:'固定脚本分配的 B 组，与 A 组共同覆盖全部演示样本，不表示实验效果。',rules:[['预设占比','50%'],['触达渠道','按脚本分配'],['生产分桶','未调用']]},
 {id:'sms',name:'短信',kind:'SMS CHANNEL',x:200,z:-245,w:60,d:56,h:65,desc:'模拟短信通道的发送与回执。等待阶段仍为处理中，到终点才记成功或失败。',rules:[['预设分配','25%'],['服务连接','无'],['回执来源','预设脚本']]},
 {id:'push',name:'Push',kind:'PUSH CHANNEL',x:200,z:-80,w:60,d:56,h:65,desc:'模拟 App 推送通道。画面中的成功仅指回执成功，不代表用户已阅读。',rules:[['预设分配','25%'],['设备数据','无'],['回执来源','预设脚本']]},
 {id:'whatsapp',name:'WhatsApp',kind:'WHATSAPP CHANNEL',x:200,z:80,w:60,d:56,h:65,desc:'模拟跨境消息通道。在回执延迟场景下停留更久，等待中的用户不会被提前判成失败。',rules:[['预设分配','25%'],['服务连接','无'],['等待时长','随场景变化']]},
 {id:'ivr',name:'IVR',kind:'VOICE CHANNEL',x:200,z:245,w:60,d:56,h:65,desc:'模拟语音触达流程。不拨打电话，不使用号码，也不连接呼叫服务。',rules:[['预设分配','25%'],['真实拨号','无'],['回执来源','预设脚本']]},
 {id:'success',name:'触达成功',kind:'SUCCESS TERMINAL',x:410,z:-135,w:92,d:84,h:34,desc:'已抵达终点并获得模拟成功回执的虚拟用户。它不是阅读率、下单率或业务转化。',rules:[['状态','终态'],['口径','模拟回执成功'],['业务转化','未模拟']]},
 {id:'failed',name:'触达失败',kind:'FAILURE TERMINAL',x:410,z:145,w:92,d:84,h:24,desc:'预设脚本产生的终态失败。延迟回执与尚未进入的人不计入这里；不自动补发。',rules:[['状态','终态'],['失败原因','模拟渠道拒绝'],['真实重试','无']]},
];
const byId = Object.fromEntries(nodes.map(n => [n.id,n]));
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
 if(phase==='entry')return pathPosition(route('audience','strategy'),progress);
 if(phase==='split')return pathPosition(route('strategy',p.group),progress);
 if(phase==='dispatch')return pathPosition(route(p.group,p.channel),progress);
 if(phase==='waiting'){const n=byId[p.channel];return [n.x-40-(p.id%4)*3,17,n.z-22+Math.floor(p.id/4)%12*4];}
 if(phase==='result')return pathPosition(route(p.channel,p.failed?'failed':'success'),progress);
 const n=byId[phase];return [n.x+58+(Math.floor(p.id/4)%8)*4,17,n.z-30+(p.id%16)*4];
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
 } else if(n.id==='strategy') {
  svg+=cube(n.x-33,n.z,15,48,63,15)+cube(n.x+33,n.z,15,48,63,15)+cube(n.x,n.z,81,48,11,78,true);
  svg+=cube(n.x+33,n.z+3,18,25,24,15,true);
 } else if(n.id==='a'||n.id==='b'){
  svg+=cube(n.x,n.z,43,43,20,15)+cube(n.x,n.z,43,43,3,35,true);
  const [x,y]=project(n.x,46,n.z);svg+=`<text x="${x}" y="${y}" text-anchor="middle" font-family="Georgia,serif" font-size="24" fill="${colors.blue}">${n.id.toUpperCase()}</text>`;
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
 const connections=[['audience','strategy'],['strategy','a'],['strategy','b'],...['a','b'].flatMap(g=>CHANNELS.map(c=>[g,c])),...CHANNELS.flatMap(c=>[[c,'success'],[c,'failed']])];
 $('routes').innerHTML=connections.map(([a,b])=>line(route(a,b),colors.white,9)+line(route(a,b),b==='failed'?colors.ink:colors.blue,1.25,b==='failed'?'4 5':'')).join('');
 const figures = reduced ? [] : data.states.map(personFigure);
 // Painter ordering uses camera depth for solids and people.
 $('objects').innerHTML=[...nodes.map(building),...figures].sort((a,b)=>a.z-b.z).map(o=>o.svg).join('');
 $('labels').innerHTML=nodes.map(n=>{
  const [x,y]=CHANNELS.includes(n.id)?project(n.x-84,24,n.z):project(n.x,0,n.z+n.d/2+22), width=n.id==='whatsapp'?100:86;
  const count=data.nodes[n.id].arrived;
  return `<g class="scene-node ${selected===n.id?'selected-label':''}" data-node="${n.id}" transform="translate(${x.toFixed(1)},${y.toFixed(1)})"><rect class="node-label-back node-outline" x="${-width/2}" y="-2" width="${width}" height="38" rx="3"/><text class="scene-label" text-anchor="middle" y="13">${n.name}</text><text class="scene-sub" text-anchor="middle" y="28">${count} ${n.id==='audience'?'样本':'累计'}</text></g>`;
 }).join('');
}
function renderDetails(){
 const n=byId[selected],s=data.nodes[selected];
 $('detail-index').textContent=String(nodes.indexOf(n)+1).padStart(2,'0');
 $('detail-icon').innerHTML=icon(n.id);$('detail-kind').textContent=n.kind;$('detail-title').textContent=n.name;$('detail-desc').textContent=n.desc;
 $('arrived-label').textContent=n.id==='audience'?'固定样本':'累计到达';$('detail-arrived').textContent=s.arrived;$('detail-active').textContent=s.active;
 $('detail-progress').style.width=(s.arrived/240*100)+'%';
 $('detail-rules').innerHTML=n.rules.map(([a,b])=>`<div><span>${a}</span><strong>${b}</strong></div>`).join('')+(CHANNELS.includes(n.id)?`<div><span>终态成功 / 失败</span><strong>${s.success} / ${s.failed}</strong></div>`:'');
}
const timeFormat = t => `${String(Math.floor(t/60)).padStart(2,'0')}:${(t%60).toFixed(1).padStart(4,'0')}`;
let lastTime=-1,lastPlaying=false;
function render(time,playing){
 if(playing===lastPlaying&&playing&&Math.abs(time-lastTime)<.04)return;
 lastTime=time;lastPlaying=playing;data=snapshot(scenario,time);
 for(const k of ['total','queued','active','success','failed'])$(k).textContent=data[k];
 $('balance').textContent=`${data.queued+data.active+data.success+data.failed} / ${data.total}`;
 $('time').textContent=timeFormat(time);$('timeline').value=time;
 $('play').textContent=playing?'暂停模拟':time>=scenario.duration?'重新播放':time===0?'开始模拟':'继续模拟';
 $('play-state').textContent=playing?'模拟运行中':time>=scenario.duration?'模拟结束':'已暂停';
 for(const n of nodes)document.querySelector(`[data-list-node="${n.id}"] .count`).textContent=data.nodes[n.id].arrived;
 renderDetails();drawScene();
}
const clock=createClock({duration:scenario.duration,render,request:cb=>requestAnimationFrame(cb),cancel:id=>cancelAnimationFrame(id)});
$('node-list').innerHTML=nodes.map(n=>`<button class="node-button ${n.id===selected?'active':''}" data-node="${n.id}" data-list-node="${n.id}" aria-pressed="${n.id===selected}">${icon(n.id)}<span>${n.name}</span><span class="count">0</span></button>`).join('');
function selectNode(id){
 if(!byId[id])return;selected=id;
 document.querySelectorAll('[data-list-node]').forEach(b=>{const on=b.dataset.node===id;b.classList.toggle('active',on);b.setAttribute('aria-pressed',on);});
 renderDetails();drawScene();$('status-message').textContent=`已选择${byId[id].name}，详情已更新。`;
}
$('node-list').addEventListener('click',e=>{const b=e.target.closest('[data-node]');if(b)selectNode(b.dataset.node);});
$('play').addEventListener('click',()=>{if(clock.playing)clock.pause();else{if(clock.time>=scenario.duration)clock.reset();clock.play();}$('status-message').textContent=clock.playing?'模拟已开始，仅使用构造数据。':'模拟已暂停。';});
$('reset').addEventListener('click',()=>{clock.reset();$('status-message').textContent='模拟已重置并暂停。';});
$('speed').addEventListener('change',e=>clock.setSpeed(Number(e.target.value)));
$('timeline').addEventListener('input',e=>clock.seek(e.target.value));
$('scenario').addEventListener('change',e=>{
 clock.pause();scenario=createScenario(e.target.value);$('timeline').max=scenario.duration;$('duration').textContent=timeFormat(scenario.duration);$('scenario-note').textContent=scenario.config.note;clock.reset(scenario.duration);$('status-message').textContent=`已切换${scenario.config.name}，模拟已重置并暂停。`;
});
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
$('timeline').max=scenario.duration;$('duration').textContent=timeFormat(scenario.duration);render(0,false);updateMotion();
