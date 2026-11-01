export const ACTIVITY_DOMAINS = Object.freeze(['audience', 'experimentation', 'campaign', 'compose']);

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

function workspace(activitySegment, domain = 'audience', suffix = []) {
 const decoded = decode(activitySegment);
 if (!decoded.valid || !decoded.value) return { screen: 'invalid', activityId: null, domain: null, campaignRoute: '' };
 if (!ACTIVITY_DOMAINS.includes(domain)) return { screen: 'invalid', activityId: decoded.value, domain: null, campaignRoute: '' };
 if (domain === 'campaign' && !validCampaignSuffix(suffix)) return { screen: 'invalid', activityId: decoded.value, domain: null, campaignRoute: '' };
 const campaignRoute = 'campaign/activities/' + activitySegment + (domain === 'campaign' && suffix.length ? '/' + suffix.join('/') : '');
 return { screen: 'workspace', activityId: decoded.value, domain, campaignRoute };
}

export function parseActivityRoute(hash = '') {
 const route = String(hash).replace(/^#/, '').replace(/^\/+|\/+$/g, '');
 if (!route) return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };

 const parts = route.split('/');
 if (parts[0] === 'activities') {
  if (parts.length === 1) return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
  if (parts.length === 2 && parts[1] === 'new') return { screen: 'new', activityId: null, domain: null, campaignRoute: 'campaign/activities/new' };
  if (parts.length === 2) return workspace(parts[1]);
  const domain = parts[2];
  if (domain === 'campaign') return workspace(parts[1], domain, parts.slice(3));
  if (parts.length === 3) return workspace(parts[1], domain);
  return { screen: 'invalid', activityId: decode(parts[1]).value, domain: null, campaignRoute: '' };
 }

 if (route === 'campaign' || route === 'campaign/activities' || ACTIVITY_DOMAINS.includes(route)) {
  return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
 }
 if (parts[0] === 'campaign' && parts[1] === 'activities') {
  if (parts.length === 3 && parts[2] === 'new') return { screen: 'new', activityId: null, domain: null, campaignRoute: 'campaign/activities/new' };
  if (parts.length >= 3) {
   const decoded = decode(parts[2]);
   if (!decoded.valid || !decoded.value) return { screen: 'invalid', activityId: null, domain: null, campaignRoute: '' };
   return workspace(parts[2], 'campaign', parts.slice(3));
  }
 }
 return { screen: 'list', activityId: null, domain: null, campaignRoute: '' };
}

export function activityRoute(activityId, domain = 'audience') {
 if (typeof activityId !== 'string' || !activityId || !ACTIVITY_DOMAINS.includes(domain)) throw new TypeError('Activity ID and domain are required');
 return 'activities/' + encodeURIComponent(activityId) + '/' + domain;
}
