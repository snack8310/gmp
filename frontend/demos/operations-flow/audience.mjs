// Presentation fixtures only: these records do not describe real people.
export const STORAGE_KEY = 'gmp-audience-demo-v1';
export const BACKUP_KEY = `${STORAGE_KEY}-backup-v4`;
export const DEFAULT_SPLIT = Object.freeze({ a: 45, b: 45, control: 10 });
const COUNTRIES = { MX: '墨西哥', BR: '巴西', CO: '哥伦比亚' };
const SOURCES = { app: 'App 模拟来源', whitelist: '白名单模拟来源' };
const LIFECYCLES = { new: '新客', active: '活跃', dormant: '沉睡' };
export const MOCK_USERS = Object.freeze(Array.from({ length: 240 }, (_, id) => {
  const inactiveDays = (id * 47 + 11) % 121;
  return Object.freeze({ id, uid: `SIM-${String(id + 1).padStart(4, '0')}`,
    country: ['MX', 'BR', 'CO'][id % 3], source: id % 5 === 0 ? 'whitelist' : 'app',
    lifecycle: id % 7 === 0 ? 'new' : inactiveDays >= 30 ? 'dormant' : 'active', inactiveDays });
}));
export const DEFAULT_EXPERIMENTS = Object.freeze([
  Object.freeze({ id: 'exp-recall', name: '召回策略实验', runtimeStatus: 'running', generation: 0, split: DEFAULT_SPLIT, colored: false }),
  Object.freeze({ id: 'exp-onboarding', name: '新客体验实验', runtimeStatus: 'stopped', generation: 0, split: DEFAULT_SPLIT, colored: false }),
]);
export const DEFAULT_BUNDLES = Object.freeze([Object.freeze({
  id: 'bundle-warmup', name: '预热实验人群包', experimentId: 'exp-recall', generation: 0,
  arms: Object.freeze(['a', 'b', 'control'].map((part, index) => Object.freeze({
    part, groupId: `group-arm-${part}`, name: part === 'control' ? '对照臂' : `${part.toUpperCase()} 臂`, audienceId: 'aud-mx-dormant', source: 'app',
  }))),
})]);
const defaultRules = { country: 'all', source: 'app', lifecycle: 'all', minInactive: 0, maxInactive: 365 };
export const DEFAULT_AUDIENCES = Object.freeze([
  { entry: { mode: 'batch', schedule: { type: 'manual' } }, id: 'aud-app', name: 'App 全体模拟人群', rules: { ...defaultRules } },
  { entry: { mode: 'batch', schedule: { type: 'daily', time: '09:00', timezone: 'UTC' } }, id: 'aud-mx-dormant', name: '墨西哥 · 沉睡召回', rules: { ...defaultRules, country: 'MX', lifecycle: 'dormant', minInactive: 30 } },
  { entry: { mode: 'event', eventType: 'app_opened' }, id: 'aud-whitelist-new', name: '白名单 · 新客体验', rules: { ...defaultRules, source: 'whitelist', lifecycle: 'new' } },
].map(value => Object.freeze({ ...value, experimentId: value.id === 'aud-mx-dormant' ? 'exp-recall' : null, entry: Object.freeze(value.entry.mode === 'batch' ? { ...value.entry, schedule: Object.freeze(value.entry.schedule) } : value.entry), rules: Object.freeze(value.rules) })));
const clone = value => JSON.parse(JSON.stringify(value));
const owns = (object, key) => typeof key === 'string' && Object.hasOwn(object, key);
const activityUsesGroup = (activity, groupId) => activity.audience?.type === 'group' && activity.audience.groupId === groupId
  || activity.activityFlow?.entries?.some(entry => entry.groupId === groupId);
