import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

const [serverPort = '8081', debugPort = '9222', screenshotDir] = process.argv.slice(2);
const runId = randomUUID();
const desktopName = `浏览器验收活动 ${runId}`;
const mobileName = `窄屏验收活动 ${runId}`;
const tabs = await (await fetch(`http://127.0.0.1:${debugPort}/json`)).json();
const page = tabs.find(tab => tab.url.startsWith(`http://127.0.0.1:${serverPort}/`));
assert.ok(page, 'demo tab missing');
const socket = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
let serial = 0;
const pending = new Map();
const networkRequests = [];
const flowWriteRequests = [];
const audienceWriteRequests = [];
let rejectNextFlowWrite = false;
socket.onmessage = event => {
 const message = JSON.parse(event.data);
 if (message.method === 'Network.requestWillBeSent') {
  networkRequests.push(message.params.request.url);
  if (message.params.request.method === 'PUT' && new URL(message.params.request.url).pathname.endsWith('/flow')) flowWriteRequests.push(message.params.request.url);
  if (message.params.request.method === 'PUT' && new URL(message.params.request.url).pathname.endsWith('/audience')) audienceWriteRequests.push(message.params.request.url);
 }
 if (message.method === 'Fetch.requestPaused') {
  const path = new URL(message.params.request.url).pathname;
  const fail = rejectNextFlowWrite && message.params.request.method === 'PUT' && path.endsWith('/flow');
  if (fail) rejectNextFlowWrite = false;
  const response = fail
   ? command('Fetch.fulfillRequest', { requestId: message.params.requestId, responseCode: 422, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body: Buffer.from(JSON.stringify({ error: { code: 'invalid_flow', message: '浏览器验收模拟的写入拒绝' } })).toString('base64') })
   : command('Fetch.continueRequest', { requestId: message.params.requestId });
  response.catch(error => console.error(error));
 }
 if (pending.has(message.id)) { const {resolve, reject} = pending.get(message.id); pending.delete(message.id); message.error ? reject(Error(message.error.message)) : resolve(message.result); }
};
function command(method, params = {}) { const id = ++serial; return new Promise((resolve, reject) => { pending.set(id, {resolve, reject}); socket.send(JSON.stringify({id, method, params})); }); }
async function evaluate(expression) { const result = await command('Runtime.evaluate', {expression, returnByValue:true, awaitPromise:true}); if (result.exceptionDetails) throw Error(`${result.exceptionDetails.text}: ${result.exceptionDetails.exception?.description}; expression: ${expression.slice(0, 200)}`); return result.result.value; }
async function until(expression) {
 const result = await evaluate(`new Promise(resolve => { let tries=0; const timer=setInterval(() => { if (${expression}) { clearInterval(timer); resolve(true); } else if (++tries > 250) { clearInterval(timer); resolve(false); } }, 20); })`);
 assert.equal(result, true, `timed out: ${expression}`);
}
const log = (step, details = {}) => console.log(JSON.stringify({ step, ...details, result: 'ok' }));
async function checkDetail(id, step) {
 const state = await evaluate(`(async () => {
  const response = await fetch(${JSON.stringify(`/api/v1/campaigns/${encodeURIComponent(id)}`)});
  const activity = await response.json();
  const detail = document.querySelector('#campaign-detail');
  return { status: response.status, activity, heading: detail?.querySelector('h2')?.textContent,
   text: detail?.textContent, rows: [...detail.querySelectorAll('section[aria-label="投放项清单"] tbody tr')].map(row => row.textContent) };
 })()`);
 assert.equal(state.status, 200);
 assert.equal(state.activity.id, id);
 assert.equal(state.heading, state.activity.name);
 assert.ok(state.text.includes(`活动 ID · ${id}`));
 assert.deepEqual(state.activity.audience, { type: 'group', groupId: 'group-recall' });
 assert.ok(state.activity.treatments.every(item => !Object.hasOwn(item, 'audienceRef')));
 assert.equal(state.rows.length, state.activity.treatments.length);
 for (const treatment of state.activity.treatments) {
  const row = state.rows.find(text => text.includes(treatment.id));
  assert.ok(row, `treatment ${treatment.id} missing`);
 }
 log(step, { apiStatus: state.status, activityId: id, name: state.activity.name,
  audience: state.activity.audience, treatmentCount: state.activity.treatments.length });
}
async function mobileCheck(step, selector) {
 const state = await evaluate(`(() => {
  const element = document.querySelector(${JSON.stringify(selector)});
  element?.scrollIntoView({block:'center'});
  const rect = element?.getBoundingClientRect();
  return { visible: Boolean(rect && rect.width > 0 && rect.height > 0 && rect.bottom > 0 && rect.top < innerHeight),
   viewport: innerWidth, scrollWidth: document.documentElement.scrollWidth };
 })()`);
 assert.equal(state.visible, true, `${step}: ${selector} not visible`);
 assert.ok(state.scrollWidth <= state.viewport + 1, `${step}: horizontal overflow ${state.scrollWidth} > ${state.viewport}`);
 log(step, state);
}
let desktopId;
let mobileId;
let mobileViewport = false;
try {
 await until("document.querySelector('#audience-catalog table')");
 await evaluate("location.hash='#activities'");
 await until("document.querySelector('#activity-list a[href=\"#activities/campaign-branch/campaign\"]')");
 await evaluate("location.hash='#activities/new'");
 await until("document.querySelector('#campaign-form')");
 await evaluate(`document.querySelector('#campaign-name').value=${JSON.stringify(desktopName)}; document.querySelector('#campaign-audience').value='group:group-recall'; document.querySelector('#campaign-form').requestSubmit()`);
 await until(`location.hash.startsWith('#activities/campaign-') && document.querySelector('#campaign-detail').textContent.includes(${JSON.stringify(desktopName)})`);
 desktopId = await evaluate("location.hash.slice('#activities/'.length).split('/')[0]");
 log('create_activity', { activityId: desktopId });

 await evaluate("document.querySelector('[data-delete-audience=\"aud-mx-dormant\"]').click()");
 await until("document.querySelector('#audience-status').textContent.includes('不能删除')");
 log('audience_delete_blocked_by_browser_usage_read');
 await evaluate(`location.hash=${JSON.stringify(`#activities/${desktopId}/campaign/treatments/new`)}`);
 await until("document.querySelector('#treatment-form')");
 await evaluate("document.querySelector('#treatment-name').value='浏览器短信'; document.querySelector('#treatment-action').value='action-sms'; document.querySelector('#treatment-form').requestSubmit()");
 await until("document.querySelector('#campaign-detail').textContent.includes('浏览器短信')");
 log('add_treatment');

 await evaluate("location.hash='#activities'");
 await until(`document.querySelector(${JSON.stringify(`#campaign-activities a[href*="${desktopId}"]`)})`);
 await evaluate(`document.querySelector(${JSON.stringify(`#campaign-activities a[href="#activities/${desktopId}/campaign"]`)}).click()`);
 await until(`location.hash===${JSON.stringify(`#activities/${desktopId}/campaign`)} && document.querySelector('#campaign-detail h2')`);
 await evaluate(`document.querySelector(${JSON.stringify(`#campaign-detail a[href="#activities/${desktopId}/campaign/edit"]`)}).click()`);
 await until("document.querySelector('#campaign-form')");
 const renamed = await evaluate(`(async () => {
  const response = await fetch(${JSON.stringify(`/api/v1/campaigns/${encodeURIComponent(desktopId)}`)});
  const activity = await response.json();
  const update = await fetch(${JSON.stringify(`/api/v1/campaigns/${encodeURIComponent(desktopId)}`)}, {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({...activity, name:'历史栈 API 最新名称'})});
  return update.status;
 })()`);
 assert.equal(renamed, 200);
 log('history_api_rename', { expected: 200, actual: renamed });
 await evaluate('history.back()');
 await until(`location.hash===${JSON.stringify(`#activities/${desktopId}/campaign`)} && document.querySelector('#campaign-detail h2')?.textContent==='历史栈 API 最新名称'`);
 await checkDetail(desktopId, 'history_back_detail');
 await evaluate('history.forward()');
 await until(`location.hash===${JSON.stringify(`#activities/${desktopId}/campaign/edit`)} && document.querySelector('#campaign-form')`);
 log('history_forward_edit', { activityId: desktopId });
 await evaluate('history.back()');
 await until(`location.hash===${JSON.stringify(`#activities/${desktopId}/campaign`)} && document.querySelector('#campaign-detail h2')?.textContent==='历史栈 API 最新名称'`);
 await checkDetail(desktopId, 'history_back_again');

 await evaluate("location.hash += '/edit'");
 await until("document.querySelector('#campaign-form')");
 await evaluate("document.querySelector('#campaign-name').value='浏览器验收已改名'; document.querySelector('#campaign-form').requestSubmit()");
 await until("document.querySelector('#campaign-detail').textContent.includes('浏览器验收已改名')");
 await evaluate("window.confirm=()=>true; document.querySelector('[data-delete-treatment]').click()");
 await until("document.querySelector('#campaign-detail').textContent.includes('暂无投放项')");
 await evaluate("document.querySelector('[data-delete-activity]').click()");
 await until("location.hash==='#activities' && !document.querySelector('#campaign-activities').textContent.includes('浏览器验收已改名')");
 desktopId = undefined;
 log('edit_and_delete');

 await command('Page.reload', { ignoreCache: true });
 await until("document.querySelector('#activity-list a[href=\"#activities/campaign-branch/campaign\"]')");
 assert.equal(await evaluate("document.querySelector('#campaign-activities').textContent.includes('浏览器验收已改名')"), false);
 log('reload_without_deleted_activity');

 await evaluate("location.hash='#activities/campaign-branch/campaign'");
 await until("document.querySelector('#activity-flow-canvas [data-world]') && document.querySelector('#campaign-detail h2')?.textContent==='跨境召回'");
 const flowDetail = await evaluate(`(() => ({
  activity: document.querySelector('#campaign-detail h2')?.textContent,
  nodes: [...new Set([...document.querySelectorAll('#activity-flow-canvas [data-flow-node]')].map(node => node.dataset.flowNode))],
  edges: [...document.querySelectorAll('#activity-flow-canvas [data-flow-edge]')].map(edge => edge.dataset.flowEdge),
  warning: document.querySelector('.flow-rehearsal-warning')?.textContent,
 }))()`);
 assert.equal(flowDetail.activity, '跨境召回');
 assert.ok(flowDetail.nodes.includes('treatment-rcs') && flowDetail.nodes.includes('treatment-email') && flowDetail.nodes.includes('treatment-sms'));
 assert.deepEqual(flowDetail.edges.sort(), ['edge-failed', 'edge-success']);
 assert.match(flowDetail.warning, /不会触达真实用户或真实渠道/);
 log('activity_flow_deep_link', { activity: flowDetail.activity, nodeIds: flowDetail.nodes, edgeIds: flowDetail.edges });
 if (screenshotDir) {
  await mkdir(screenshotDir, { recursive: true });
  await command('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1200, deviceScaleFactor: 1, mobile: false });
  await evaluate("document.querySelector('#activity-flow-canvas').scrollIntoView({block:'center'})");
  const screenshot = await command('Page.captureScreenshot', { format: 'png' });
  const output = join(screenshotDir, `activity-flow-${runId}.png`);
  await writeFile(output, Buffer.from(screenshot.data, 'base64'));
  await command('Emulation.clearDeviceMetricsOverride');
  log('activity_flow_screenshot', { output });
 }

 await evaluate("location.hash='#activities/campaign-empty/campaign'");
 await until("document.querySelector('#activity-flow-canvas [data-node-total]')?.textContent==='0'");
 const emptyFlow = await evaluate(`(() => ({
  activity: document.querySelector('#campaign-detail h2')?.textContent,
  nodes: document.querySelectorAll('#activity-flow-canvas [data-flow-node]').length,
  playDisabled: document.querySelector('#activity-flow-canvas [data-flow-play]').disabled,
 }))()`);
 assert.equal(emptyFlow.activity, '待装配活动');
 assert.equal(emptyFlow.nodes, 0);
 assert.equal(emptyFlow.playDisabled, true);
 log('activity_flow_empty_activity', emptyFlow);
 await evaluate("location.hash='#audience?return=activities/campaign-empty/campaign'");
 await until("document.querySelector('#new-audience') && document.querySelector('#audience-browse')");
 await evaluate("document.querySelector('#new-audience').click(); document.querySelector('#audience-name').value='画布入口验收人群'; document.querySelector('#audience-form').requestSubmit();");
 await until("document.querySelector('#audience-status')?.textContent.includes('人群定义已保存')");
 const newGroupId = await evaluate(`(() => {
  const card = [...document.querySelectorAll('#audience-list article')].find(item => item.textContent.includes('画布入口验收人群'));
  const localId = card?.querySelector('small')?.textContent.trim().split(/\\s+/)[0];
  return localId ? 'group-' + localId : '';
 })()`);
 assert.match(newGroupId, /^group-aud-custom-/);
 const synchronizedReference = await evaluate(`(async () => {
  const response = await fetch('/api/v1/audience-references');
  const { audienceReferences } = await response.json();
  return audienceReferences.find(item => item.groupId === ${JSON.stringify(newGroupId)});
 })()`);
 assert.equal(synchronizedReference.name, '画布入口验收人群');
 await evaluate("document.querySelector('#asset-return-link').click()");
 await until("location.hash==='#activities/campaign-empty/campaign' && document.querySelector('#activity-flow-canvas [data-flow-config-entry]')");
 await command('Network.enable');
 await evaluate("location.hash='#activities/campaign-empty/campaign/edit'");
 await until("document.querySelector('#campaign-form')");
 const audienceWriteStart = audienceWriteRequests.length;
 await evaluate(`document.querySelector('#campaign-audience').value=${JSON.stringify(`group:${newGroupId}`)}; document.querySelector('#campaign-form').requestSubmit()`);
 await until(`location.hash==='#activities/campaign-empty/campaign' && document.querySelector('#activity-flow-canvas [data-flow-config-entry]')`);
 const audienceWrites = audienceWriteRequests.length - audienceWriteStart;
 assert.equal(audienceWrites, 1);
 const selectedAudience = await evaluate(`(async () => {
  const response = await fetch('/api/v1/campaigns/campaign-empty');
  const campaign = await response.json();
  return { audience: campaign.audience, entries: campaign.activityFlow?.entries || [], nodes: document.querySelectorAll('#activity-flow-canvas [data-flow-node]').length,
   addTreatment: document.querySelector('#activity-flow-canvas a[href*="/treatments/new"]')?.textContent };
 })()`);
 assert.deepEqual(selectedAudience.audience, { type: 'group', groupId: newGroupId });
 assert.deepEqual(selectedAudience.entries, []);
 assert.ok(selectedAudience.nodes > 0 && selectedAudience.addTreatment.includes('添加投放项'));
 log('activity_flow_audience_selection', { ...selectedAudience, serializedWrites: audienceWrites });
 const staleAudienceWrite = await evaluate(`(async () => {
  const response=await fetch('/api/v1/campaigns/campaign-empty/audience',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({expectedAudience:null,audience:null})});
  const body=await response.json(); const current=await (await fetch('/api/v1/campaigns/campaign-empty')).json();
  return { status:response.status, code:body.error?.code, audience:current.audience };
 })()`);
 assert.equal(staleAudienceWrite.status, 409);
 assert.equal(staleAudienceWrite.code, 'audience_conflict');
 assert.deepEqual(staleAudienceWrite.audience, { type: 'group', groupId: newGroupId });
 log('stale_activity_audience_write_rejected', staleAudienceWrite);
 await evaluate(`document.querySelector("#activity-flow-canvas a[href*='/treatments/new']").click()`);
 await until("document.querySelector('#treatment-form')");
 await evaluate(`document.querySelector('#treatment-name').value='画布短信投放项'; document.querySelector('#treatment-action').value='action-sms'; document.querySelector('#treatment-form').requestSubmit();`);
 await until("document.querySelector('#campaign-detail').textContent.includes('画布短信投放项')");
 const newTreatmentId = await evaluate("(async () => (await (await fetch('/api/v1/campaigns/campaign-empty')).json()).treatments[0].id)()");
 await evaluate("document.querySelector('#activity-flow-canvas [data-flow-config-entry]').click()");
 await until("document.querySelector('#activity-entry-form select[data-entry-group]')");
 await evaluate(`(() => { const form=document.querySelector('#activity-entry-form'); const select=form.querySelector('[data-entry-group]'); select.value=${JSON.stringify(newTreatmentId)}; form.requestSubmit(); })()`);
 await until(`(async () => { const activity=await (await fetch('/api/v1/campaigns/campaign-empty')).json(); return activity.activityFlow?.entries?.some(entry=>entry.groupId===${JSON.stringify(newGroupId)} && entry.treatmentIds.includes(${JSON.stringify(newTreatmentId)})); })()`);
 const configuredActivity = await evaluate("(async () => (await (await fetch('/api/v1/campaigns/campaign-empty')).json()) )()");
 assert.deepEqual(configuredActivity.activityFlow.entries, [{ groupId: newGroupId, kind: 'start', treatmentIds: [newTreatmentId] }]);
 assert.equal(configuredActivity.treatments.every(item => !Object.hasOwn(item, 'audienceRef')), true);
 log('activity_flow_entry_routed', { audience: configuredActivity.audience, entry: configuredActivity.activityFlow.entries[0] });
 await evaluate("location.hash='#activities/campaign-empty/campaign/edit'");
 await until("document.querySelector('#campaign-form')");
 await evaluate("document.querySelector('#campaign-audience').value='group:group-new'; document.querySelector('#campaign-form').requestSubmit()");
 await until("location.hash==='#activities/campaign-empty/campaign' && document.querySelector('#activity-flow-canvas [data-flow-config-entry]')");
 const resetActivity = await evaluate("(async () => (await (await fetch('/api/v1/campaigns/campaign-empty')).json()) )()");
 assert.deepEqual(resetActivity.audience, { type: 'group', groupId: 'group-new' });
 assert.deepEqual(resetActivity.activityFlow.entries, []);
 assert.equal(await evaluate("document.querySelectorAll('#activity-flow-canvas [data-flow-node]').length > 0"), true);
 log('audience_change_clears_only_incompatible_entries', { audience: resetActivity.audience, entries: resetActivity.activityFlow.entries, edges: resetActivity.activityFlow.edges });

 await evaluate("location.hash='#activities/campaign-branch/campaign'");
 await until("document.querySelector('#activity-flow-canvas [data-flow-play]') && document.querySelector('#campaign-detail h2')?.textContent==='跨境召回'");
 await command('Page.reload', { ignoreCache: true });
 await until("document.querySelector('#activity-flow-canvas [data-world]') && document.querySelector('#campaign-detail h2')?.textContent==='跨境召回'");
 const refreshedFlow = await evaluate("document.querySelectorAll('#activity-flow-canvas [data-flow-edge]').length");
 assert.equal(refreshedFlow, 2);
 log('activity_flow_refresh_binding', { activity: '跨境召回', receiptEdges: refreshedFlow });

 await evaluate("document.querySelector('#activity-flow-canvas [data-flow-add-edge]').click()");
 await evaluate("(() => { const form=document.querySelector('#activity-flow-form'); form.elements.fromTreatmentId.value='treatment-email'; form.elements.fromTreatmentId.dispatchEvent(new Event('change',{bubbles:true})); form.elements.toTreatmentId.value='treatment-sms'; form.elements.conditionLabel.value='成功'; form.requestSubmit(); })()");
 const addedEdge = await evaluate("(async () => { const campaign=await (await fetch('/api/v1/campaigns/campaign-branch')).json(); return campaign.activityFlow.edges.find(edge=>edge.fromTreatmentId==='treatment-email' && edge.toTreatmentId==='treatment-sms' && edge.conditionLabel==='成功'); })()");
 assert.ok(addedEdge?.id);
 await until(`document.querySelector('#activity-flow-canvas [data-flow-edge="${addedEdge.id}"]')`);
 await evaluate(`document.querySelector('#activity-flow-canvas [data-flow-edge="${addedEdge.id}"]').dispatchEvent(new MouseEvent('click',{bubbles:true}))`);
 await until(`document.querySelector('#activity-flow-canvas [data-delete-edge="${addedEdge.id}"]')`);
 await evaluate(`document.querySelector('#activity-flow-canvas [data-delete-edge="${addedEdge.id}"]').click()`);
 await until("document.querySelectorAll('#activity-flow-canvas [data-flow-edge]').length===2");
 const afterDeleteEdges = await evaluate("(async () => (await (await fetch('/api/v1/campaigns/campaign-branch')).json()).activityFlow.edges.length)()");
 assert.equal(afterDeleteEdges, 2);
 log('activity_flow_edge_create_delete', { created: addedEdge, remaining: afterDeleteEdges });

 await evaluate("document.querySelector('[data-flow-edge=\"edge-success\"]').dispatchEvent(new MouseEvent('click',{bubbles:true}))");
 await until("document.querySelector('#activity-flow-canvas [data-edit-edge=\"edge-success\"]')");
 await evaluate("document.querySelector('#activity-flow-canvas [data-edit-edge=\"edge-success\"]').click()");
 await until("document.querySelector('#activity-flow-form') && !document.querySelector('#activity-flow-form').closest('.flow-edge-form').hidden");
 const existingEdge = await evaluate("(() => { const form=document.querySelector('#activity-flow-form'); return { from:form.elements.fromTreatmentId.value, to:form.elements.toTreatmentId.value, condition:form.elements.conditionLabel.value }; })()");
 assert.deepEqual(existingEdge, { from: 'treatment-rcs', to: 'treatment-email', condition: '成功' });
 await command('Network.enable');
 const failedWriteStart = flowWriteRequests.length;
 await command('Fetch.enable', { patterns: [{ urlPattern: `http://127.0.0.1:${serverPort}/api/v1/campaigns/campaign-branch/flow`, requestStage: 'Request' }] });
 rejectNextFlowWrite = true;
 await evaluate("document.querySelector('#activity-flow-form').dispatchEvent(new SubmitEvent('submit',{bubbles:true,cancelable:true}))");
 await until("document.querySelector('#activity-flow-canvas .flow-save-status')?.textContent.includes('保存失败')");
 const failedFormState = await evaluate("(() => { const form=document.querySelector('#activity-flow-form'); return { busy:form.hasAttribute('aria-busy'), submitDisabled:form.querySelector('[type=submit]').disabled, selectsEnabled:[...form.querySelectorAll('select')].every(item=>!item.disabled) }; })()");
 assert.deepEqual(failedFormState, { busy: false, submitDisabled: false, selectsEnabled: true });
 assert.equal(flowWriteRequests.length - failedWriteStart, 1);
 await command('Fetch.disable');
 log('activity_flow_edge_edit_failure_unlock', failedFormState);
 const edgeWriteStart = flowWriteRequests.length;
 await evaluate("(() => { const form=document.querySelector('#activity-flow-form'); form.elements.fromTreatmentId.value='treatment-email'; form.elements.fromTreatmentId.dispatchEvent(new Event('change',{bubbles:true})); form.elements.toTreatmentId.value='treatment-sms'; form.elements.conditionLabel.value='失败'; const submit=new SubmitEvent('submit',{bubbles:true,cancelable:true}); form.dispatchEvent(submit); form.dispatchEvent(new SubmitEvent('submit',{bubbles:true,cancelable:true})); })()");
 await until("document.querySelector('#activity-flow-canvas [data-flow-edge=\"edge-success\"]')?.textContent==='失败'");
 const concurrentFlowWrites = flowWriteRequests.length - edgeWriteStart;
 assert.equal(concurrentFlowWrites, 1);
 const updatedEdge = await evaluate("(async () => { const campaign=await (await fetch('/api/v1/campaigns/campaign-branch')).json(); return campaign.activityFlow.edges.find(edge=>edge.id==='edge-success'); })()");
 assert.deepEqual(updatedEdge, { id: 'edge-success', fromTreatmentId: 'treatment-email', toTreatmentId: 'treatment-sms', conditionLabel: '失败' });
 await until("document.querySelector('#activity-flow-canvas [data-flow-edge=\"edge-success\"]')?.textContent==='失败'");
 const seedEdges = [
  { id: 'edge-success', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-email', conditionLabel: '成功' },
  { id: 'edge-failed', fromTreatmentId: 'treatment-rcs', toTreatmentId: 'treatment-sms', conditionLabel: '失败' },
 ];
 const seedEntries = [{ groupId: 'group-recall', kind: 'start', treatmentIds: ['treatment-rcs'] }];
 const staleFlowWrite = await evaluate(`(async () => {
  const expectedEntries=${JSON.stringify(seedEntries)};
  const expectedEdges=${JSON.stringify(seedEdges)};
  const response=await fetch('/api/v1/campaigns/campaign-branch/flow',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:'flow-recall',expectedEntries,expectedEdges,entries:expectedEntries,edges:expectedEdges})});
  const body=await response.json(); const current=await (await fetch('/api/v1/campaigns/campaign-branch')).json();
  return { status:response.status, code:body.error?.code, edges:current.activityFlow.edges };
 })()`);
 assert.equal(staleFlowWrite.status, 409);
 assert.equal(staleFlowWrite.code, 'flow_conflict');
 assert.deepEqual(staleFlowWrite.edges.find(edge => edge.id === 'edge-success'), updatedEdge);
 log('stale_activity_flow_write_rejected', { status: staleFlowWrite.status, code: staleFlowWrite.code });
 log('activity_flow_edge_edit', { ...updatedEdge, concurrentSubmits: 2, serializedWrites: concurrentFlowWrites });

 await command('Network.enable');
 const flowNetworkStart = networkRequests.length;
 await evaluate("(() => { const speed=document.querySelector('#activity-flow-canvas [data-flow-speed]'); speed.value='4'; speed.dispatchEvent(new Event('change',{bubbles:true})); document.querySelector('#activity-flow-canvas [data-flow-play]').click(); })()");
 await until("document.querySelector('#activity-flow-canvas [data-play-state]')?.textContent==='本地演练中' && Number(document.querySelector('#activity-flow-canvas [data-flow-time]')?.textContent.split(':').at(-1))>0.2");
 const runningFlow = await evaluate(`(() => ({
  state: document.querySelector('#activity-flow-canvas [data-flow-state]')?.textContent,
  figures: document.querySelectorAll('#activity-flow-canvas [data-objects] ellipse').length,
  time: document.querySelector('#activity-flow-canvas [data-flow-time]')?.textContent,
 }))()`);
 assert.equal(runningFlow.state, '演练中');
 assert.ok(runningFlow.figures > 0);
 await evaluate("document.querySelector('#activity-flow-canvas [data-flow-play]').click()");
 await until("document.querySelector('#activity-flow-canvas [data-flow-state]')?.textContent==='已暂停'");
 const pausedTime = await evaluate("document.querySelector('#activity-flow-canvas [data-flow-time]')?.textContent");
 await new Promise(resolve => setTimeout(resolve, 120));
 assert.equal(await evaluate("document.querySelector('#activity-flow-canvas [data-flow-time]')?.textContent"), pausedTime);
 await evaluate("document.querySelector('#activity-flow-canvas [data-flow-reset]').click()");
 await until("document.querySelector('#activity-flow-canvas [data-flow-time]')?.textContent==='00:00.0'");
 const resetFlow = await evaluate(`(() => ({
  state: document.querySelector('#activity-flow-canvas [data-flow-state]')?.textContent,
  queued: document.querySelector('#activity-flow-canvas [data-flow-queued]')?.textContent,
  complete: document.querySelector('#activity-flow-canvas [data-flow-complete]')?.textContent,
 }))()`);
 assert.deepEqual(resetFlow, { state: '已暂停', queued: String(runningFlow.figures), complete: '0' });
 assert.deepEqual(networkRequests.slice(flowNetworkStart), []);
 log('activity_flow_play_pause_reset', { running: runningFlow, pausedAt: pausedTime, reset: resetFlow, networkRequestsDuringPlayback: 0 });

 await command('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 3, mobile: true });
 mobileViewport = true;
 await evaluate("location.hash='#activities'");
 await until("document.querySelector('#campaign-activities .primary-link')");
 await mobileCheck('mobile_list', '#campaign-activities .primary-link');
 await evaluate("location.hash='#activities/campaign-branch/campaign'");
 await until("document.querySelector('#activity-flow-canvas [data-world]')");
 await mobileCheck('mobile_activity_flow', '#activity-flow-canvas [data-world]');
 await evaluate("location.hash='#activities'");
 await until("document.querySelector('#campaign-activities .primary-link')");
 await evaluate("document.querySelector('#campaign-activities .primary-link').click()");
 await until("document.querySelector('#campaign-form')");
 await mobileCheck('mobile_new_form', '#campaign-form');
 await evaluate(`document.querySelector('#campaign-name').value=${JSON.stringify(mobileName)}; document.querySelector('#campaign-audience').value='group:group-new'; document.querySelector('#campaign-form').requestSubmit()`);
 await until(`location.hash.startsWith('#activities/campaign-') && document.querySelector('#campaign-detail').textContent.includes(${JSON.stringify(mobileName)})`);
 mobileId = await evaluate("location.hash.slice('#activities/'.length).split('/')[0]");
 await mobileCheck('mobile_created_detail', '#campaign-detail');
 await evaluate(`document.querySelector(${JSON.stringify(`#campaign-detail a[href="#activities/${mobileId}/campaign/treatments/new"]`)}).click()`);
 await until("document.querySelector('#treatment-form')");
 await mobileCheck('mobile_treatment_form', '#treatment-form');
 await evaluate("document.querySelector('#treatment-name').value='窄屏短信'; document.querySelector('#treatment-action').value='action-sms'; document.querySelector('#treatment-form').requestSubmit()");
 await until("document.querySelector('#campaign-detail').textContent.includes('窄屏短信')");
 await mobileCheck('mobile_added_treatment', '#campaign-detail section[aria-label="投放项清单"]');
 if (screenshotDir) {
  await mkdir(screenshotDir, { recursive: true });
  const screenshot = await command('Page.captureScreenshot', { format: 'png' });
  const output = join(screenshotDir, `campaign-mobile-${runId}.png`);
  await writeFile(output, Buffer.from(screenshot.data, 'base64'));
  log('mobile_screenshot', { output });
 }
 await evaluate("window.confirm=()=>true; document.querySelector('[data-delete-activity]').click()");
 await until(`location.hash==='#activities' && !document.querySelector('#campaign-activities').textContent.includes(${JSON.stringify(mobileName)})`);
 mobileId = undefined;
 await mobileCheck('mobile_deleted_campaign', '#campaign-activities .primary-link');
} finally {
 try {
  const response = await fetch(`http://127.0.0.1:${serverPort}/api/v1/campaigns`);
  assert.equal(response.status, 200);
  const list = await response.json();
  for (const activity of list.campaigns.filter(item => [desktopId, mobileId].includes(item.id) || [desktopName, mobileName].includes(item.name))) {
   const deleted = await fetch(`http://127.0.0.1:${serverPort}/api/v1/campaigns/${encodeURIComponent(activity.id)}`, { method: 'DELETE' });
   console.log(JSON.stringify({ step: 'cleanup_campaign', activityId: activity.id, expected: 204, actual: deleted.status }));
   assert.equal(deleted.status, 204);
  }
 } finally {
  try { if (mobileViewport) await command('Emulation.clearDeviceMetricsOverride'); }
  finally { socket.close(); }
 }
}
