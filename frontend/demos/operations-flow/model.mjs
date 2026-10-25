// Presentation-only fixtures. No domain assignment or delivery code is used.
export const TOTAL = 240;
export const CHANNELS = ['sms', 'push', 'whatsapp', 'ivr'];
export const SCENARIOS = {
  baseline: { name: '均衡触达', note: '多渠道并行，观察人群如何抵达终点。', delay: 0, failures: [10, 5, 8, 14] },
  pressure: { name: '回执延迟', note: 'WhatsApp 回执延迟，处理中不是失败。', delay: 12, failures: [10, 5, 8, 14] },
  recovery: { name: '优化示意', note: '缩短等待、降低模拟失败率，仅作展示，不代表真实提升。', delay: -2, failures: [5, 2, 4, 8] },
};
export function createScenario(key = 'baseline') {
  const config = SCENARIOS[key];
  if (!config) throw new RangeError('Unknown scenario');
  const people = Array.from({ length: TOTAL }, (_, i) => {
    const channelIndex = i % 4;
    const channel = CHANNELS[channelIndex];
    const delay = channel === 'whatsapp' ? config.delay : Math.min(0, config.delay);
    return {
      id: i, group: i % 8 < 4 ? 'a' : 'b', channel,
      start: i * 0.16, hold: 5 + delay,
      failed: ((Math.floor(i / 4) * 37 + channelIndex * 17) % 100) < config.failures[channelIndex],
    };
  });
  return { key, config, people, duration: Math.max(...people.map(p => p.start + 17 + p.hold)) };
}
export function personState(person, time) {
  const age = time - person.start;
  if (age < 0) return { phase: 'queued', progress: 0 };
  if (age < 3) return { phase: 'entry', progress: age / 3 };
  if (age < 7) return { phase: 'split', progress: (age - 3) / 4 };
  if (age < 12) return { phase: 'dispatch', progress: (age - 7) / 5 };
  if (age < 12 + person.hold) return { phase: 'waiting', progress: (age - 12) / person.hold };
  if (age < 17 + person.hold) return { phase: 'result', progress: (age - 12 - person.hold) / 5 };
  return { phase: person.failed ? 'failed' : 'success', progress: 1 };
}
export function snapshot(scenario, time) {
  const now = Math.max(-0.001, Math.min(scenario.duration, time));
  const counts = { total: TOTAL, queued: 0, active: 0, success: 0, failed: 0 };
  const nodes = Object.fromEntries(['audience', 'strategy', 'a', 'b', ...CHANNELS, 'success', 'failed'].map(k => [k, { arrived: 0, active: 0, success: 0, failed: 0 }]));
  nodes.audience.arrived = TOTAL;
  const states = scenario.people.map(person => {
    // Zero is an intentionally clean, paused start frame.
    const state = personState(person, time === 0 ? -0.001 : now);
    const age = time - person.start;
    const terminal = state.phase === 'success' || state.phase === 'failed';
    counts[state.phase === 'queued' ? 'queued' : terminal ? state.phase : 'active']++;
    if (state.phase === 'queued') nodes.audience.active++;
    else {
      if (age >= 3) nodes.strategy.arrived++;
      if (age >= 7) nodes[person.group].arrived++;
      if (age >= 12) nodes[person.channel].arrived++;
      const owner = state.phase === 'entry' ? 'audience' : state.phase === 'split' ? 'strategy' : state.phase === 'dispatch' ? person.group : person.channel;
      if (!terminal) nodes[owner].active++;
      if (terminal) {
        nodes[state.phase].arrived++;
        nodes[person.channel][state.phase]++;
      }
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
