const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const timers = new Map();
const instances = [];
const intervals = new Map();
const events = [];
let rejectEnd = false;
let timerID = 0;
class Hls {
  static Events = { ERROR: 'error' };
  static ErrorTypes = {};
  constructor(config) { this.config = config; this.starts = []; instances.push(this); }
  on() {}
  loadSource() {}
  attachMedia(audio) { this.audio = audio; }
  stopLoad() { this.stopped = true; }
  startLoad(position) { this.starts.push(position); this.stopped = false; }
  destroy() { this.destroyed = true; }
}
const context = vm.createContext({
  Hls, window: {},
  AbortSignal,
  fetch: (url, options) => {
    const event = JSON.parse(options.body);
    events.push(event);
    return Promise.resolve({ ok: !(rejectEnd && event.type === 'ended'), status: 503 });
  },
  setInterval: (fn) => { intervals.set(++timerID, fn); return timerID; },
  clearInterval: (id) => intervals.delete(id),
  setTimeout: (fn) => { timers.set(++timerID, fn); return timerID; },
  clearTimeout: (id) => timers.delete(id),
  document: { body: { append() {} }, createElement: () => ({
    currentTime: 0, duration: 250, readyState: 1, paused: false, seeking: false,
    buffered: { length: 1, start: () => 220, end: () => 250 },
    handlers: {},
    addEventListener(name, fn) { this.handlers[name] = fn; },
    play() { this.paused = false; return Promise.resolve(); },
    pause() { this.paused = true; }, remove() { this.removed = true; },
  }) },
});
const source = fs.readFileSync(__dirname + '/player.html', 'utf8').match(/<script>([\s\S]*?)<\/script>/)[1];
vm.runInContext(source, context);
context.vinyl = context.window.vinyl;
const player = context.vinyl;
// A short pause reuses the pipeline; a longer one stops loading and resumes at its position.
player.load(1);
const first = instances[0];
player.play(1);
player.pause(1);
player.play(1);
assert.equal(timers.size, 0);
assert.equal(first.starts.length, 0);
player.pause(1);
const expired = [...timers.values()][0];
timers.clear();
expired();
assert.equal(first.stopped, true);
first.audio.currentTime = 23;
player.play(1);
assert.deepEqual(first.starts, [23]);
assert.equal(first.config.backBufferLength, 30);
// Switching tracks cancels old pause callbacks and never stops the new track.
player.pause(1);
const stale = [...timers.values()][0];
player.load(2);
stale();
assert.equal(timers.size, 0);
assert.equal(first.destroyed, true);
assert.equal(first.audio.removed, true);
assert.equal(instances[1].stopped, undefined);
player.pause(2);
player.unload(2);
assert.equal(timers.size, 0);
async function checkEndings() {
  // A fully buffered tail that stops advancing used to wait forever for ended.
  player.load(3);
  player.play(3);
  const audio = instances.at(-1).audio;
  const tick = () => { for (const fn of intervals.values()) fn(); };
  const ended = () => events.filter(event => event.type === 'ended');
  audio.handlers.playing();
  audio.currentTime = 249.9;
  tick();
  for (let count = 0; count < 3; count++) tick();
  assert.equal(ended().length, 0);
  tick();
  assert.equal(ended().length, 1);
  assert.equal(ended()[0].msg, 'tail stalled');
  audio.handlers.ended();
  tick();
  assert.equal(ended().length, 1);
  await new Promise(setImmediate);

  // A repeat uses a new sequence; failed delivery retries the same sequence.
  player.seek(3, 0);
  rejectEnd = true;
  audio.handlers.ended();
  await new Promise(setImmediate);
  assert.equal(timers.size, 1);
  const retry = [...timers.values()][0];
  timers.clear();
  rejectEnd = false;
  await retry();
  assert.equal(ended().length, 3);
  assert.deepEqual(ended().map(event => event.seq), [1, 2, 2]);
  player.unload(3);
  assert.equal(intervals.size, 0);
  await retry();
  assert.equal(ended().length, 3);

  // Pauses, seeks, incomplete buffers and stalls away from the tail must not skip.
  for (const mode of ['pause', 'seek', 'buffer', 'middle', 'moving', 'unheard']) {
    player.load(4);
    player.play(4);
    const current = instances.at(-1).audio;
    current.currentTime = 249.9;
    if (mode !== 'unheard') current.handlers.playing();
    if (mode === 'pause') player.pause(4);
    if (mode === 'seek') current.seeking = true;
    if (mode === 'buffer') current.buffered.end = () => 249.5;
    if (mode === 'middle') current.currentTime = 100;
    if (mode === 'moving') current.currentTime = 249.76;
    for (let count = 0; count < 6; count++) {
      if (mode === 'moving') current.currentTime += 0.01;
      tick();
    }
    assert.equal(ended().length, 3, mode);
    player.unload(4);
  }
  assert.equal(timers.size, 0);
  assert.equal(intervals.size, 0);

  // A page without Web Audio has no analyser, so it sends no spectrum: the
  // app draws its bars the way it always did rather than waiting on one.
  assert.equal(events.filter((e) => e.type === 'levels').length, 0,
    'a page with no analyser sent a spectrum');
  player.load(5);
  player.play(5);
  const quiet = instances.at(-1).audio;
  quiet.handlers.playing();
  await new Promise((ok) => setImmediate(ok));
  assert.equal(events.filter((e) => e.type === 'levels').length, 0,
    'a page with no analyser sent a spectrum after playing');
  player.unload(5);

  console.log('player lifecycle and ending JS tests passed');
}
checkEndings().catch(error => { console.error(error); process.exitCode = 1; });
