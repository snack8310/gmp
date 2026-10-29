export const CAMPAIGN_API_BASE = 'http://127.0.0.1:8081';

export function browserCampaignApiBase(location, override) {
 return override || (/^https?:$/.test(location.protocol) ? location.origin : '');
}

export class CampaignApi {
 constructor(base = CAMPAIGN_API_BASE, request = globalThis.fetch.bind(globalThis)) {
  this.base = base.replace(/\/$/, '');
  this.request = request;
 }

 async call(method, path, body) {
  const started = Date.now();
  let status = 0;
  try {
   if (!this.base) throw new Error('活动管理 API 不可用：离线单文件没有同源 HTTP 服务。');
   let response;
   try {
    response = await this.request(`${this.base}/api/v1${path}`, { method, ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }) });
   } catch {
    throw new Error('无法连接活动管理 API，请启动本机服务后重试。');
   }
   status = response.status;
   if (response.status === 204) return undefined;
   let payload;
   try { payload = await response.json(); } catch { throw new Error(`活动管理 API 返回了无效 JSON（HTTP ${status}）。`); }
   if (!response.ok) {
    const error = new Error(payload?.error?.message || `活动管理 API 请求失败（HTTP ${status}）。`);
    error.status = status;
    error.code = payload?.error?.code;
    throw error;
   }
   return payload;
  } finally {
   globalThis.console?.info?.(JSON.stringify({ component: 'campaign-api-client', method, status, duration_ms: Date.now() - started }));
  }
 }

 list() { return this.call('GET', '/campaigns'); }
 create(value) { return this.call('POST', '/campaigns', value); }
 get(id) { return this.call('GET', `/campaigns/${encodeURIComponent(id)}`); }
 update(id, value) { return this.call('PUT', `/campaigns/${encodeURIComponent(id)}`, value); }
 delete(id) { return this.call('DELETE', `/campaigns/${encodeURIComponent(id)}`); }
 createTreatment(id, value) { return this.call('POST', `/campaigns/${encodeURIComponent(id)}/treatments`, value); }
 updateTreatment(id, treatmentId, value) { return this.call('PUT', `/campaigns/${encodeURIComponent(id)}/treatments/${encodeURIComponent(treatmentId)}`, value); }
 deleteTreatment(id, treatmentId) { return this.call('DELETE', `/campaigns/${encodeURIComponent(id)}/treatments/${encodeURIComponent(treatmentId)}`); }
 actions() { return this.call('GET', '/actions'); }
 audienceReferences() { return this.call('GET', '/audience-references'); }
 usages(ref) {
  const query = new URLSearchParams(ref.type === 'arm' ? { type: 'arm', bundleId: ref.bundleId, groupId: ref.groupId } : { type: 'group', groupId: ref.groupId });
  return this.call('GET', `/audience-reference-usages?${query}`);
 }
}
