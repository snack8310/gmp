import { mountAudiences } from './audience-ui.mjs';
import { mountExperiments } from './experiment-ui.mjs';
import { CampaignApi, browserCampaignApiBase } from './campaign-api-client.mjs';
import { mountCampaigns } from './campaign-controller.mjs';
import { mountActivityNavigation } from './activity-navigation.mjs';
import { activityRoute, assetRoute, parseActivityRoute } from './activity-routing.mjs';
import { groupIdOf, STORAGE_KEY } from './audience.mjs';
import { createFlowExperimentStore, FLOW_EXPERIMENT_KEY } from './flow-experiment-draft.mjs';

const $ = id => document.getElementById(id);
const assetDomains = { audience: 'audience-section', experimentation: 'experimentation-section' };
let renderSerial = 0;
let activityNavigation;
let campaignUI;
let audienceUI;
let experimentUI;

async function renderDomain() {
 const version = ++renderSerial;
 const route = parseActivityRoute(window.location.hash);
 const landing = $('activity-landing');
 const notFound = $('activity-not-found');
 const workspace = $('lab-shell');
 landing.hidden = route.screen !== 'list';
 notFound.hidden = true;
 workspace.hidden = true;
 $('activity-context').hidden = true;
 $('shared-asset-note').hidden = true;
 $('activity-route-loading').hidden = route.screen === 'list' || route.screen === 'global';
 $('asset-return').hidden = true;
 workspace.classList.toggle('global-asset-view', route.screen === 'global' || route.screen === 'new');
 $('activity-menu-list').hidden = route.screen === 'global';
 $('campaign-section').hidden = true;
 for (const section of Object.values(assetDomains)) $(section).hidden = true;
 for (const key of ['activities', 'audience', 'experimentation']) {
  $(`module-${key}`).setAttribute('aria-current', route.screen === 'global'
   ? route.module === key ? 'page' : 'false'
   : key === 'activities' ? 'page' : 'false');
 }
 campaignUI.clear();

 if (route.screen === 'global') {
  workspace.hidden = false;
  $(assetDomains[route.module]).hidden = false;
  if (route.module === 'experimentation') experimentUI.render(route);
  $('experiment-audience-link').href = `#${assetRoute('audience', route.returnTo)}`;
  if (route.returnTo) {
   $('asset-return').hidden = false;
   $('asset-return-link').href = `#${route.returnTo}`;
  }
  return;
 }

 await audienceUI.ready;
 if (version !== renderSerial) return;
 const activityResult = await activityNavigation.refresh(route.activityId);
 if (version !== renderSerial) return;
 $('activity-route-loading').hidden = true;

 if (route.screen === 'list') {
  campaignUI.render('campaign/activities');
  return;
 }
 if (route.screen === 'new') {
  workspace.hidden = false;
  $('campaign-section').hidden = false;
  campaignUI.render('campaign/activities/new');
  return;
 }
 if (route.screen === 'invalid' || activityResult.error || !activityResult.selected) {
  notFound.hidden = false;
  $('activity-not-found-title').textContent = activityResult.error ? '无法读取活动上下文' : '未找到该活动';
  $('activity-not-found-message').textContent = activityResult.error
   ? activityResult.error.message
   : `活动 ID「${route.activityId || ''}」不在当前管理 API 活动列表中。`;
  return;
 }

 const selected = activityResult.selected;
 workspace.hidden = false;
 $('activity-context').hidden = false;
 $('campaign-section').hidden = false;
 $('experiment-audience-link').href = `#${assetRoute('audience', activityRoute(selected.id))}`;
 await campaignUI.render(route.campaignRoute);
}

function navigate(route) {
 const current = parseActivityRoute(window.location.hash);
 const returnTo = current.screen === 'workspace' ? activityRoute(current.activityId) : current.returnTo;
 const next = ['audience', 'experimentation'].includes(route)
  ? assetRoute(route, returnTo)
  : route === 'campaign' ? current.activityId ? activityRoute(current.activityId) : 'activities' : route;
 window.location.hash = `#${next}`;
 renderDomain();
}

window.addEventListener('hashchange', renderDomain);
window.addEventListener('storage', event => { if (event.key === FLOW_EXPERIMENT_KEY || event.key === STORAGE_KEY) renderDomain(); });
const campaignApi = new CampaignApi(browserCampaignApiBase(window.location, window.CAMPAIGN_API_BASE));
let flowStorage;
try { flowStorage = window.localStorage; } catch { flowStorage = null; }
const flowDraftStore = createFlowExperimentStore(flowStorage);
activityNavigation = mountActivityNavigation({ api: campaignApi });
audienceUI = mountAudiences({ api: campaignApi,
 onLoaded: audiences => campaignApi.base ? Promise.all(audiences.map(item => campaignApi.upsertGroupAudienceReference(groupIdOf(item.id), item.name))) : undefined,
 onSaved: item => campaignApi.base ? campaignApi.upsertGroupAudienceReference(groupIdOf(item.id), item.name) : undefined,
 onDeleted: id => campaignApi.base ? campaignApi.deleteGroupAudienceReference(groupIdOf(id)) : undefined, onNavigate: navigate });
campaignUI = mountCampaigns({ api: campaignApi, audienceStore: audienceUI.store, flowDraftStore, onNavigate: navigate,
 onChanged: () => audienceUI.refreshProtection() });
experimentUI = mountExperiments({ store: audienceUI.store, flowDraftStore, onChanged: () => audienceUI.refreshExperiments() });
renderDomain();
