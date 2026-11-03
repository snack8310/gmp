import test from 'node:test';
import assert from 'node:assert/strict';
import { mountExperiments } from './experiment-ui.mjs';

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
