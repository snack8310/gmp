import test from 'node:test';
import assert from 'node:assert/strict';
import { MOCK_USERS, DEFAULT_AUDIENCES, DEFAULT_EXPERIMENTS, validateExperiment, STORAGE_KEY, validateAudience, filterAudience, summarizeRules, assignGroup, createAudienceStore } from './audience.mjs';
const base = () => structuredClone(DEFAULT_AUDIENCES[0]);
const memory = () => { const data = new Map(); return { getItem: key => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) }; };

test('fixtures are synthetic, unique, frozen and deterministically cover each selector', () => {
  assert.equal(MOCK_USERS.length, 240);
  assert.equal(new Set(MOCK_USERS.map(user => user.uid)).size, MOCK_USERS.length);
  assert.ok(MOCK_USERS.every((user, index) => user.id === index && /^SIM-\d{4}$/.test(user.uid) && user.inactiveDays >= 0 && user.inactiveDays <= 120));
  assert.deepEqual([...new Set(MOCK_USERS.map(user => user.country))].sort(), ['BR', 'CO', 'MX']);
  assert.deepEqual([...new Set(MOCK_USERS.map(user => user.source))].sort(), ['app', 'whitelist']);
  assert.throws(() => { MOCK_USERS[0].country = 'BR'; }, TypeError);
  for (const definition of DEFAULT_AUDIENCES) { assert.deepEqual(validateAudience(definition), []); assert.ok(filterAudience(definition.rules).length > 0); }
});

test('all criteria are ANDed, inclusive boundaries and zero match are accurate', () => {
  const user = MOCK_USERS.find(user => user.source === 'app');
  const rules = { ...base().rules, country: user.country, lifecycle: user.lifecycle, minInactive: user.inactiveDays, maxInactive: user.inactiveDays };
  const matched = filterAudience(rules);
  assert.ok(matched.includes(user));
  assert.ok(matched.every(item => Object.entries(rules).every(([key, value]) => key === 'minInactive' ? item.inactiveDays >= value : key === 'maxInactive' ? item.inactiveDays <= value : item[key] === value)));
  assert.equal(filterAudience({ ...rules, minInactive: 121, maxInactive: 365 }).length, 0);
  assert.equal(filterAudience(base().rules).length, MOCK_USERS.filter(user => user.source === 'app').length);
  assert.match(summarizeRules(rules), /未活跃/);
});

test('invalid names, IDs, enums, missing and noninteger day ranges fail closed', () => {
  for (const definition of [null, [], {}, { ...base(), name: ' ' }, { ...base(), name: 'a'.repeat(49) }, { ...base(), id: '<script>' }]) assert.ok(validateAudience(definition).length);
  for (const patch of [{ source: 'all' }, { source: '__proto__' }, { country: 'XX' }, { lifecycle: 'other' }, { minInactive: '' }, { minInactive: NaN }, { minInactive: .5 }, { minInactive: -1 }, { maxInactive: 366 }, { minInactive: 20, maxInactive: 10 }]) {
    const rules = { ...base().rules, ...patch };
    assert.ok(validateAudience({ ...base(), rules }).length);
    assert.throws(() => filterAudience(rules), RangeError);
  }
});

test('assignment is stable per identity, disjoint, and off by default', () => {
  const groups = MOCK_USERS.map(user => assignGroup(user.uid, 'exp-recall'));
  assert.deepEqual([...MOCK_USERS].reverse().map(user => assignGroup(user.uid, 'exp-recall')).reverse(), groups);
  assert.ok(MOCK_USERS.every(user => assignGroup(user.uid) === 'direct'));
  assert.deepEqual([...new Set(groups)].sort(), ['a', 'b', 'control']);
  const subset = filterAudience(DEFAULT_AUDIENCES[1].rules);
  for (const user of subset) assert.equal(assignGroup(user.uid, 'exp-recall'), groups[user.id]);
  assert.throws(() => assignGroup('', 'exp-recall'), RangeError);
});

test('save persists only definitions, returns isolated copies and updates instead of duplicating', () => {
  const storage = memory(), store = createAudienceStore(storage);
  const draft = store.list()[0]; draft.name = '未保存'; draft.rules.country = 'BR';
  assert.notEqual(store.list()[0].name, draft.name);
  const saved = store.save({ ...base(), id: '', name: '  我的模拟人群  ', users: MOCK_USERS });
  assert.equal(saved.name, '我的模拟人群');
  saved.name = '返回值不可污染';
  const length = store.list().length;
  store.save({ ...store.list().at(-1), name: '再次修改' });
  assert.equal(store.list().length, length);
  assert.equal(createAudienceStore(storage).list().at(-1).name, '再次修改');
  assert.equal(storage.getItem(STORAGE_KEY).includes('SIM-'), false);
  assert.equal(storage.getItem(STORAGE_KEY).includes('users'), false);
  assert.equal(store.warning, '');
  assert.throws(() => store.save({ ...base(), name: '' }), RangeError);
  assert.equal(store.list().length, length);
});

