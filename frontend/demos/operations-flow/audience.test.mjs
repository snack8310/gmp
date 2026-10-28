import test from 'node:test';
import assert from 'node:assert/strict';
import { MOCK_USERS, DEFAULT_AUDIENCES, DEFAULT_EXPERIMENTS, validateExperiment, validateEntry, summarizeEntry, STORAGE_KEY, validateAudience, filterAudience, summarizeRules, assignGroup, createAudienceStore } from './audience.mjs';
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

test('deleting an experiment protects saved references and persists removal', () => {
  const storage = memory(), store = createAudienceStore(storage);
  const experiment = store.saveExperiment({ name: '待删除实验' });
  const audience = store.save({ ...base(), experimentId: experiment.id });
  assert.throws(() => store.deleteExperiment(experiment.id), /引用/);
  assert.equal(store.experiments().at(-1).id, experiment.id);
  store.save({ ...audience, experimentId: null });
  store.deleteExperiment(experiment.id);
  assert.ok(!createAudienceStore(storage).experiments().some(item => item.id === experiment.id));
});

test('deleting an audience removes only that saved definition', () => {
  const storage=memory(),store=createAudienceStore(storage);
  const created=store.save(base());
  const before=store.list().length;
  store.delete(created.id);
  assert.equal(store.list().length,before-1);
  assert.ok(!createAudienceStore(storage).list().some(value=>value.id===created.id));
  assert.throws(()=>store.delete(created.id),/不存在/);
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

test('v1 caches migrate without writes and default to stopped experiments; explicit save writes v4', () => {
  const storage = memory();
  const oldAudience = base(); delete oldAudience.experimentId; delete oldAudience.entry;
  const raw = JSON.stringify({ version: 1, audiences: [oldAudience] });
  storage.setItem(STORAGE_KEY, raw);
  const store = createAudienceStore(storage);
  assert.equal(store.list()[0].experimentId, null);
  assert.ok(store.experiments().every(experiment => experiment.runtimeStatus === 'stopped'));
  assert.deepEqual(store.bundles(), []);
  assert.equal(storage.getItem(STORAGE_KEY), raw);
  store.save(store.list()[0]);
  assert.equal(JSON.parse(storage.getItem(STORAGE_KEY)).version, 4);
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
    if (payload.experiments.length === 0) { assert.match(store.warning, /旧版配置/); assert.deepEqual(store.experiments(), []); continue; }
    assert.match(store.warning, /当前会话/);
    assert.deepEqual(store.list(), DEFAULT_AUDIENCES);
    assert.deepEqual(store.experiments(), DEFAULT_EXPERIMENTS);
    store.saveExperiment({ name: '仅本会话' });
    assert.equal(storage.getItem(STORAGE_KEY), raw);
  }
});


test('entry validates tagged mode, daily UTC time and event whitelist', () => {
  const valid = [{ mode: 'batch', schedule: { type: 'manual' } },
    { mode: 'batch', schedule: { type: 'daily', time: '00:00', timezone: 'UTC' } },
    { mode: 'batch', schedule: { type: 'daily', time: '23:59', timezone: 'UTC' } },
    ...['app_opened', 'order_paid', 'coupon_claimed'].map(eventType => ({ mode: 'event', eventType }))];
  for (const entry of valid) { assert.deepEqual(validateEntry(entry), []); assert.deepEqual(validateAudience({ ...base(), entry }), []); }
  const invalid = [undefined, null, [], {}, { mode: 'unknown' }, { mode: 'event', eventType: 'refund' },
    { mode: 'event', eventType: '__proto__' }, { mode: 'batch' }, { mode: 'batch', schedule: {} },
    { mode: 'batch', schedule: { type: 'weekly' } },
    ...['24:00', '12:60', '9:00', '', 900].map(time => ({ mode: 'batch', schedule: { type: 'daily', time, timezone: 'UTC' } })),
    { mode: 'batch', schedule: { type: 'daily', time: '09:00', timezone: 'Asia/Shanghai' } }];
  for (const entry of invalid) { assert.ok(validateEntry(entry).length); assert.ok(validateAudience({ ...base(), entry }).length); }
  assert.match(summarizeEntry(DEFAULT_AUDIENCES[0]), /App 模拟来源.*批量进入.*手动/);
  assert.match(summarizeEntry(DEFAULT_AUDIENCES[1]), /每日 09:00 UTC/);
  assert.match(summarizeEntry(DEFAULT_AUDIENCES[2]), /白名单模拟来源.*事件触发.*App 打开/);
});

test('entry normalization keeps only chosen branch and returns deep isolated values', () => {
  const storage = memory(), store = createAudienceStore(storage);
  const event = store.save({ ...base(), entry: { mode: 'event', eventType: 'order_paid', source: 'whitelist', schedule: { type: 'daily', time: '09:00', timezone: 'UTC' } } });
  assert.deepEqual(event.entry, { mode: 'event', eventType: 'order_paid' });
  assert.equal(event.rules.source, 'app');
  event.entry.eventType = 'coupon_claimed';
  assert.equal(store.list()[0].entry.eventType, 'order_paid');
  const batch = store.save({ ...store.list()[0], entry: { mode: 'batch', eventType: 'order_paid', schedule: { type: 'manual', time: '09:00', timezone: 'UTC' } } });
  assert.deepEqual(batch.entry, { mode: 'batch', schedule: { type: 'manual' } });
  batch.entry.schedule.type = 'daily';
  assert.deepEqual(createAudienceStore(storage).list()[0].entry, { mode: 'batch', schedule: { type: 'manual' } });
  assert.equal(JSON.parse(storage.getItem(STORAGE_KEY)).version, 4);
  assert.throws(() => { DEFAULT_AUDIENCES[1].entry.schedule.time = '00:00'; }, TypeError);
});

test('v2 migration preserves experiment identity, warns clearly and leaves original cache untouched', () => {
  const storage = memory(), old = { ...base(), experimentId: 'exp-recall' }; delete old.entry;
  const raw = JSON.stringify({ version: 2, audiences: [old], experiments: DEFAULT_EXPERIMENTS });
  storage.setItem(STORAGE_KEY, raw);
  const store = createAudienceStore(storage);
  assert.deepEqual(store.list()[0].entry, { mode: 'batch', schedule: { type: 'manual' } });
  assert.equal(store.list()[0].experimentId, 'exp-recall');
  assert.match(store.warning, /旧版配置/);
  assert.doesNotMatch(store.warning, /丢失/);
  assert.equal(storage.getItem(STORAGE_KEY), raw);
  store.save(store.list()[0]);
  assert.equal(store.warning, '');
  assert.equal(JSON.parse(storage.getItem(STORAGE_KEY)).version, 4);
  assert.deepEqual(createAudienceStore(storage).list(), store.list());
});

test('v3 missing or malformed entry fails closed without replacing original cache', () => {
  for (const version of [2, 3]) {
    for (const entry of [null, { mode: 'unknown' }, { mode: 'event', eventType: 'unknown' }, { mode: 'batch', schedule: { type: 'daily', time: '99:00', timezone: 'UTC' } }]) {
      const storage = memory(), raw = JSON.stringify({ version, audiences: [{ ...base(), entry }], experiments: DEFAULT_EXPERIMENTS });
      storage.setItem(STORAGE_KEY, raw);
      const store = createAudienceStore(storage);
      assert.match(store.warning, /损坏/);
      assert.deepEqual(store.list(), DEFAULT_AUDIENCES);
      store.save({ ...base(), name: '仅内存' });
      assert.equal(storage.getItem(STORAGE_KEY), raw);
    }
  }
  const storage = memory(), missing = base(); delete missing.entry;
  storage.setItem(STORAGE_KEY, JSON.stringify({ version: 3, audiences: [missing], experiments: DEFAULT_EXPERIMENTS }));
  assert.match(createAudienceStore(storage).warning, /损坏/);
});

test('publishing requires a running saved split, complete unique arms and one source', () => {
  const store = createAudienceStore(memory());
  const audience = store.list().find(item => item.id === 'aud-mx-dormant');
  const arms = ['a', 'b', 'control'].map(part => ({ part, name: part === 'control' ? '对照臂' : `${part.toUpperCase()} 臂`, audienceId: audience.id }));
  const input = { name: '新包', experimentId: 'exp-recall', arms };
  for (const bad of [
    { ...input, experimentId: 'missing' },
    { ...input, arms: arms.slice(0, 2) },
    { ...input, arms: [arms[0], arms[0], arms[2]] },
    { ...input, arms: arms.map(arm => ({ ...arm, groupId: 'same-id' })) },
    { ...input, id: 'bundle-warmup' },
    { ...input, arms: [null, arms[1], arms[2]] },
  ]) assert.throws(() => store.publishBundle(bad), RangeError);
  const otherSource = store.save({ ...store.list().find(item => item.id === 'aud-whitelist-new'), id: 'aud-other-source', experimentId: 'exp-recall' });
  assert.throws(() => store.publishBundle({ ...input, arms: [{ ...arms[0], audienceId: otherSource.id }, ...arms.slice(1)] }), /同一来源/);
  store.setExperimentStatus('exp-recall', 'stopped');
  assert.throws(() => store.publishBundle(input), /运行中/);
  store.setExperimentStatus('exp-recall', 'running');
  const published = store.publishBundle(input);
  assert.equal(new Set(published.arms.map(arm => arm.groupId)).size, 3);
  assert.ok(published.arms.some(arm => arm.part === 'control'));
  assert.ok(store.catalog().filter(item => item.bundleId === published.id).every(item => item.available));
});

test('published identity survives reload; stop invalidates old arms without revival', () => {
  const storage = memory(), store = createAudienceStore(storage);
  const audience = store.list().find(item => item.id === 'aud-mx-dormant');
  const arms = ['a', 'b', 'control'].map(part => ({ part, name: part, audienceId: audience.id }));
  const bundle = store.publishBundle({ name: '持久包', experimentId: 'exp-recall', arms });
  assert.equal(JSON.parse(storage.getItem(STORAGE_KEY)).version, 4);
  assert.deepEqual(createAudienceStore(storage).bundles().find(item => item.id === bundle.id), bundle);
  store.setExperimentStatus('exp-recall', 'stopped');
  assert.ok(store.catalog().filter(item => item.bundleId === bundle.id).every(item => !item.available));
  store.setExperimentStatus('exp-recall', 'running');
  assert.ok(store.catalog().filter(item => item.bundleId === bundle.id).every(item => !item.available));
  const fresh = store.publishBundle({ name: '重发包', experimentId: 'exp-recall', arms });
  assert.ok(store.catalog().filter(item => item.bundleId === fresh.id).every(item => item.available));
  assert.ok(createAudienceStore(storage).catalog().filter(item => item.bundleId === bundle.id).every(item => !item.available));
});

test('saved experiment reference alone is an ordinary group; cited bundles and groups resist deletion', () => {
  const activities = [{ treatments: [{ audienceRef: { type: 'group', groupId: 'group-custom' } }] }];
  const store = createAudienceStore(memory(), activities);
  const saved = store.save({ ...base(), id: 'custom', name: '普通组', experimentId: 'exp-recall' });
  assert.equal(store.catalog().find(item => item.groupId === 'group-custom').type, 'group');
  assert.equal(store.catalog().filter(item => item.type === 'arm').length, 3);
  assert.throws(() => store.delete(saved.id), /不能删除/);
  activities.push({ treatments: [{ audienceRef: { type: 'arm', bundleId: 'bundle-warmup', groupId: 'group-arm-a' } }] });
  assert.throws(() => store.deleteBundle('bundle-warmup'), /不能删除/);
  assert.throws(() => store.delete('aud-mx-dormant'), /不能删除/);
});

test('editing a published bundle source cannot write a cache that fails on reload', () => {
  const storage = memory(), store = createAudienceStore(storage);
  const audience = store.list().find(item => item.id === 'aud-mx-dormant');
  assert.throws(() => store.save({ ...audience, experimentId: null }), /不能修改实验引用/);
  assert.throws(() => store.save({ ...audience, rules: { ...audience.rules, source: 'whitelist' } }), /不能修改来源/);
  assert.equal(storage.getItem(STORAGE_KEY), null);
  store.save({ ...audience, name: '允许改显示名' });
  const loaded = createAudienceStore(storage);
  assert.equal(loaded.warning, '');
  assert.equal(loaded.list().find(item => item.id === audience.id).name, '允许改显示名');
  assert.ok(loaded.catalog().find(item => item.bundleId === 'bundle-warmup').available);
});

test('v2-v4 missing runtime is stopped and damaged cache is never overwritten', () => {
  for (const version of [2, 3, 4]) {
    const storage = memory(), payload = { version, audiences: [base()], experiments: [{ id: 'exp-recall', name: '旧分流' }] };
    if (version === 4) payload.bundles = [];
    const raw = JSON.stringify(payload); storage.setItem(STORAGE_KEY, raw);
    const store = createAudienceStore(storage);
    assert.equal(store.experiments()[0].runtimeStatus, 'stopped');
    assert.deepEqual(store.bundles(), []);
    assert.equal(storage.getItem(STORAGE_KEY), raw);
    store.save(store.list()[0]);
    assert.equal(JSON.parse(storage.getItem(STORAGE_KEY)).version, 4);
  }
  for (const raw of ['{broken', JSON.stringify({ version: 99, audiences: [] }), JSON.stringify({ version: 4, audiences: [base()], experiments: [{ id: 'exp-recall', name: '旧分流' }], bundles: null })]) {
    const storage = memory(); storage.setItem(STORAGE_KEY, raw);
    const store = createAudienceStore(storage);
    assert.match(store.warning, /当前会话/);
    store.save({ ...base(), name: '会话修改' });
    assert.equal(storage.getItem(STORAGE_KEY), raw);
  }
});
