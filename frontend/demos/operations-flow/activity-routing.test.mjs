import test from 'node:test';
import assert from 'node:assert/strict';
import { activityRoute, assetRoute, parseActivityRoute } from './activity-routing.mjs';

test('activity, audience and experimentation are independent top-level routes', () => {
 assert.equal(parseActivityRoute('#activities').screen, 'list');
 assert.deepEqual(parseActivityRoute('#audience'), {
  screen: 'global', module: 'audience', returnTo: null, activityId: null, domain: null, campaignRoute: '',
 });
 assert.deepEqual(parseActivityRoute('#experimentation'), {
  screen: 'global', module: 'experimentation', returnTo: null, activityId: null, domain: null, campaignRoute: '',
 });
});

test('activity opens its content directly and old composition routes fall back to the activity', () => {
 assert.deepEqual(parseActivityRoute('#activities/campaign-branch'), {
  screen: 'workspace', activityId: 'campaign-branch', domain: 'campaign', campaignRoute: 'campaign/activities/campaign-branch',
 });
 assert.deepEqual(parseActivityRoute('#activities/campaign-branch/compose'), {
  screen: 'workspace', activityId: 'campaign-branch', domain: 'campaign', campaignRoute: 'campaign/activities/campaign-branch',
 });
 assert.equal(activityRoute('campaign-branch'), 'activities/campaign-branch/campaign');
 assert.throws(() => activityRoute('campaign-branch', 'compose'), TypeError);
});

test('asset routes preserve a valid activity return path and reject other return targets', () => {
 const target = activityRoute('campaign alpha/β', 'campaign');
 const route = assetRoute('audience', target);
 assert.equal(route, 'audience?return=activities%2Fcampaign%2520alpha%252F%25CE%25B2%2Fcampaign');
 assert.deepEqual(parseActivityRoute('#' + route), {
  screen: 'global', module: 'audience', returnTo: target, activityId: null, domain: null, campaignRoute: '',
 });
 assert.equal(parseActivityRoute('#audience?return=activities%2Fbad%2Funknown').returnTo, null);
 assert.equal(assetRoute('audience', 'activities/campaign-branch/compose'), 'audience?return=activities%2Fcampaign-branch%2Fcampaign');
 assert.equal(assetRoute('experimentation', 'audience'), 'experimentation');
 assert.throws(() => assetRoute('activities'), TypeError);
});

test('activity and nested operation IDs are encoded and decoded by path segment', () => {
 const route = activityRoute('campaign alpha/β', 'campaign') + '/treatments/' + encodeURIComponent('treatment one') + '/edit';
 assert.equal(route, 'activities/campaign%20alpha%2F%CE%B2/campaign/treatments/treatment%20one/edit');
 assert.deepEqual(parseActivityRoute('#' + route), {
  screen: 'workspace', activityId: 'campaign alpha/β', domain: 'campaign',
  campaignRoute: 'campaign/activities/campaign%20alpha%2F%CE%B2/treatments/treatment%20one/edit',
 });
});

test('old activity asset routes return to the activity and old top-level domains remain understandable', () => {
 assert.deepEqual(parseActivityRoute('#activities/campaign-branch/audience'), {
  screen: 'global', module: 'audience', returnTo: 'activities/campaign-branch/campaign', activityId: null, domain: null, campaignRoute: '',
 });
 assert.deepEqual(parseActivityRoute('#activities/campaign-branch/experimentation'), {
  screen: 'global', module: 'experimentation', returnTo: 'activities/campaign-branch/campaign', activityId: null, domain: null, campaignRoute: '',
 });
 assert.equal(parseActivityRoute('#campaign').screen, 'list');
 assert.equal(parseActivityRoute('#compose').screen, 'list');
 assert.equal(parseActivityRoute('#campaign/activities').screen, 'list');
 assert.equal(parseActivityRoute('#activities/new').screen, 'new');
 assert.equal(parseActivityRoute('#campaign/activities/new').screen, 'new');
});

test('old activity details, malformed IDs and unknown workspace domains have explicit routes', () => {
 assert.deepEqual(parseActivityRoute('#campaign/activities/campaign-branch/edit'), {
  screen: 'workspace', activityId: 'campaign-branch', domain: 'campaign',
  campaignRoute: 'campaign/activities/campaign-branch/edit',
 });
 assert.equal(parseActivityRoute('#activities/campaign-branch/campaign/treatments/new').campaignRoute, 'campaign/activities/campaign-branch/treatments/new');
 assert.equal(parseActivityRoute('#campaign/activities/campaign-branch/treatments/new').campaignRoute, 'campaign/activities/campaign-branch/treatments/new');
 assert.equal(parseActivityRoute('#activities/%ZZ/campaign').screen, 'invalid');
 assert.equal(parseActivityRoute('#activities/campaign-branch/not-a-domain').screen, 'invalid');
 assert.equal(parseActivityRoute('#activities/campaign-branch/campaign/unknown').screen, 'invalid');
 assert.equal(parseActivityRoute('#activities/campaign-branch/campaign/treatments/%ZZ/edit').screen, 'invalid');
 assert.throws(() => activityRoute('', 'campaign'), TypeError);
 assert.throws(() => activityRoute('campaign-branch', 'audience'), TypeError);
});