function validateRules(rules) {
  if (!rules || typeof rules !== 'object' || Array.isArray(rules)) return ['请设置筛选条件。'];
  const errors = [];
  if (rules.country !== 'all' && !owns(COUNTRIES, rules.country)) errors.push('请选择有效国家。');
  if (!owns(SOURCES, rules.source)) errors.push('请选择一个模拟来源。');
  if (rules.lifecycle !== 'all' && !owns(LIFECYCLES, rules.lifecycle)) errors.push('请选择有效生命周期。');
  for (const field of ['minInactive', 'maxInactive']) {
    if (!Number.isInteger(rules[field]) || rules[field] < 0 || rules[field] > 365) errors.push('未活跃天数须为 0 至 365 的整数。');
  }
  if (Number.isInteger(rules.minInactive) && Number.isInteger(rules.maxInactive) && rules.minInactive > rules.maxInactive) errors.push('最少天数不能大于最多天数。');
  return [...new Set(errors)];
}
const EVENT_TYPES = { app_opened: 'App 打开', order_paid: '订单支付', coupon_claimed: '领取优惠券' };
export function validateEntry(entry) {
  if (!entry || typeof entry !== 'object' || Array.isArray(entry)) return ['请选择人群进入方式。'];
  if (entry.mode === 'event') return owns(EVENT_TYPES, entry.eventType) ? [] : ['请选择有效的模拟事件。'];
  if (entry.mode !== 'batch') return ['请选择批量进入或事件触发。'];
  const schedule = entry.schedule;
  if (!schedule || typeof schedule !== 'object' || Array.isArray(schedule)) return ['请选择批量排期方式。'];
  if (schedule.type === 'manual') return [];
  if (schedule.type !== 'daily') return ['请选择手动或每日排期。'];
  const errors = [];
  if (typeof schedule.time !== 'string' || !/^([01]\d|2[0-3]):[0-5]\d$/.test(schedule.time)) errors.push('每日时间须为有效的 HH:mm 格式。');
  if (schedule.timezone !== 'UTC') errors.push('演示排期仅支持 UTC 时区。');
  return errors;
}
export function summarizeEntry(definition) {
  if (!definition || !owns(SOURCES, definition.rules?.source) || validateEntry(definition.entry).length) return '入口配置无效';
  const entry = definition.entry;
  const mode = entry.mode === 'event' ? `事件触发 · ${EVENT_TYPES[entry.eventType]} (${entry.eventType})`
    : entry.schedule.type === 'manual' ? '批量进入 · 手动模拟'
    : `批量进入 · 每日 ${entry.schedule.time} UTC（仅配置）`;
  return `${SOURCES[definition.rules.source]} · ${mode}`;
}
function normalizeEntry(entry) {
  if (entry.mode === 'event') return { mode: 'event', eventType: entry.eventType };
  return { mode: 'batch', schedule: entry.schedule.type === 'manual' ? { type: 'manual' }
    : { type: 'daily', time: entry.schedule.time, timezone: 'UTC' } };
}
const validId = id => typeof id === 'string' && /^[a-zA-Z0-9_-]{1,64}$/.test(id);
export function validateSplit(split) {
  return split && typeof split === 'object' && !Array.isArray(split)
    && ['a', 'b', 'control'].every(part => Number.isInteger(split[part]) && split[part] >= 0 && split[part] <= 100)
    && Object.keys(split).length === 3 && split.a + split.b + split.control === 100 ? [] : ['A、B、对照臂比例须为 0 至 100 的整数，合计 100%。'];
}
export function validateExperiment(experiment) {
  if (!experiment || typeof experiment !== 'object' || Array.isArray(experiment)) return ['请填写实验定义。'];
  const errors = [];
  if (typeof experiment.name !== 'string' || !experiment.name.trim() || experiment.name.trim().length > 48) errors.push('实验名称须为 1 至 48 个字符。');
  if (experiment.id !== undefined && experiment.id !== '' && !validId(experiment.id)) errors.push('实验标识无效。');
  if (experiment.runtimeStatus !== undefined && !['running', 'stopped'].includes(experiment.runtimeStatus)) errors.push('实验运行态无效。');
  if (experiment.generation !== undefined && (!Number.isSafeInteger(experiment.generation) || experiment.generation < 0)) errors.push('实验代际无效。');
  errors.push(...validateSplit(experiment.split));
  if (typeof experiment.colored !== 'boolean') errors.push('请选择是否记录首次分配。');
  return errors;
}
export function validateAudience(definition, experiments) {
  if (!definition || typeof definition !== 'object' || Array.isArray(definition)) return ['请填写人群定义。'];
  const errors = [...validateRules(definition.rules), ...validateEntry(definition.entry)];
  if (typeof definition.name !== 'string' || !definition.name.trim() || definition.name.trim().length > 48) errors.unshift('名称须为 1 至 48 个字符。');
  if (definition.id !== undefined && definition.id !== '' && (typeof definition.id !== 'string' || !/^[a-zA-Z0-9_-]{1,64}$/.test(definition.id))) errors.push('人群标识无效。');
  if (definition.experimentId !== undefined && definition.experimentId !== null) {
    if (!validId(definition.experimentId)) errors.push('实验引用无效。');
    else if (experiments && !experiments.some(experiment => experiment.id === definition.experimentId)) errors.push('引用的实验不存在，请重新选择。');
  }
  return errors;
}
export function filterAudience(rules, users = MOCK_USERS) {
  const errors = validateRules(rules);
  if (errors.length) throw new RangeError(errors.join(' '));
  return users.filter(user => user.source === rules.source && (rules.country === 'all' || user.country === rules.country)
    && (rules.lifecycle === 'all' || user.lifecycle === rules.lifecycle)
    && user.inactiveDays >= rules.minInactive && user.inactiveDays <= rules.maxInactive);
}
export function summarizeRules(rules) {
  if (validateRules(rules).length) return '筛选条件无效';
  return `${SOURCES[rules.source]} · ${rules.country === 'all' ? '全部国家' : COUNTRIES[rules.country]} · ${rules.lifecycle === 'all' ? '全部生命周期' : LIFECYCLES[rules.lifecycle]} · 未活跃 ${rules.minInactive}–${rules.maxInactive} 天`;
}
export function assignGroup(uid, experimentId = null, split = DEFAULT_SPLIT) {
  if (experimentId === null) return 'direct';
  if (!validId(experimentId)) throw new RangeError('实验标识无效。');
  if (typeof uid !== 'string' || !uid) throw new RangeError('模拟用户标识不能为空。');
  if (validateSplit(split).length) throw new RangeError(validateSplit(split).join(' '));
  // Fixed presentation-only salt/version, experiment identity, independent from names, ordering and filters.
  let hash = 2166136261;
  for (const character of `gmp-demo-split-v1:${experimentId}:${uid}`) hash = Math.imul(hash ^ character.charCodeAt(0), 16777619) >>> 0;
  const bucket = hash % 10000;
  return bucket < split.a * 100 ? 'a' : bucket < (split.a + split.b) * 100 ? 'b' : 'control';
}
function normalize(definition, id) {
  const { country, source, lifecycle, minInactive, maxInactive } = definition.rules;
  return { id, name: definition.name.trim(), entry: normalizeEntry(definition.entry), experimentId: definition.experimentId ?? null, rules: { country, source, lifecycle, minInactive, maxInactive } };
}
function validCollection(values, validate) {
  return Array.isArray(values) && values.length <= 100
    && values.every(value => validate(value).length === 0 && value.id)
    && new Set(values.map(value => value.id)).size === values.length;
}
function nextId(values, prefix) {
  let serial = 1;
  while (values.some(value => value.id === `${prefix}-${serial}`)) serial++;
  return `${prefix}-${serial}`;
}
export const groupIdOf = id => id === 'aud-mx-dormant' ? 'group-recall' : id === 'aud-whitelist-new' ? 'group-new' : `group-${id}`;
const experimentState = experiment => ({ id: experiment.id, name: experiment.name.trim(), runtimeStatus: experiment.runtimeStatus ?? 'stopped', generation: experiment.generation ?? 0,
  split: { a: experiment.split.a, b: experiment.split.b, control: experiment.split.control }, colored: experiment.colored });
