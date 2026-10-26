import { createSignal } from 'solid-js'

// The song the window in the hero plays.
export const track = {
  title: '春日影',
  artist: 'MyGO!!!!!',
  src: '/haruhikage.m4a',
  duration: 281,
}

const [playing, setPlaying] = createSignal(false)
const [position, setPosition] = createSignal(62)
const [liked, setLiked] = createSignal(false)
export { playing, position, liked, setLiked }

// Made on the first play, so the page doesn't download the song until then.
let audio: HTMLAudioElement | undefined
let frame = 0

function element() {
  if (audio) return audio
  audio = new Audio(track.src)
  audio.loop = true
  audio.currentTime = position()
  // The browser can pause it on its own, from the media keys or another tab.
  audio.addEventListener('pause', () => {
    setPlaying(false)
    cancelAnimationFrame(frame)
  })
  return audio
}

// timeupdate only fires a few times a second, too coarse for the progress bar.
function tick() {
  if (audio) setPosition(audio.currentTime)
  frame = requestAnimationFrame(tick)
}

export function play() {
  if (playing()) return
  setPlaying(true)
  element()
    .play()
    .then(() => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(tick)
    })
    .catch(() => setPlaying(false))
}

export function pause() {
  audio?.pause()
}

export function toggle() {
  playing() ? pause() : play()
}

export function seek(fraction: number) {
  const p = Math.max(0, Math.min(1, fraction)) * track.duration
  setPosition(p)
  if (audio) audio.currentTime = p
}

export function clock(seconds: number) {
  const s = Math.floor(seconds)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}
