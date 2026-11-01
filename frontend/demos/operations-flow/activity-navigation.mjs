import { escapeHTML } from './experiment-ui.mjs';

const activityLink = id => '#activities/' + encodeURIComponent(id) + '/audience';

export function renderActivityNavigation(campaigns, selectedId = null) {
 if (!campaigns.length) return '<p class="activity-nav-empty">暂无活动</p>';
 return campaigns.map(activity => {
  const current = activity.id === selectedId;
  return '<a class="activity-nav-link' + (current ? ' active' : '') + '" href="' + escapeHTML(activityLink(activity.id)) + '"' +
   (current ? ' aria-current="location"' : '') + '>' + escapeHTML(activity.name) + '</a>';
 }).join('');
}

export function mountActivityNavigation({ api }) {
 const el = id => document.getElementById(id);
 const root = el('activity-list');
 let serial = 0, activities = [], selected = null;

 async function refresh(selectedId = null) {
  const request = ++serial;
  root.innerHTML = '<p class="activity-nav-loading">正在读取活动列表…</p>';
  try {
   const result = await api.list();
   if (request !== serial) return { stale: true, activities, selected };
   activities = result.campaigns;
   selected = activities.find(item => item.id === selectedId) || null;
   root.innerHTML = renderActivityNavigation(activities, selectedId);
   el('activity-nav-status').textContent = '';
   el('activity-context-name').textContent = selected?.name || '';
   el('activity-context-id').textContent = selected ? '活动 ID · ' + selected.id : '';
   return { activities, selected, error: null };
  } catch (error) {
   if (request !== serial) return { stale: true, activities, selected };
   activities = [];
   selected = null;
   root.innerHTML = '<p class="activity-nav-empty">暂时无法显示活动列表。</p>';
   el('activity-nav-status').textContent = '无法读取活动管理 API：' + error.message;
   el('activity-context-name').textContent = '';
   el('activity-context-id').textContent = '';
   return { activities, selected, error };
  }
 }

 return { refresh, current: () => selected, activities: () => activities };
}
