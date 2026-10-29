import { escapeHTML } from './experiment-ui.mjs';
import { BROWSE_ACTIVITIES, DEFAULT_AUDIENCE_CATALOG, REFERENCE_CATALOG, actionAvailable, resolveAudience } from './campaign-activities.mjs';

const safe = value => escapeHTML(String(value ?? ''));
const unavailable = '<span class="campaign-unavailable">引用不可用 · 构造引用</span>';
export const referenceKey = ref => ref?.type === 'arm' ? `arm:${ref.bundleId}:${ref.groupId}` : `group:${ref?.groupId}`;

export function renderActivitiesList(activities = BROWSE_ACTIVITIES, editable = false) {
 const heading = `<div class="section-heading"><div><p class="eyebrow">CAMPAIGN / ACTIVITIES</p><h2>活动列表</h2><p>${editable ? '来自活动管理 API；只管理演示数据，不执行或触达。' : '仅浏览本地构造活动，不会执行或触达。'}</p></div>${editable ? '<a class="primary-link" href="#campaign/activities/new">＋ 新建活动</a>' : ''}</div>`;
 if (!activities.length) return `${heading}<p class="campaign-empty">暂无演示活动</p>`;
 const rows = activities.map(activity => `<tr><th scope="row">${safe(activity.name)} <small>· ${safe(activity.id)}</small></th><td>${activity.treatments.length}</td><td><a href="#campaign/activities/${safe(encodeURIComponent(activity.id))}">查看详情</a></td></tr>`).join('');
 return `${heading}<div class="campaign-table-scroll" tabindex="0" aria-label="活动列表表格，可横向滚动"><table class="campaign-table"><thead><tr><th scope="col">活动名称</th><th scope="col">投放项数量</th><th scope="col">操作</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function audienceLabel(ref, catalog, serverRefs = null) {
 if (!ref) return unavailable;
 const resolved = resolveAudience(ref, catalog) || catalog.find(item => referenceKey(item) === referenceKey(ref));
 const server = serverRefs?.find(item => referenceKey(item) === referenceKey(ref));
 const available = Boolean(resolved?.available) && (serverRefs === null || Boolean(server?.available));
 const label = ref.type === 'arm' ? `${safe(resolved?.bundleName || ref.bundleName || server?.bundleName || ref.bundleId)} / ${safe(resolved?.armName || ref.armName || server?.name || ref.groupId)}` : safe(resolved?.groupName || ref.groupName || server?.name || ref.groupId);
 const kind = ref.type === 'arm' ? '实验人群包臂' : '普通人群组';
 const ids = ref.type === 'arm' ? `${safe(ref.bundleId)} / ${safe(ref.groupId)}` : safe(ref.groupId);
 return `<span class="campaign-ref-kind">${kind}</span> ${label} <small>· ${ids}</small>${available ? '' : ` ${unavailable}`}`;
}

function renderTreatments(treatments, catalog, editable = false, actions = REFERENCE_CATALOG.actions, serverRefs = null, activityId = '') {
 if (!treatments.length) return '<p class="campaign-empty">暂无投放项</p>';
 const rows = treatments.map(treatment => { const action = actions.find(item => item.id === treatment.actionId); const valid = treatment.actionRef ? actionAvailable(treatment.actionRef) : Boolean(action?.available); return `<tr><th scope="row">${safe(treatment.name)} <small>· ${safe(treatment.id)}</small></th><td>${safe(treatment.actionRef?.name || action?.name || treatment.actionId)} <small>· ${safe(treatment.actionRef?.id || treatment.actionId)}</small>${valid ? '' : ` ${unavailable}`}</td><td>${audienceLabel(treatment.audienceRef, catalog, serverRefs)}</td><td>${treatment.slotId ? safe(treatment.slotId) : '—'}</td>${editable ? `<td><a href="#campaign/activities/${safe(encodeURIComponent(activityId))}/treatments/${safe(encodeURIComponent(treatment.id))}/edit">编辑</a> <button type="button" data-delete-treatment="${safe(treatment.id)}">删除</button></td>` : ''}</tr>`; }).join('');
 return `<div class="campaign-table-scroll" tabindex="0" aria-label="投放项表格，可横向滚动"><table class="campaign-table"><thead><tr><th scope="col">投放项</th><th scope="col">动作</th><th scope="col">引用人群</th><th scope="col">展示位</th>${editable ? '<th scope="col">操作</th>' : ''}</tr></thead><tbody>${rows}</tbody></table></div>`;
}

function treatmentLabel(id, treatments) {
 const treatment = treatments.find(item => item.id === id);
 return `${safe(treatment?.name)} <small>· ${safe(id)}</small>`;
}

function renderFlow(activity) {
 const flow = activity.activityFlow;
 if (!flow) return '<p class="campaign-empty">未配置活动流程</p>';
 if (!flow.edges.length) return '<p class="campaign-empty">暂无活动流程边</p><p class="campaign-note">同一入口且无边相连的投放项可并行；本页不计算入口，也不按列表顺序推断执行。</p>';
 const rows = flow.edges.map(edge => `<tr><td>${treatmentLabel(edge.fromTreatmentId, activity.treatments)} <span aria-hidden="true">→</span> ${treatmentLabel(edge.toTreatmentId, activity.treatments)}</td><td>${safe(edge.conditionLabel)}</td></tr>`).join('');
 return `<p class="campaign-note">活动流程 · ${safe(flow.id)}。结果含义来自演示动作说明；边只展示回执关系，不在此求值或播放。</p><div class="campaign-table-scroll" tabindex="0" aria-label="活动流程关系表，可横向滚动"><table class="campaign-table"><thead><tr><th scope="col">起点 → 终点</th><th scope="col">回执条件</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

export function renderActivityDetail(id, activities = BROWSE_ACTIVITIES, catalog = DEFAULT_AUDIENCE_CATALOG, editable = false, actions = REFERENCE_CATALOG.actions, serverRefs = null) {
 const back = '<a class="campaign-back" href="#campaign/activities">← 返回活动列表</a>';
 const activity = activities.find(item => item.id === id);
 if (!activity) return `${back}<div class="section-heading"><div><h2>未找到该演示活动</h2></div></div>`;
 return `${back}<div class="section-heading"><div><p class="eyebrow">CAMPAIGN / ACTIVITY DETAIL</p><h2>${safe(activity.name)}</h2><p>活动 ID · ${safe(activity.id)} · ${editable ? '管理 API 当前状态' : '仅本地构造数据'}</p></div>${editable ? `<div class="campaign-actions"><a href="#campaign/activities/${safe(encodeURIComponent(id))}/edit">编辑活动</a><button type="button" data-delete-activity="${safe(id)}">删除活动</button></div>` : ''}</div><section class="campaign-detail-block" aria-label="投放项清单"><h3>投放项清单</h3>${editable ? `<a href="#campaign/activities/${safe(encodeURIComponent(id))}/treatments/new">＋ 添加投放项</a>` : ''}${renderTreatments(activity.treatments, catalog, editable, actions, serverRefs, id)}<p class="campaign-note">运营引用受众侧统一目录中的人群组或实验人群包臂；分流定义归实验侧。可用性随本浏览器的目录变化。</p></section><section class="campaign-detail-block" aria-label="活动流程"><h3>活动流程</h3>${renderFlow(activity)}</section>`;
}

export function validateTreatmentDraft(value, actions, references, others) {
 if (!value.name?.trim()) return '投放项名称必填。';
 const action = actions.find(item => item.id === value.actionId && item.available);
 if (!value.actionId) return '每项必须选择一个动作。';
 if (!action) return '动作引用不可用，请修正后保存。';
 if (!value.audienceRef) return '每项必须选择一个人群。';
 if (!references.some(item => referenceKey(item) === referenceKey(value.audienceRef) && item.available)) return '人群引用不可用，请修正后保存。';
 if (action.requiresSlot && !value.slotId) return '展示动作必须选择展示位。';
 if (!action.requiresSlot && value.slotId) return '推送动作不得带展示位。';
 if (value.slotId && others.some(item => item.slotId === value.slotId && actions.find(action => action.id === item.actionId)?.requiresSlot && !(item.audienceRef?.type === 'arm' && value.audienceRef.type === 'arm' && item.audienceRef.bundleId === value.audienceRef.bundleId && item.audienceRef.groupId !== value.audienceRef.groupId))) return '同一展示位的人群无法证明互斥。';
 return '';
}
