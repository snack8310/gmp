// Presentation fixtures only: these records do not describe real people.
export const STORAGE_KEY = 'gmp-audience-demo-v1';
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
  Object.freeze({ id: 'exp-recall', name: '召回策略实验' }),
  Object.freeze({ id: 'exp-onboarding', name: '新客体验实验' }),
]);
const defaultRules = { country: 'all', source: 'app', lifecycle: 'all', minInactive: 0, maxInactive: 365 };
export const DEFAULT_AUDIENCES = Object.freeze([
  { id: 'aud-app', name: 'App 全体模拟人群', rules: { ...defaultRules } },
  { id: 'aud-mx-dormant', name: '墨西哥 · 沉睡召回', rules: { ...defaultRules, country: 'MX', lifecycle: 'dormant', minInactive: 30 } },
  { id: 'aud-whitelist-new', name: '白名单 · 新客体验', rules: { ...defaultRules, source: 'whitelist', lifecycle: 'new' } },
].map(value => Object.freeze({ ...value, experimentId: null, rules: Object.freeze(value.rules) })));
const clone = value => JSON.parse(JSON.stringify(value));
const owns = (object, key) => typeof key === 'string' && Object.hasOwn(object, key);
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
const validId = id => typeof id === 'string' && /^[a-zA-Z0-9_-]{1,64}$/.test(id);
export function validateExperiment(experiment) {
  if (!experiment || typeof experiment !== 'object' || Array.isArray(experiment)) return ['请填写实验定义。'];
  const errors = [];
  if (typeof experiment.name !== 'string' || !experiment.name.trim() || experiment.name.trim().length > 48) errors.push('实验名称须为 1 至 48 个字符。');
  if (experiment.id !== undefined && experiment.id !== '' && !validId(experiment.id)) errors.push('实验标识无效。');
  return errors;
}
export function validateAudience(definition, experiments) {
  if (!definition || typeof definition !== 'object' || Array.isArray(definition)) return ['请填写人群定义。'];
  const errors = validateRules(definition.rules);
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
export function assignGroup(uid, experimentId = null) {
  if (experimentId === null) return 'direct';
  if (!validId(experimentId)) throw new RangeError('实验标识无效。');
  if (typeof uid !== 'string' || !uid) throw new RangeError('模拟用户标识不能为空。');
  // Fixed presentation-only salt/version, experiment identity, independent from names, ordering and filters.
  let hash = 2166136261;
  for (const character of `gmp-demo-split-v1:${experimentId}:${uid}`) hash = Math.imul(hash ^ character.charCodeAt(0), 16777619) >>> 0;
  const bucket = hash % 10000;
  return bucket < 4500 ? 'a' : bucket < 9000 ? 'b' : 'control';
}
function normalize(definition, id) {
  const { country, source, lifecycle, minInactive, maxInactive } = definition.rules;
  return { id, name: definition.name.trim(), experimentId: definition.experimentId ?? null, rules: { country, source, lifecycle, minInactive, maxInactive } };
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
export function createAudienceStore(storage) {
  let definitions = clone(DEFAULT_AUDIENCES), experiments = clone(DEFAULT_EXPERIMENTS), warning = '', usable = true;
  const unavailable = () => { usable = false; warning = '浏览器存储不可用：修改仅保留在当前会话，刷新后会丢失。'; };
  try {
    if (!storage || typeof storage.getItem !== 'function' || typeof storage.setItem !== 'function') throw new Error('Storage unavailable');
    const raw = storage.getItem(STORAGE_KEY);
    if (raw !== null) {
      const value = JSON.parse(raw);
      if (!value || ![1, 2].includes(value.version)) throw new SyntaxError('Unknown stored version');
      const loadedExperiments = value.version === 1 ? clone(DEFAULT_EXPERIMENTS) : value.experiments;
      if (!validCollection(loadedExperiments, validateExperiment)
        || !validCollection(value.audiences, def => validateAudience(def, loadedExperiments))
        || (value.version === 1 && value.audiences.some(def => def.experimentId != null))) throw new SyntaxError('Invalid stored definitions');
      // Commit both collections only after all references have been validated.
      definitions = value.audiences.map(def => normalize(def, def.id));
      experiments = loadedExperiments.map(experiment => ({ id: experiment.id, name: experiment.name.trim() }));
    }
  } catch (error) {
    if (error instanceof SyntaxError) {
      usable = false;
      warning = '本地人群或实验配置损坏或版本不兼容：已载入示例，修改仅保留在当前会话。';
    } else unavailable();
  }
  function persist() {
    if (!usable) return;
    try { storage.setItem(STORAGE_KEY, JSON.stringify({ version: 2, audiences: definitions, experiments })); }
    catch { unavailable(); }
  }
  return {
    list: () => clone(definitions),
    experiments: () => clone(experiments),
    get warning() { return warning; },
    save(definition) {
      const errors = validateAudience(definition, experiments);
      if (errors.length) throw new RangeError(errors.join(' '));
      const id = definition.id || nextId(definitions, 'aud-custom');
      const index = definitions.findIndex(def => def.id === id);
      if (index === -1 && definitions.length >= 100) throw new RangeError('演示最多保存 100 份人群定义。');
      const normalized = normalize(definition, id);
      if (index === -1) definitions.push(normalized); else definitions[index] = normalized;
      persist();
      return clone(normalized);
    },
    saveExperiment(experiment) {
      const errors = validateExperiment(experiment);
      if (errors.length) throw new RangeError(errors.join(' '));
      const id = experiment.id || nextId(experiments, 'exp-custom');
      const index = experiments.findIndex(value => value.id === id);
      if (index === -1 && experiments.length >= 100) throw new RangeError('演示最多保存 100 份实验定义。');
      const normalized = { id, name: experiment.name.trim() };
      if (index === -1) experiments.push(normalized); else experiments[index] = normalized;
      persist();
      return clone(normalized);
    },
  };
}
