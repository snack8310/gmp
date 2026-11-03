import test from 'node:test';
import assert from 'node:assert/strict';
import { mountExperiments } from './experiment-ui.mjs';
import { createAudienceStore } from './audience.mjs';

test('experiment detail escapes the editable name in its heading', () => {
  const root = { innerHTML: '', addEventListener() {}, querySelector() { return null; } };
  const previous = globalThis.document;
  globalThis.document = { getElementById: () => root };
  try {
    const experiment = { id: 'exp-x', name: '<img src=x onerror=alert(1)>', runtimeStatus: 'stopped', generation: 0,
      split: { a: 45, b: 45, control: 10 }, colored: false };
    const store = { experiments: () => [experiment], bundles: () => [], experimentColoring: () => ({}), peekExperimentGroup: () => 'a', warning: '' };
    mountExperiments({ store }).render({ assetView: 'detail', experimentId: experiment.id, returnTo: null });
    assert.match(root.innerHTML, /<h2>&lt;img src=x onerror=alert\(1\)&gt;<\/h2>/);
    assert.doesNotMatch(root.innerHTML, /<h2><img/);
    assert.equal((root.innerHTML.match(/<h2>/g) ?? []).length, 1);
    assert.equal((root.innerHTML.match(/class="campaign-detail-block experiment-panel"/g) ?? []).length, 4);
    assert.doesNotMatch(root.innerHTML, /experiment-detail-title/);
  } finally { globalThis.document = previous; }
});

test('detail explains which Core experiment controls are not connected', () => {
  const root = { innerHTML: '', addEventListener() {}, querySelector() { return null; } };
  const previous = globalThis.document;
  globalThis.document = { getElementById: () => root };
  try {
    mountExperiments({ store: createAudienceStore({ getItem: () => null, setItem() {} }) }).render({ assetView: 'detail', experimentId: 'exp-recall', returnTo: null });
    for (const label of ['白名单', '算法版本', '盐', '实验指标', '放量配置']) assert.match(root.innerHTML, new RegExp(label));
  } finally { globalThis.document = previous; }
});

test('failed status write reports session-only state without success', () => {
  const listeners = {}, root = { innerHTML: '', addEventListener(type, handler) { listeners[type] = handler; },
    querySelector() { return null; }, contains() { return true; } };
  const previous = globalThis.document;
  globalThis.document = { getElementById: () => root };
  try {
    const store = createAudienceStore({ getItem: () => null, setItem() { throw Error('quota'); } });
    mountExperiments({ store }).render({ assetView: 'detail', experimentId: 'exp-recall', returnTo: null });
    listeners.click({ target: { closest: () => ({ dataset: { action: 'toggle' } }) } });
    assert.match(root.innerHTML, /仅保留在当前会话/);
    assert.doesNotMatch(root.innerHTML, /已更新运行状态/);
  } finally { globalThis.document = previous; }
});