test('storage denial, quota failure, bad JSON and unknown versions safely use memory', () => {
  const variants = [undefined, { getItem() { throw new Error('denied'); }, setItem() {} },
    { getItem() { return null; }, setItem() { throw new Error('quota'); } },
    { getItem() { return '{bad'; }, setItem() { throw new Error('must not overwrite corruption'); } },
    { getItem() { return JSON.stringify({ version: 99, audiences: [] }); }, setItem() {} },
    { getItem() { return JSON.stringify({ version: 1, audiences: [{ ...base(), rules: {} }] }); }, setItem() {} },
    { getItem() { return JSON.stringify({ version: 1, audiences: [base(), base()] }); }, setItem() {} }];
  for (const storage of variants) {
    const store = createAudienceStore(storage);
    store.save({ ...base(), name: '内存修改' });
    assert.equal(store.list()[0].name, '内存修改');
    assert.match(store.warning, /当前会话/);
  }
});

test('a valid empty saved list stays empty, new IDs remain unique', () => {
  const storage = memory(); storage.setItem(STORAGE_KEY, JSON.stringify({ version: 1, audiences: [] }));
  const store = createAudienceStore(storage);
  assert.deepEqual(store.list(), []);
  const one = store.save({ ...base(), id: '' });
  const two = store.save({ ...base(), id: '' });
  assert.notEqual(one.id, two.id);
  assert.equal(store.list().length, 2);
});


test('experiments are reusable independent identities, renaming keeps assignments', () => {
  const storage = memory(), store = createAudienceStore(storage);
  assert.deepEqual(store.experiments(), DEFAULT_EXPERIMENTS);
  const experiment = store.saveExperiment({ name: '  新实验  ' });
  assert.equal(experiment.name, '新实验');
  const first = store.save({ ...base(), id: '', experimentId: experiment.id });
  const second = store.save({ ...base(), id: '', experimentId: experiment.id });
  const before = MOCK_USERS.map(user => assignGroup(user.uid, first.experimentId));
  assert.deepEqual(MOCK_USERS.map(user => assignGroup(user.uid, second.experimentId)), before);
  const size = store.experiments().length;
  store.saveExperiment({ ...experiment, name: '已改名' });
  assert.equal(store.experiments().length, size);
  assert.deepEqual(MOCK_USERS.map(user => assignGroup(user.uid, experiment.id)), before);
  assert.notDeepEqual(MOCK_USERS.map(user => assignGroup(user.uid, 'exp-onboarding')), before);
  assert.equal(createAudienceStore(storage).experiments().at(-1).name, '已改名');
  assert.equal(createAudienceStore(storage).list().at(-1).experimentId, experiment.id);
  const isolated = store.experiments(); isolated[0].name = '不可污染';
  experiment.name = '返回值不可污染';
  assert.notEqual(store.experiments()[0].name, isolated[0].name);
  assert.equal(store.experiments().at(-1).name, '已改名');
});

test('experiment validation and orphan references fail before any mutation', () => {
  const store = createAudienceStore(memory());
  const previous = store.list();
  assert.throws(() => store.save({ ...base(), experimentId: 'missing-experiment' }), RangeError);
  assert.deepEqual(store.list(), previous);
  for (const value of [true, false, '', {}, 7]) {
    assert.throws(() => assignGroup('SIM-0001', value), RangeError);
    assert.ok(validateAudience({ ...base(), experimentId: value }).length);
  }
  for (const value of [null, {}, { name: '' }, { name: 'a'.repeat(49) }, { name: 'Good', id: '<bad>' }]) {
    assert.ok(validateExperiment(value).length);
    assert.throws(() => store.saveExperiment(value), RangeError);
  }
  assert.deepEqual(store.experiments(), DEFAULT_EXPERIMENTS);
});

test('v1 caches migrate without writes and default to no experiment; explicit save writes v2', () => {
  const storage = memory();
  const oldAudience = base(); delete oldAudience.experimentId;
  const raw = JSON.stringify({ version: 1, audiences: [oldAudience] });
  storage.setItem(STORAGE_KEY, raw);
  const store = createAudienceStore(storage);
  assert.equal(store.list()[0].experimentId, null);
  assert.deepEqual(store.experiments(), DEFAULT_EXPERIMENTS);
  assert.equal(storage.getItem(STORAGE_KEY), raw);
  store.save(store.list()[0]);
  assert.equal(JSON.parse(storage.getItem(STORAGE_KEY)).version, 2);
  assert.deepEqual(createAudienceStore(storage).list(), store.list());
});

test('corrupt v2 experiments or orphan references fall back atomically without overwriting', () => {
  for (const payload of [
    { version: 2, audiences: [base()], experiments: [] },
    { version: 2, audiences: [{ ...base(), experimentId: 'missing' }], experiments: DEFAULT_EXPERIMENTS },
    { version: 2, audiences: [base()], experiments: [DEFAULT_EXPERIMENTS[0], DEFAULT_EXPERIMENTS[0]] },
    { version: 2, audiences: [base()], experiments: [{ id: 'bad', name: '' }] },
  ]) {
    const storage = memory(); const raw = JSON.stringify(payload); storage.setItem(STORAGE_KEY, raw);
    const store = createAudienceStore(storage);
    if (payload.experiments.length === 0) { assert.equal(store.warning, ''); assert.deepEqual(store.experiments(), []); continue; }
    assert.match(store.warning, /当前会话/);
    assert.deepEqual(store.list(), DEFAULT_AUDIENCES);
    assert.deepEqual(store.experiments(), DEFAULT_EXPERIMENTS);
    store.saveExperiment({ name: '仅本会话' });
    assert.equal(storage.getItem(STORAGE_KEY), raw);
  }
});
