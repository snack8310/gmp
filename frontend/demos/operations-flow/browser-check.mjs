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
socket.onmessage = event => { const message = JSON.parse(event.data); if (pending.has(message.id)) { const {resolve, reject} = pending.get(message.id); pending.delete(message.id); message.error ? reject(Error(message.error.message)) : resolve(message.result); } };
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
 assert.ok(state.activity.treatments.some(item => item.audienceRef.groupId === 'group-recall'));
 assert.equal(state.rows.length, state.activity.treatments.length);
 for (const treatment of state.activity.treatments) {
  const row = state.rows.find(text => text.includes(treatment.id));
  assert.ok(row, `treatment ${treatment.id} missing`);
  assert.ok(row.includes(treatment.audienceRef.groupId));
  if (treatment.audienceRef.bundleId) assert.ok(row.includes(treatment.audienceRef.bundleId));
 }
 log(step, { apiStatus: state.status, activityId: id, name: state.activity.name,
  references: state.activity.treatments.map(item => item.audienceRef) });
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
 await evaluate("location.hash='#campaign/activities'");
 await until("document.querySelector('#campaign-activities').textContent.includes('campaign-branch')");
 await evaluate("location.hash='#campaign/activities/new'");
 await until("document.querySelector('#campaign-form')");
 await evaluate(`document.querySelector('#campaign-name').value=${JSON.stringify(desktopName)}; document.querySelector('#campaign-form').requestSubmit()`);
 await until(`location.hash.startsWith('#campaign/activities/campaign-') && document.querySelector('#campaign-detail').textContent.includes(${JSON.stringify(desktopName)})`);
 desktopId = await evaluate("location.hash.slice('#campaign/activities/'.length)");
 log('create_activity', { activityId: desktopId });

 await evaluate("document.querySelector('[data-delete-audience=\"aud-mx-dormant\"]').click()");
 await until("document.querySelector('#audience-status').textContent.includes('不能删除')");
 log('audience_delete_blocked_by_browser_usage_read');
 await evaluate(`location.hash=${JSON.stringify(`#campaign/activities/${desktopId}/treatments/new`)}`);
 await until("document.querySelector('#treatment-form')");
 await evaluate("document.querySelector('#treatment-name').value='浏览器短信'; document.querySelector('#treatment-action').value='action-sms'; document.querySelector('#treatment-audience').value='group:group-new'; document.querySelector('#treatment-form').requestSubmit()");
 await until("document.querySelector('#campaign-detail').textContent.includes('浏览器短信')");
 log('add_treatment');

 await evaluate("location.hash='#campaign/activities'");
 await until(`document.querySelector(${JSON.stringify(`#campaign-activities a[href*="${desktopId}"]`)})`);
 await evaluate(`document.querySelector(${JSON.stringify(`#campaign-activities a[href="#campaign/activities/${desktopId}"]`)}).click()`);
 await until(`location.hash===${JSON.stringify(`#campaign/activities/${desktopId}`)} && document.querySelector('#campaign-detail h2')`);
 await evaluate(`document.querySelector(${JSON.stringify(`#campaign-detail a[href="#campaign/activities/${desktopId}/edit"]`)}).click()`);
 await until("document.querySelector('#campaign-form')");
 const renamed = await evaluate(`(async () => {
  const response = await fetch(${JSON.stringify(`/api/v1/campaigns/${encodeURIComponent(desktopId)}`)});
  const activity = await response.json();
  const update = await fetch(${JSON.stringify(`/api/v1/campaigns/${encodeURIComponent(desktopId)}`)}, {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({...activity, name:'历史栈 API 最新名称', treatments:activity.treatments.map(item => ({...item, audienceRef:{type:'group', groupId:'group-recall'}}))})});
  return update.status;
 })()`);
 assert.equal(renamed, 200);
 log('history_api_rename', { expected: 200, actual: renamed });
 await evaluate('history.back()');
 await until(`location.hash===${JSON.stringify(`#campaign/activities/${desktopId}`)} && document.querySelector('#campaign-detail h2')?.textContent==='历史栈 API 最新名称'`);
 await checkDetail(desktopId, 'history_back_detail');
 await evaluate('history.forward()');
 await until(`location.hash===${JSON.stringify(`#campaign/activities/${desktopId}/edit`)} && document.querySelector('#campaign-form')`);
 log('history_forward_edit', { activityId: desktopId });
 await evaluate('history.back()');
 await until(`location.hash===${JSON.stringify(`#campaign/activities/${desktopId}`)} && document.querySelector('#campaign-detail h2')?.textContent==='历史栈 API 最新名称'`);
 await checkDetail(desktopId, 'history_back_again');

 await evaluate("location.hash += '/edit'");
 await until("document.querySelector('#campaign-form')");
 await evaluate("document.querySelector('#campaign-name').value='浏览器验收已改名'; document.querySelector('#campaign-form').requestSubmit()");
 await until("document.querySelector('#campaign-detail').textContent.includes('浏览器验收已改名')");
 await evaluate("window.confirm=()=>true; document.querySelector('[data-delete-treatment]').click()");
 await until("document.querySelector('#campaign-detail').textContent.includes('暂无投放项')");
 await evaluate("document.querySelector('[data-delete-activity]').click()");
 await until("location.hash==='#campaign/activities' && !document.querySelector('#campaign-activities').textContent.includes('浏览器验收已改名')");
 desktopId = undefined;
 log('edit_and_delete');

 await command('Page.reload', { ignoreCache: true });
 await until("document.querySelector('#campaign-activities').textContent.includes('campaign-branch')");
 assert.equal(await evaluate("document.querySelector('#campaign-activities').textContent.includes('浏览器验收已改名')"), false);
 log('reload_without_deleted_activity');

 await command('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 3, mobile: true });
 mobileViewport = true;
 await evaluate("location.hash='#campaign/activities'");
 await until("document.querySelector('#campaign-activities .primary-link')");
 await mobileCheck('mobile_list', '#campaign-activities .primary-link');
 await evaluate("document.querySelector('#campaign-activities .primary-link').click()");
 await until("document.querySelector('#campaign-form')");
 await mobileCheck('mobile_new_form', '#campaign-form');
 await evaluate(`document.querySelector('#campaign-name').value=${JSON.stringify(mobileName)}; document.querySelector('#campaign-form').requestSubmit()`);
 await until(`location.hash.startsWith('#campaign/activities/campaign-') && document.querySelector('#campaign-detail').textContent.includes(${JSON.stringify(mobileName)})`);
 mobileId = await evaluate("location.hash.slice('#campaign/activities/'.length)");
 await mobileCheck('mobile_created_detail', '#campaign-detail');
 await evaluate(`document.querySelector(${JSON.stringify(`#campaign-detail a[href="#campaign/activities/${mobileId}/treatments/new"]`)}).click()`);
 await until("document.querySelector('#treatment-form')");
 await mobileCheck('mobile_treatment_form', '#treatment-form');
 await evaluate("document.querySelector('#treatment-name').value='窄屏短信'; document.querySelector('#treatment-action').value='action-sms'; document.querySelector('#treatment-audience').value='group:group-new'; document.querySelector('#treatment-form').requestSubmit()");
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
 await until(`location.hash==='#campaign/activities' && !document.querySelector('#campaign-activities').textContent.includes(${JSON.stringify(mobileName)})`);
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
