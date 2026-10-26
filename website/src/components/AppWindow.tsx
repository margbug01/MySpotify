import { For, createSignal, onMount } from 'solid-js'
import { Icon, Transport, type IconName } from './Icon'
import { Cover } from './Cover'
import { Like } from './Like'
import { clock, playing, position, seek, toggle, track } from '../player'

const weekdays = ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六']

// The greeting over the home page, as the app words it by the hour.
function greeting(h: number) {
  if (h < 5) return '夜深了'
  if (h < 11) return '早上好'
  if (h < 13) return '中午好'
  if (h < 18) return '下午好'
  if (h < 23) return '晚上好'
  return '夜深了'
}

const nav: { icon: IconName; label: string }[] = [
  { icon: 'home', label: '为你推荐' },
  { icon: 'calendar', label: '每日推荐' },
  { icon: 'radio', label: '私人 FM' },
  { icon: 'chart', label: '排行榜' },
]

const playlists = ['我喜欢的音乐', 'BanG Dream! 精选', '新番 OP 合集', '深夜动画 ED']

const picks = [
  { name: 'MyGO!!!!! 全曲收录', hue: 230 },
  { name: '燃向动漫 OP', hue: 20 },
  { name: '治愈系动画 ED', hue: 160 },
  { name: 'Ave Mujica 精选', hue: 300 },
  { name: '经典动画主题曲', hue: 45 },
]

const charts = [
  { name: '飙升榜', hue: 18, songs: ['春日影', 'アイドル', '青のすみか'] },
  { name: '新歌榜', hue: 205, songs: ['迷星叫', '碧天伴走', '壱雫空'] },
  { name: 'ACG 榜', hue: 150, songs: ['紅蓮華', 'unravel', '残酷な天使のテーゼ'] },
]

// The app's main window, built in HTML: the sidebar, the home page with
// its daily card and charts, and the player bar. It plays the page's song.
export function AppWindow() {
  // Rendered at build time, the date is fixed; the visitor's own fills in.
  const [now, setNow] = createSignal(new Date(2026, 9, 26, 23, 30))
  onMount(() => setNow(new Date()))
  const date = () => `${now().getMonth() + 1}月${now().getDate()}日 ${weekdays[now().getDay()]}`

  return (
    <div class="window" classList={{ playing: playing() }}>
      <aside class="win-side">
        <div class="lights" aria-hidden="true">
          <i />
          <i />
          <i />
        </div>
        <div class="win-search">
          <Icon name="search" size={13} />
          搜索歌曲、歌手
        </div>
        <For each={nav}>
          {(n, i) => (
            <div class="win-nav" classList={{ on: i() === 0 }}>
              <Icon name={n.icon} size={16} />
              {n.label}
            </div>
          )}
        </For>
        <div class="win-heading">我的歌单</div>
        <For each={playlists}>
          {(p, i) => (
            <div class="win-list">
              <Cover hue={(i() * 70 + 340) % 360} size={22} radius={5} />
              {p}
            </div>
          )}
        </For>
        <div class="win-side-foot">
          <div class="win-now">
            <Cover hue={18} size={36} radius={8} />
            <div>
              <b>{track.title}</b>
              <span>{track.artist}</span>
            </div>
          </div>
        </div>
      </aside>

      <section class="win-main">
        <div class="win-page">
          <div class="win-date">{date()}</div>
          <div class="win-greet">{greeting(now().getHours())}</div>

          <div class="daily">
            <div class="daily-day" aria-hidden="true">
              {now().getDate()}
            </div>
            <div class="daily-text">
              <div class="daily-note">{date()} · 每天 6:00 更新</div>
              <div class="daily-title">每日推荐</div>
              <div class="daily-artists">MyGO!!!!!、YOASOBI、LiSA、Ave Mujica 等</div>
              <button type="button" class="daily-play" onClick={toggle}>
                <span class="bars" aria-hidden="true">
                  <i />
                  <i />
                  <i />
                </span>
                {playing() ? '正在播放' : '播放'}
              </button>
            </div>
            <div class="daily-stack" aria-hidden="true">
              <Cover hue={205} size={112} radius={14} class="sleeve s3" />
              <Cover hue={150} size={124} radius={14} class="sleeve s2" />
              <div class="daily-record">
                <div class="vinyl" />
              </div>
              <Cover hue={18} size={136} radius={14} class="sleeve s1" />
            </div>
          </div>

          <div class="win-section">官方榜单</div>
          <div class="win-charts">
            <For each={charts}>
              {(c) => (
                <div class="chart">
                  <Cover hue={c.hue} size={64} radius={10} label={c.name} />
                  <ol>
                    <For each={c.songs}>{(s) => <li>{s}</li>}</For>
                  </ol>
                </div>
              )}
            </For>
          </div>

          <div class="win-section">推荐歌单</div>
          <div class="win-picks">
            <For each={picks}>
              {(p) => (
                <div class="pick">
                  <Cover hue={p.hue} size={120} radius={12} class="pick-cover" />
                  <span>{p.name}</span>
                </div>
              )}
            </For>
          </div>
        </div>

        <div class="win-bar">
          <div class="bar-song">
            <Cover hue={18} size={44} radius={8} />
            <div>
              <b>{track.title}</b>
              <span>{track.artist}</span>
            </div>
            <Like />
          </div>
          <div class="bar-center">
            <div class="bar-controls">
              <span class="ctl" aria-hidden="true">
                <Icon name="shuffle" size={14} />
              </span>
              <span class="ctl" aria-hidden="true">
                <Transport name="prev" size={16} />
              </span>
              <button type="button" class="ctl play" aria-label={playing() ? '暂停' : '播放'} onClick={toggle}>
                <Transport name={playing() ? 'pause' : 'play'} size={20} />
              </button>
              <span class="ctl" aria-hidden="true">
                <Transport name="next" size={16} />
              </span>
              <span class="ctl" aria-hidden="true">
                <Icon name="repeat" size={14} />
              </span>
            </div>
            <div class="bar-progress">
              <span>{clock(position())}</span>
              <Progress />
              <span>{clock(track.duration)}</span>
            </div>
          </div>
          <div class="bar-end" aria-hidden="true">
            <Icon name="lyrics" size={16} />
            <Icon name="queue" size={16} />
            <Icon name="volume" size={16} />
          </div>
        </div>
      </section>
    </div>
  )
}

// The progress bar: click or drag to seek.
function Progress() {
  let el!: HTMLDivElement
  const at = (e: PointerEvent) => {
    const r = el.getBoundingClientRect()
    seek((e.clientX - r.left) / r.width)
  }
  return (
    <div
      ref={el}
      class="progress"
      role="slider"
      aria-label="播放进度"
      aria-valuemin={0}
      aria-valuemax={Math.round(track.duration)}
      aria-valuenow={Math.round(position())}
      aria-valuetext={clock(position())}
      tabindex="0"
      onPointerDown={(e) => {
        el.setPointerCapture(e.pointerId)
        at(e)
      }}
      onPointerMove={(e) => el.hasPointerCapture(e.pointerId) && at(e)}
      onKeyDown={(e) => {
        const step = e.key === 'ArrowRight' ? 5 : e.key === 'ArrowLeft' ? -5 : 0
        if (step) {
          e.preventDefault()
          seek((position() + step) / track.duration)
        }
      }}
    >
      <div class="progress-fill" style={{ width: `${(position() / track.duration) * 100}%` }} />
    </div>
  )
}
