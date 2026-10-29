const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const timers = new Map();
const instances = [];
const intervals = new Map();
const events = [];
const elements = [];
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
const session = { handlers: {}, setActionHandler(action, fn) { this.handlers[action] = fn; } };
const context = vm.createContext({
  Hls, window: {}, navigator: { mediaSession: session },
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
  document: { body: { append() {} }, createElement: () => {
    const element = {
      currentTime: 0, duration: 250, readyState: 1, paused: false, seeking: false,
      buffered: { length: 1, start: () => 220, end: () => 250 },
      handlers: {},
      addEventListener(name, fn) { this.handlers[name] = fn; },
      play() { this.paused = false; return Promise.resolve(); },
      pause() { this.paused = true; }, remove() { this.removed = true; },
      removeAttribute(name) { delete this[name]; }, load() { this.loads = (this.loads || 0) + 1; },
    };
    elements.push(element);
    return element;
  } },
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
// WebKit pauses and plays the element itself for the system (the media
// keys, Control Center, apps such as Mami): the page tells Vinyl, with how
// many times Vinyl had asked, and loads again what a long pause stopped.
// Vinyl's own pause, and the one at the end, are Vinyl's.
player.load(7);
const sys = elements.at(-1);
const sysHls = instances.at(-1);
const reported = () => events.filter(event => event.gen === 7 && event.type.startsWith('system-')).map(event => event.type + '@' + event.seq);
player.play(7, 1);
sys.paused = true;
sys.handlers.pause();
assert.deepEqual(reported(), ['system-pause@1']);
assert.equal(timers.size, 1);
const resting = [...timers.values()][0];
timers.clear();
resting();
assert.equal(sysHls.stopped, true);
sys.currentTime = 40;
sys.paused = false;
sys.handlers.play();
assert.deepEqual(reported(), ['system-pause@1', 'system-play@1']);
assert.deepEqual(sysHls.starts, [40]);
player.pause(7, 2);
sys.handlers.pause();
player.play(7, 3);
sys.handlers.play();
sys.ended = true;
sys.paused = true;
sys.handlers.pause();
assert.deepEqual(reported(), ['system-pause@1', 'system-play@1']);
// The skips it passes the page skip.
session.handlers.nexttrack();
session.handlers.previoustrack();
assert.deepEqual(reported(), ['system-pause@1', 'system-play@1', 'system-next@0', 'system-previous@0']);
player.unload(7);
// WebKit pauses an element taken out of the page: that is not the system's.
sys.ended = false;
sys.handlers.pause();
assert.equal(reported().length, 4);
assert.equal(timers.size, 0);
session.handlers.nexttrack();
assert.equal(events.filter(event => event.type === 'system-next').length, 1);

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

  // A file, an episode's where its podcast keeps it, plays as it is: the
  // element loads it, no hls.js, and it says how long it is once loaded.
  const made = instances.length;
  player.load(5, 'file');
  const file = elements.at(-1);
  assert.equal(instances.length, made);
  assert.equal(file.src, '/m/5/0');
  file.duration = 754.5;
  file.handlers.loadedmetadata();
  assert.deepEqual(events.filter(event => event.gen === 5 && event.type === 'duration').map(event => event.t), [754.5]);
  player.play(5);
  assert.equal(file.paused, false);
  player.unload(5);
  assert.equal(file.removed, true);
  assert.equal(file.src, undefined);
  // A song's manifest said already.
  player.load(6);
  elements.at(-1).handlers.loadedmetadata();
  assert.equal(events.filter(event => event.gen === 6 && event.type === 'duration').length, 0);
  player.unload(6);
  assert.equal(intervals.size, 0);
  console.log('player lifecycle and ending JS tests passed');
}
checkEndings().catch(error => { console.error(error); process.exitCode = 1; });
