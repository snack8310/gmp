import assert from 'node:assert/strict';
const [serverPort = '8081', debugPort = '9222'] = process.argv.slice(2);
const tabs = await (await fetch(`http://127.0.0.1:${debugPort}/json`)).json();
const page = tabs.find(tab => tab.url.startsWith(`http://127.0.0.1:${serverPort}/`));
assert.ok(page, 'demo tab missing');
const socket = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
let serial = 0;
const pending = new Map();
socket.onmessage = event => { const message = JSON.parse(event.data); if (pending.has(message.id)) { const {resolve, reject} = pending.get(message.id); pending.delete(message.id); message.error ? reject(Error(message.error.message)) : resolve(message.result); } };
function command(method, params = {}) { const id = ++serial; return new Promise((resolve, reject) => { pending.set(id, {resolve, reject}); socket.send(JSON.stringify({id, method, params})); }); }
async function evaluate(expression) { const result = await command('Runtime.evaluate', {expression, returnByValue:true, awaitPromise:true}); if (result.exceptionDetails) throw Error(result.exceptionDetails.text); return result.result.value; }
async function until(expression) {
 const result = await evaluate(`new Promise(resolve => { let tries=0; const timer=setInterval(() => { if (${expression}) { clearInterval(timer); resolve(true); } else if (++tries > 250) { clearInterval(timer); resolve(false); } }, 20); })`);
 assert.equal(result, true, `timed out: ${expression}`);
}

await until("document.querySelector('#audience-catalog table')");
await evaluate("location.hash='#campaign/activities'");
await until("document.querySelector('#campaign-activities').textContent.includes('campaign-branch')");
await evaluate("location.hash='#campaign/activities/new'");
await until("document.querySelector('#campaign-form')");
await evaluate("document.querySelector('#campaign-name').value='浏览器验收活动'; document.querySelector('#campaign-form').requestSubmit()");
await until("location.hash.startsWith('#campaign/activities/campaign-') && document.querySelector('#campaign-detail').textContent.includes('浏览器验收活动')");
const created = await evaluate("location.hash.slice('#campaign/activities/'.length)");
console.log(JSON.stringify({ step: 'create_activity', result: 'ok' }));

await evaluate("document.querySelector('[data-delete-audience=\"aud-mx-dormant\"]').click()");
await until("document.querySelector('#audience-status').textContent.includes('不能删除')");
await evaluate(`location.hash='#campaign/activities/${created}/treatments/new'`);
await until("document.querySelector('#treatment-form')");
await evaluate("document.querySelector('#treatment-name').value='浏览器短信'; document.querySelector('#treatment-action').value='action-sms'; document.querySelector('#treatment-audience').value='group:group-new'; document.querySelector('#treatment-form').requestSubmit()");
await until("document.querySelector('#campaign-detail').textContent.includes('浏览器短信')");
console.log(JSON.stringify({ step: 'add_treatment', result: 'ok' }));

await evaluate("location.hash += '/edit'");
await until("document.querySelector('#campaign-form')");
await evaluate("document.querySelector('#campaign-name').value='浏览器验收已改名'; document.querySelector('#campaign-form').requestSubmit()");
await until("document.querySelector('#campaign-detail').textContent.includes('浏览器验收已改名')");
await evaluate("window.confirm=()=>true; document.querySelector('[data-delete-treatment]').click()");
await until("document.querySelector('#campaign-detail').textContent.includes('暂无投放项')");
await evaluate("document.querySelector('[data-delete-activity]').click()");
await until("location.hash==='#campaign/activities' && !document.querySelector('#campaign-activities').textContent.includes('浏览器验收已改名')");
console.log(JSON.stringify({ step: 'edit_and_delete', result: 'ok' }));

await command('Page.reload', { ignoreCache: true });
await until("document.querySelector('#campaign-activities').textContent.includes('campaign-branch')");
assert.equal(await evaluate("document.querySelector('#campaign-activities').textContent.includes('浏览器验收已改名')"), false);
console.log(JSON.stringify({ step: 'reload_without_deleted_activity', result: 'ok' }));
socket.close();
