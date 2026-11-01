import test from 'node:test';
import assert from 'node:assert/strict';
import { activityRoute, parseActivityRoute } from './activity-routing.mjs';

test('activity routes place shared domains inside a selected activity', () => {
 assert.deepEqual(parseActivityRoute('#activities/campaign-branch/experimentation'), {
  screen: 'workspace', activityId: 'campaign-branch', domain: 'experimentation', campaignRoute: 'campaign/activities/campaign-branch',
 });
 assert.equal(activityRoute('campaign-branch', 'campaign'), 'activities/campaign-branch/campaign');
});

test('activity and nested operation IDs are encoded and decoded by path segment', () => {
 const route = activityRoute('campaign alpha/β', 'campaign') + '/treatments/' + encodeURIComponent('treatment one') + '/edit';
 assert.equal(route, 'activities/campaign%20alpha%2F%CE%B2/campaign/treatments/treatment%20one/edit');
 assert.deepEqual(parseActivityRoute('#' + route), {
  screen: 'workspace', activityId: 'campaign alpha/β', domain: 'campaign',
  campaignRoute: 'campaign/activities/campaign%20alpha%2F%CE%B2/treatments/treatment%20one/edit',
 });
});

test('activity list and create routes accept old top-level entry links', () => {
 assert.equal(parseActivityRoute('').screen, 'list');
 assert.equal(parseActivityRoute('#activities').screen, 'list');
 assert.equal(parseActivityRoute('#activities/new').screen, 'new');
 for (const legacy of ['#audience', '#experimentation', '#campaign', '#compose', '#campaign/activities']) {
  assert.equal(parseActivityRoute(legacy).screen, 'list');
 }
 assert.equal(parseActivityRoute('#campaign/activities/new').screen, 'new');
});

test('old activity detail and editor deep links map into the campaign subpage', () => {
 assert.deepEqual(parseActivityRoute('#campaign/activities/campaign-branch/edit'), {
  screen: 'workspace', activityId: 'campaign-branch', domain: 'campaign',
  campaignRoute: 'campaign/activities/campaign-branch/edit',
 });
 assert.equal(parseActivityRoute('#activities/campaign-branch/campaign/treatments/new').campaignRoute, 'campaign/activities/campaign-branch/treatments/new');
 assert.equal(parseActivityRoute('#campaign/activities/campaign-branch/treatments/new').campaignRoute, 'campaign/activities/campaign-branch/treatments/new');
});

test('malformed activity IDs and unknown workspace domains do not become valid activity routes', () => {
 assert.equal(parseActivityRoute('#activities/%ZZ/audience').screen, 'invalid');
 assert.equal(parseActivityRoute('#activities/campaign-branch/not-a-domain').screen, 'invalid');
 assert.equal(parseActivityRoute('#activities/campaign-branch/campaign/unknown').screen, 'invalid');
 assert.equal(parseActivityRoute('#activities/campaign-branch/campaign/treatments/%ZZ/edit').screen, 'invalid');
 assert.throws(() => activityRoute('', 'audience'), TypeError);
});