function validateBundle(bundle, definitions, experiments, bundles) {
  if (!bundle || typeof bundle !== 'object' || Array.isArray(bundle)) return ['实验人群包无效。'];
  const errors = [];
  if (!validId(bundle.id) || bundles.some(item => item.id === bundle.id)) errors.push('实验人群包 ID 重复或无效。');
  if (typeof bundle.name !== 'string' || !bundle.name.trim() || bundle.name.trim().length > 48) errors.push('实验人群包名称无效。');
  if (!experiments.some(item => item.id === bundle.experimentId)) errors.push('引用的实验不存在。');
  if (!Number.isSafeInteger(bundle.generation) || bundle.generation < 0) errors.push('实验代际无效。');
  if (!Array.isArray(bundle.arms) || bundle.arms.length !== 3 || new Set(bundle.arms.map(arm => arm?.part)).size !== 3 || !['a', 'b', 'control'].every(part => bundle.arms.some(arm => arm?.part === part))) errors.push('须包含完整且不重复的 A、B、对照臂。');
  const usedGroups = new Set(definitions.map(item => groupIdOf(item.id)).concat(bundles.flatMap(item => item.arms.map(arm => arm.groupId))));
  const sources = new Set();
  for (const arm of Array.isArray(bundle.arms) ? bundle.arms : []) {
    const audience = definitions.find(item => item.id === arm?.audienceId);
    if (!validId(arm?.groupId) || usedGroups.has(arm.groupId)) errors.push('臂人群组 ID 重复或无效。');
    usedGroups.add(arm?.groupId);
    if (typeof arm?.name !== 'string' || !arm.name.trim() || arm.name.trim().length > 48) errors.push('臂名称无效。');
    if (!audience || audience.experimentId !== bundle.experimentId || audience.rules.source !== arm.source) errors.push('臂须引用同一实验下已保存的人群及来源。');
    if (audience) sources.add(audience.rules.source);
  }
  if (sources.size !== 1) errors.push('所有臂必须来自同一来源。');
  return [...new Set(errors)];
}
export function buildReferenceCatalog(definitions, experiments, bundles) {
  const groups = definitions.map(item => ({ type: 'group', groupId: groupIdOf(item.id), groupName: item.name, available: true }));
  const arms = bundles.flatMap(bundle => bundle.arms.map(arm => ({
    type: 'arm', bundleId: bundle.id, bundleName: bundle.name, groupId: arm.groupId, armName: arm.name, part: arm.part,
    available: experiments.some(experiment => experiment.id === bundle.experimentId && experiment.runtimeStatus === 'running' && experiment.generation === bundle.generation)
      && definitions.some(item => item.id === arm.audienceId && item.experimentId === bundle.experimentId && item.rules.source === arm.source),
  })));
  return [...groups, ...arms];
}
export function createAudienceStore(storage, activities = []) {
  let definitions = clone(DEFAULT_AUDIENCES), experiments = clone(DEFAULT_EXPERIMENTS), bundles = clone(DEFAULT_BUNDLES), coloring = {}, warning = '', usable = true, legacyRaw = null;
  const logStorage = (operation, outcome) => globalThis.console?.info?.(JSON.stringify({ component: 'audience-demo', operation, outcome }));
  const unavailable = () => { usable = false; warning = '浏览器存储不可用：修改仅保留在当前会话，刷新后会丢失。'; };
  try {
    if (!storage || typeof storage.getItem !== 'function' || typeof storage.setItem !== 'function') throw new Error('Storage unavailable');
    const raw = storage.getItem(STORAGE_KEY);
    logStorage('read', 'ok');
    if (raw !== null) {
      const value = JSON.parse(raw);
      if (!value || ![1, 2, 3, 4, 5].includes(value.version)) throw new SyntaxError('Unknown stored version');
      // Legacy schemas had no entry field. Explicit malformed fields are never defaulted.
      const loadedAudiences = value.version < 3 && Array.isArray(value.audiences)
        ? value.audiences.map(def => def && typeof def === 'object' && !Array.isArray(def) && !Object.hasOwn(def, 'entry')
          ? { ...def, entry: { mode: 'batch', schedule: { type: 'manual' } } } : def) : value.audiences;
      const oldExperiments = value.version === 1 ? DEFAULT_EXPERIMENTS.map(({ id, name }) => ({ id, name })) : value.experiments;
      const loadedExperiments = value.version < 5 && Array.isArray(oldExperiments)
        ? oldExperiments.map(item => item && typeof item === 'object' && !Array.isArray(item) ? { ...item, split: DEFAULT_SPLIT, colored: false } : item) : oldExperiments;
      const loadedBundles = value.version >= 4 && Object.hasOwn(value, 'bundles') ? value.bundles : value.version === 5 ? null : [];
      const loadedColoring = value.version === 5 ? value.coloring : {};
      const validColoring = loadedColoring && typeof loadedColoring === 'object' && !Array.isArray(loadedColoring)
        && Object.keys(loadedColoring).every(id => loadedExperiments?.some(experiment => experiment?.id === id && experiment.colored)
          && loadedColoring[id] && typeof loadedColoring[id] === 'object' && !Array.isArray(loadedColoring[id])
          && Object.keys(loadedColoring[id]).length <= MOCK_USERS.length
          && Object.entries(loadedColoring[id]).every(([uid, part]) => /^SIM-\d{4}$/.test(uid) && ['a', 'b', 'control'].includes(part)));
      if (!validCollection(loadedExperiments, validateExperiment)
        || !validCollection(loadedAudiences, def => validateAudience(def, loadedExperiments))
        || (value.version === 1 && value.audiences.some(def => def.experimentId != null))
        || !Array.isArray(loadedBundles) || loadedBundles.length > 100 || !validColoring
        || new Set(loadedAudiences.map(item => groupIdOf(item.id))).size !== loadedAudiences.length) throw new SyntaxError('Invalid stored definitions');
      const normalizedExperiments = loadedExperiments.map(experimentState);
      const checkedBundles = [];
      for (const bundle of loadedBundles) {
        if (validateBundle(bundle, loadedAudiences, normalizedExperiments, checkedBundles).length) throw new SyntaxError('Invalid stored bundle');
        checkedBundles.push(bundle);
      }
      // Commit both collections only after all references have been validated.
      definitions = loadedAudiences.map(def => normalize(def, def.id));
      experiments = normalizedExperiments;
      bundles = clone(checkedBundles);
      coloring = clone(loadedColoring);
      if (value.version < 5) {
        legacyRaw = raw;
        warning = '旧版配置已兼容载入；实验比例按 45/45/10、不记录染色解释。尚未改写本地缓存，下次保存前会备份原文。';
      }
    }
  } catch (error) {
    logStorage('read', error instanceof SyntaxError ? 'invalid' : 'unavailable');
    if (error instanceof SyntaxError) {
      usable = false;
      warning = '本地人群或实验配置损坏或版本不兼容：已载入示例，修改仅保留在当前会话。';
    } else unavailable();
  }
  function persist() {
    if (!usable) return;
    try {
      if (legacyRaw !== null && storage.getItem(BACKUP_KEY) === null) { storage.setItem(BACKUP_KEY, legacyRaw); logStorage('backup', 'ok'); }
      storage.setItem(STORAGE_KEY, JSON.stringify({ version: 5, audiences: definitions, experiments, bundles, coloring }));
      legacyRaw = null; warning = ''; logStorage('write', 'ok');
    }
    catch { unavailable(); logStorage('write', 'unavailable'); }
  }
  return {
    list: () => clone(definitions),
    experiments: () => clone(experiments),
    bundles: () => clone(bundles),
    experimentColoring: id => {
      if (!experiments.some(item => item.id === id)) throw new RangeError('实验不存在。');
      return clone(coloring[id] ?? {});
    },
    peekExperimentGroup(id, uid) {
      const experiment = experiments.find(item => item.id === id);
      if (!experiment) throw new RangeError('实验不存在。');
      if (!/^SIM-\d{4}$/.test(uid)) throw new RangeError('仅支持构造 SIM 用户标识。');
      return coloring[id]?.[uid] ?? assignGroup(uid, id, experiment.split);
    },
    assignExperimentGroup(id, uid) {
      const experiment = experiments.find(item => item.id === id);
      if (!experiment) throw new RangeError('实验不存在。');
      const part = this.peekExperimentGroup(id, uid);
      if (experiment.colored && !Object.hasOwn(coloring[id] ?? {}, uid)) {
        (coloring[id] ??= {})[uid] = part;
        persist();
      }
      return part;
    },
    updateExperimentSplit(id, split) {
      const experiment = experiments.find(item => item.id === id);
      if (!experiment) throw new RangeError('实验不存在。');
      const errors = validateSplit(split);
      if (errors.length) throw new RangeError(errors.join(' '));
      experiment.split = { a: split.a, b: split.b, control: split.control };
      persist();
      return clone(experiment);
    },
    catalog: () => buildReferenceCatalog(definitions, experiments, bundles),
    get warning() { return warning; },
    save(definition) {
      const errors = validateAudience(definition, experiments);
      if (errors.length) throw new RangeError(errors.join(' '));
      const id = definition.id || nextId(definitions, 'aud-custom');
      const index = definitions.findIndex(def => def.id === id);
      if (index < 0 && (definitions.some(item => groupIdOf(item.id) === groupIdOf(id)) || bundles.some(bundle => bundle.arms.some(arm => arm.groupId === groupIdOf(id))))) throw new RangeError('人群组 ID 已存在。');
      if (index >= 0 && bundles.some(bundle => bundle.arms.some(arm => arm.audienceId === id && arm.source !== definition.rules.source))) throw new RangeError('已发布包引用该人群，不能修改来源。');
      if (index >= 0 && bundles.some(bundle => bundle.arms.some(arm => arm.audienceId === id) && bundle.experimentId !== definition.experimentId)) throw new RangeError('已发布包引用该人群，不能修改实验引用。');
      if (index === -1 && definitions.length >= 100) throw new RangeError('演示最多保存 100 份人群定义。');
      const normalized = normalize(definition, id);
      if (index === -1) definitions.push(normalized); else definitions[index] = normalized;
      persist();
      return clone(normalized);
    },
    delete(id) {
      const index = definitions.findIndex(value => value.id === id);
      if (index < 0) throw new RangeError('人群不存在。');
      if (bundles.some(bundle => bundle.arms.some(arm => arm.audienceId === id)) || activities.some(activity => activityUsesGroup(activity, groupIdOf(id)))) throw new RangeError('人群组已被包或活动引用，不能删除。');
      definitions.splice(index, 1);
      persist();
    },
    saveExperiment(experiment) {
      const existing = experiments.find(value => value.id === experiment?.id);
      const errors = validateExperiment(existing ? { ...experiment, split: experiment.split ?? existing.split, colored: experiment.colored ?? existing.colored } : experiment);
      if (errors.length) throw new RangeError(errors.join(' '));
      const id = experiment.id || nextId(experiments, 'exp-custom');
      const index = experiments.findIndex(value => value.id === id);
      if (index === -1 && experiments.length >= 100) throw new RangeError('演示最多保存 100 份实验定义。');
      if (existing && experiment.colored !== undefined && experiment.colored !== existing.colored) throw new RangeError('染色策略创建后不可修改。');
      const normalized = { id, name: experiment.name.trim(), runtimeStatus: existing?.runtimeStatus ?? 'stopped', generation: existing?.generation ?? 0,
        split: clone(existing?.split ?? experiment.split), colored: existing?.colored ?? experiment.colored };
      if (index === -1) experiments.push(normalized); else experiments[index] = normalized;
      persist();
      return clone(normalized);
    },
    deleteExperiment(id) {
      const index = experiments.findIndex(value => value.id === id);
      if (index < 0) throw new RangeError('实验不存在。');
      if (definitions.some(value => value.experimentId === id) || bundles.some(value => value.experimentId === id)) throw new RangeError('实验已被人群或包引用，请先移除引用。');
      experiments.splice(index, 1);
      persist();
    },
    setExperimentStatus(id, status) {
      if (!['running', 'stopped'].includes(status)) throw new RangeError('实验运行态无效。');
      const experiment = experiments.find(item => item.id === id);
      if (!experiment) throw new RangeError('实验不存在。');
      if (experiment.runtimeStatus !== status) {
        experiment.generation++;
        experiment.runtimeStatus = status;
        persist();
      }
      return clone(experiment);
    },
    publishBundle(input) {
      const experiment = experiments.find(item => item.id === input?.experimentId);
      if (!experiment || experiment.runtimeStatus !== 'running') throw new RangeError('仅运行中的已保存实验可发布人群包。');
      if (bundles.length >= 100) throw new RangeError('演示最多发布 100 个人群包。');
      const id = input.id || nextId(bundles, 'bundle-custom');
      const bundle = { id, name: input.name, experimentId: experiment.id, generation: experiment.generation,
        arms: Array.isArray(input.arms) ? input.arms.map(arm => ({ part: arm?.part, groupId: arm?.groupId || `group-${id}-${arm?.part}`,
          name: arm?.name, audienceId: arm?.audienceId, source: definitions.find(item => item.id === arm?.audienceId)?.rules.source })) : input.arms };
      const errors = validateBundle(bundle, definitions, experiments, bundles);
      if (errors.length) throw new RangeError(errors.join(' '));
      bundles.push(bundle);
      persist();
      return clone(bundle);
    },
    deleteBundle(id) {
      const index = bundles.findIndex(item => item.id === id);
      if (index < 0) throw new RangeError('实验人群包不存在。');
      if (activities.some(activity => activity.audience?.type === 'bundle' && activity.audience.bundleId === id || activity.activityFlow?.entries?.some(entry => bundles[index].arms.some(arm => arm.groupId === entry.groupId)))) throw new RangeError('实验人群包已被活动引用，不能删除。');
      bundles.splice(index, 1);
      persist();
    },
  };
}
