import { For, createSignal, onMount } from 'solid-js'
import { Icon, type IconName } from './components/Icon'

export type Theme = 'light' | 'dark' | 'system'

const key = 'theme'

// Runs in <head> before the page paints, so a chosen theme never flashes
// the other one first.
export const themeScript = `try{var t=localStorage.getItem('${key}');if(t==='light'||t==='dark')document.documentElement.dataset.theme=t}catch(e){}`

const [theme, setThemeSignal] = createSignal<Theme>('system')
export { theme }

export function setTheme(t: Theme) {
  setThemeSignal(t)
  const root = document.documentElement
  if (t === 'system') delete root.dataset.theme
  else root.dataset.theme = t
  try {
    if (t === 'system') localStorage.removeItem(key)
    else localStorage.setItem(key, t)
  } catch {}
}

const options: { value: Theme; label: string; icon: IconName }[] = [
  { value: 'light', label: '浅色', icon: 'sun' },
  { value: 'dark', label: '深色', icon: 'moon' },
  { value: 'system', label: '跟随系统', icon: 'monitor' },
]

// The app's own choice of light, dark or following the system, as a
// segmented control.
export function ThemeSwitch(props: { labels?: boolean; class?: string }) {
  onMount(() => {
    const t = document.documentElement.dataset.theme
    if (t === 'light' || t === 'dark') setThemeSignal(t)
  })
  return (
    <div class={`segmented ${props.class ?? ''}`} role="radiogroup" aria-label="外观">
      <For each={options}>
        {(o) => (
          <button
            type="button"
            role="radio"
            aria-checked={theme() === o.value}
            aria-label={o.label}
            title={o.label}
            classList={{ on: theme() === o.value }}
            onClick={() => setTheme(o.value)}
          >
            <Icon name={o.icon} size={14} />
            {props.labels && <span>{o.label}</span>}
          </button>
        )}
      </For>
    </div>
  )
}
