export const ACTIVITY_DOMAINS = Object.freeze(['campaign']);

const decode = value => {
 try { return { value: decodeURIComponent(value), valid: true }; }
 catch { return { value: '', valid: false }; }
};

function validCampaignSuffix(parts) {
 if (!parts.length) return true;
 if (parts.length === 1 && parts[0] === 'edit') return true;
 if (parts.length === 2 && parts[0] === 'treatments' && parts[1] === 'new') return true;
 if (parts.length === 3 && parts[0] === 'treatments' && parts[2] === 'edit') {
  const treatment = decode(parts[1]);
  return treatment.valid && Boolean(treatment.value);
 }
 return false;
}

function activityWorkspace(activitySegment, domain = 'campaign', suffix = []) {
 const decoded = decode(activitySegment);
 if (!decoded.valid || !decoded.value) return { screen: 'invalid', activityId: null, domain: null, campaignRoute: '' };
 if (domain === 'compose') domain = 'campaign';
 if (!ACTIVITY_DOMAINS.includes(domain)) return { screen: 'invalid', activityId: decoded.value, domain: null, campaignRoute: '' };
 if (domain === 'campaign' && !validCampaignSuffix(suffix)) return { screen: 'invalid', activityId: decoded.value, domain: null, campaignRoute: '' };
 const campaignRoute = 'campaign/activities/' + activitySegment + (domain === 'campaign' && suffix.length ? '/' + suffix.join('/') : '');
 return { screen: 'workspace', activityId: decoded.value, domain, campaignRoute };
}

function returnRoute(value) {
 if (!value || value.includes('?') || value.includes('#')) return null;
 const parts = value.split('/');
 if (parts.length !== 3 || parts[0] !== 'activities') return null;
 const workspace = activityWorkspace(parts[1], parts[2]);
 if (workspace.screen !== 'workspace') return null;
 return 'activities/' + encodeURIComponent(workspace.activityId) + '/' + workspace.domain;
}

function parseAssetRoute(module, search = '') {
 const params = new URLSearchParams(search);
 return { screen: 'global', module, returnTo: returnRoute(params.get('return')), activityId: null, domain: null, campaignRoute: '' };
}

export function parseActivityRoute(hash = '') {
 const input = String(hash).replace(/^#/, '');
 const queryIndex = input.indexOf('?');
 const rawRoute = queryIndex < 0 ? input : input.slice(0, queryIndex);
 const search = queryIndex < 0 ? '' : input.slice(queryIndex + 1);
 const route = rawRoute.replace(/^\/+|\/+$/g, '');
 if (!route) return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };

 if (route === 'audience' || route === 'experimentation') return parseAssetRoute(route, search);

 const parts = route.split('/');
 if (parts[0] === 'activities') {
  if (parts.length === 1) return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
  if (parts.length === 2 && parts[1] === 'new') return { screen: 'new', activityId: null, domain: null, campaignRoute: 'campaign/activities/new' };
  if (parts.length === 2) return activityWorkspace(parts[1]);
  if (parts.length === 3 && ['audience', 'experimentation'].includes(parts[2])) {
   const decoded = decode(parts[1]);
   if (!decoded.valid || !decoded.value) return { screen: 'invalid', activityId: null, domain: null, campaignRoute: '' };
   return { ...parseAssetRoute(parts[2]), returnTo: 'activities/' + encodeURIComponent(decoded.value) + '/campaign' };
  }
  const domain = parts[2];
  if (domain === 'campaign') return activityWorkspace(parts[1], domain, parts.slice(3));
  if (parts.length === 3) return activityWorkspace(parts[1], domain);
  return { screen: 'invalid', activityId: decode(parts[1]).value, domain: null, campaignRoute: '' };
 }

 if (route === 'campaign/activities') return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
 if (route === 'campaign') return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
 if (route === 'compose') return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
 if (parts[0] === 'campaign' && parts[1] === 'activities') {
  if (parts.length === 3 && parts[2] === 'new') return { screen: 'new', activityId: null, domain: null, campaignRoute: 'campaign/activities/new' };
  if (parts.length >= 3) {
   const decoded = decode(parts[2]);
   if (!decoded.valid || !decoded.value) return { screen: 'invalid', activityId: null, domain: null, campaignRoute: '' };
   return activityWorkspace(parts[2], 'campaign', parts.slice(3));
  }
 }
 return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
}

export function activityRoute(activityId, domain = 'campaign') {
 if (typeof activityId !== 'string' || !activityId || !ACTIVITY_DOMAINS.includes(domain)) throw new TypeError('Activity ID and domain are required');
 return 'activities/' + encodeURIComponent(activityId) + '/' + domain;
}

export function assetRoute(module, returnTo = null) {
 if (!['audience', 'experimentation'].includes(module)) throw new TypeError('Asset module is required');
 const canonicalReturn = returnTo ? returnRoute(returnTo) : null;
 return module + (canonicalReturn ? '?return=' + encodeURIComponent(canonicalReturn) : '');
}
