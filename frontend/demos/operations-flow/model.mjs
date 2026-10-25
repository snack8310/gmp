import { MOCK_USERS, assignGroup } from './audience.mjs';
// Presentation-only fixtures. No domain assignment or delivery code is used.
export const TOTAL = 240;
export const CHANNELS = ['sms', 'push', 'whatsapp', 'ivr'];
export const SCENARIOS = {
  baseline: { name: '均衡触达', note: '多渠道并行，观察人群如何抵达终点。', delay: 0, failures: [10, 5, 8, 14] },
  pressure: { name: '回执延迟', note: 'WhatsApp 回执延迟，处理中不是失败。', delay: 12, failures: [10, 5, 8, 14] },
  recovery: { name: '优化示意', note: '缩短等待、降低模拟失败率，仅作展示，不代表真实提升。', delay: -2, failures: [5, 2, 4, 8] },
};
export function createScenario(key = 'baseline', users = MOCK_USERS, experimentId = null, extra = null) {
  const config = SCENARIOS[key];
  if (!Object.hasOwn(SCENARIOS, key)) throw new RangeError('Unknown scenario');
  const validId = id => typeof id === 'string' && /^[a-zA-Z0-9_-]{1,64}$/.test(id);
  if (experimentId !== null && !validId(experimentId)) throw new RangeError('Invalid inherited experiment');
  if (extra !== null && (!extra || !validId(extra.experimentId) || !(experimentId ? ['a', 'b'] : ['direct']).includes(extra.branch))) throw new RangeError('Invalid downstream experiment');
  const people = users.map((user, i) => {
    const channelIndex = i % 4;
    const channel = CHANNELS[channelIndex];
    const delay = channel === 'whatsapp' ? config.delay : Math.min(0, config.delay);
    const group = assignGroup(user.uid, experimentId);
    const extraGroup = extra && group === extra.branch ? `${assignGroup(user.uid, extra.experimentId)}2` : null;
    return {
      id: i, uid: user.uid, group, extraGroup, channel,
      start: i * 0.16, hold: 5 + delay,
      failed: ((Math.floor(i / 4) * 37 + channelIndex * 17) % 100) < config.failures[channelIndex],
    };
  });
  const finishAge = p => p.group === 'control' ? 4 : p.extraGroup === 'control2' ? 8 : (p.extraGroup ? 18 : 14) + p.hold;
  return { key, config, people, experimentId, extra: extra ? { ...extra } : null, duration: Math.max(1, ...people.map(p => p.start + finishAge(p))) };
}
export function personState(person, time) {
  const age = time - person.start;
  const before = offset => time < person.start + offset;
  if (before(0)) return { phase: 'queued', progress: 0 };
  if (before(4)) return { phase: 'entry', progress: age / 4 };
  if (person.group === 'control') return { phase: 'control', node: 'control', progress: 1 };
  if (person.extraGroup && before(6)) return { phase: 'extraEntry', progress: (age - 4) / 2 };
  if (person.extraGroup && before(8)) return { phase: 'extraSplit', progress: (age - 6) / 2 };
  if (person.extraGroup === 'control2') return { phase: 'control', node: 'control2', progress: 1 };
  const dispatchStart = person.extraGroup ? 8 : 4;
  const arrival = dispatchStart + 5;
  if (before(arrival)) return { phase: 'dispatch', progress: (age - dispatchStart) / 5 };
  if (before(arrival + person.hold)) return { phase: 'waiting', progress: (age - arrival) / person.hold };
  if (before(arrival + 5 + person.hold)) return { phase: 'result', progress: (age - arrival - person.hold) / 5 };
  return { phase: person.failed ? 'failed' : 'success', progress: 1 };
}
export function snapshot(scenario, time) {
  const now = Math.max(-0.001, Math.min(scenario.duration, time));
  const stateTime = time === 0 ? -0.001 : now;
  const counts = { total: scenario.people.length, queued: 0, active: 0, success: 0, failed: 0, control: 0 };
  const nodes = Object.fromEntries(['audience', 'direct', 'a', 'b', 'control', 'flowExperiment', 'a2', 'b2', 'control2', ...CHANNELS, 'success', 'failed'].map(k => [k, { arrived: 0, active: 0, success: 0, failed: 0 }]));
  nodes.audience.arrived = scenario.people.length;
  const states = scenario.people.map(person => {
    // Zero is an intentionally clean, paused start frame.
    const state = personState(person, stateTime);
    const terminal = ['success', 'failed', 'control'].includes(state.phase);
    counts[state.phase === 'queued' ? 'queued' : terminal ? state.phase : 'active']++;
    if (stateTime >= person.start + 4) nodes[person.group].arrived++;
    if (person.extraGroup && stateTime >= person.start + 6) nodes.flowExperiment.arrived++;
    if (person.extraGroup && stateTime >= person.start + 8) nodes[person.extraGroup].arrived++;
    const arrival = person.extraGroup ? 13 : 9;
    const noDelivery = person.group === 'control' || person.extraGroup === 'control2';
    if (!noDelivery && stateTime >= person.start + arrival) nodes[person.channel].arrived++;
    if (!terminal) {
      const owner = ['queued', 'entry'].includes(state.phase) ? 'audience'
        : state.phase === 'extraEntry' ? person.group
        : state.phase === 'extraSplit' ? 'flowExperiment'
        : state.phase === 'dispatch' ? person.extraGroup || person.group : person.channel;
      nodes[owner].active++;
    } else if (state.phase !== 'control') {
      nodes[state.phase].arrived++;
      nodes[person.channel][state.phase]++;
    }
    return { person, ...state };
  });
  return { ...counts, nodes, states, time: Math.max(0, now) };
}
// The single owned frame handle is testable without a browser or wall clock.
export function createClock({ duration, render, request, cancel }) {
  let frame = null, last = null, time = 0, speed = 1, playing = false;
  function tick(stamp) {
    frame = null;
    if (!playing) return;
    if (last !== null) time = Math.min(duration, time + Math.max(0, Math.min(stamp - last, 250)) / 1000 * speed);
    last = stamp;
    if (time >= duration) playing = false;
    render(time, playing);
    if (playing) frame = request(tick);
  }
  return {
    play() { if (playing || time >= duration) return; playing = true; last = null; render(time, true); frame = request(tick); },
    pause() { playing = false; if (frame !== null) cancel(frame); frame = null; last = null; render(time, false); },
    reset(nextDuration = duration) { this.pause(); duration = nextDuration; time = 0; render(time, false); },
    seek(value) { time = Math.max(0, Math.min(duration, Number(value) || 0)); last = null; if (time >= duration) this.pause(); else render(time, playing); },
    setSpeed(value) { if (![0.5, 1, 2, 4].includes(value)) throw new RangeError('Unsupported speed'); speed = value; },
    get time() { return time; }, get playing() { return playing; },
  };
}
