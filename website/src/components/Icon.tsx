import type { JSX } from 'solid-js'

// Line icons in the style of the app's own: 24 units, round joins. Each
// is made where it shows: a node can be in one place only.
const paths = {
  search: () => <><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></>,
  home: () => <path d="M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" />,
  calendar: () => <><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M16 3v4M8 3v4M3 10h18" /></>,
  radio: () => <><circle cx="12" cy="12" r="2" /><path d="M16.2 7.8a6 6 0 0 1 0 8.4M7.8 16.2a6 6 0 0 1 0-8.4M19.1 4.9a10 10 0 0 1 0 14.2M4.9 19.1a10 10 0 0 1 0-14.2" /></>,
  chart: () => <path d="M3 3v18h18M8 17v-5M12 17V8M16 17v-3M20 17v-7" />,
  heart: () => <path d="M19.5 13.6 12 21l-7.5-7.4A5 5 0 0 1 12 6.5a5 5 0 0 1 7.5 7.1z" />,
  shuffle: () => <path d="M2 18h1.4a4 4 0 0 0 3.3-1.7l6.6-8.6A4 4 0 0 1 16.6 6H22M18 2l4 4-4 4M2 6h1.9a4 4 0 0 1 3.1 1.5M22 18h-5.5a4 4 0 0 1-3.1-1.5M18 14l4 4-4 4" />,
  repeat: () => <path d="m17 2 4 4-4 4M3 11v-1a4 4 0 0 1 4-4h14M7 22l-4-4 4-4M21 13v1a4 4 0 0 1-4 4H3" />,
  lyrics: () => <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2zM8 9h8M8 13h5" />,
  queue: () => <path d="M3 6h13M3 12h9M3 18h9M16 18a2 2 0 1 0 4 0 2 2 0 0 0-4 0zm4 0V9l3-1" />,
  volume: () => <path d="M11 5 6 9H2v6h4l5 4zM15.5 8.5a5 5 0 0 1 0 7M19 5a10 10 0 0 1 0 14" />,
  music: () => <path d="M9 18V5l12-2v13M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm12-2a3 3 0 1 1-6 0 3 3 0 0 1 6 0z" />,
  sun: () => <><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></>,
  moon: () => <path d="M20.5 14.5A8.5 8.5 0 1 1 9.5 3.5a7 7 0 0 0 11 11z" />,
  monitor: () => <><rect x="2" y="4" width="20" height="13" rx="2" /><path d="M8 21h8M12 17v4" /></>,
  download: () => <path d="M12 3v12M7 10l5 5 5-5M5 21h14" />,
  arrow: () => <path d="M5 12h14M13 6l6 6-6 6" />,
  qr: () => <path d="M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2v2h-2zM18 14h2M14 18v2M18 18h2v2" />,
  wave: () => <path d="M2 12h2M6 8v8M10 5v14M14 8v8M18 10v4M22 12h-2" />,
  history: () => <path d="M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5M12 7v5l3 2" />,
  disk: () => <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6" /></>,
  sparkle: () => <path d="M12 3l1.9 5.6L19.5 10l-5.6 1.9L12 17.5l-1.9-5.6L4.5 10l5.6-1.4zM19 16l.8 2.2L22 19l-2.2.8L19 22l-.8-2.2L16 19l2.2-.8z" />,
  notch: () => <><rect x="2" y="4" width="20" height="16" rx="3" /><path d="M9 4v1.5a1.5 1.5 0 0 0 1.5 1.5h3A1.5 1.5 0 0 0 15 5.5V4" /></>,
} satisfies Record<string, () => JSX.Element>

export type IconName = keyof typeof paths

export function Icon(props: { name: IconName; size?: number; class?: string; stroke?: number }) {
  return (
    <svg
      class={props.class}
      width={props.size ?? 18}
      height={props.size ?? 18}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width={props.stroke ?? 1.8}
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      {paths[props.name]()}
    </svg>
  )
}

// Filled shapes for the transport, as solid as the app's.
const transport = {
  play: 'M7 4.5v15a1 1 0 0 0 1.5.86l12.5-7.5a1 1 0 0 0 0-1.72L8.5 3.64A1 1 0 0 0 7 4.5z',
  pause: 'M6.5 4.5h3v15h-3zM14.5 4.5h3v15h-3z',
  prev: 'M11 6.5v11L3 12zM20 6.5v11L12 12z',
  next: 'M13 6.5v11l8-5.5zM4 6.5v11l8-5.5z',
}

export function Transport(props: { name: keyof typeof transport; size?: number }) {
  return (
    <svg width={props.size ?? 18} height={props.size ?? 18} viewBox="0 0 24 24" aria-hidden="true">
      <path
        d={transport[props.name]}
        fill="currentColor"
        stroke="currentColor"
        stroke-width="1"
        stroke-linejoin="round"
      />
    </svg>
  )
}

export function GitHubMark(props: { size?: number }) {
  return (
    <svg width={props.size ?? 18} height={props.size ?? 18} viewBox="0 0 24 24" aria-hidden="true">
      <path
        fill="currentColor"
        d="M12 .5a11.5 11.5 0 0 0-3.6 22.4c.6.1.8-.3.8-.6v-2c-3.2.7-3.9-1.5-3.9-1.5-.5-1.3-1.3-1.7-1.3-1.7-1-.7.1-.7.1-.7 1.2.1 1.8 1.2 1.8 1.2 1 1.8 2.8 1.3 3.5 1 .1-.8.4-1.3.7-1.6-2.6-.3-5.3-1.3-5.3-5.7 0-1.3.5-2.3 1.2-3.1-.1-.3-.5-1.5.1-3.1 0 0 1-.3 3.2 1.2a11 11 0 0 1 5.8 0c2.2-1.5 3.2-1.2 3.2-1.2.6 1.6.2 2.8.1 3.1.8.8 1.2 1.8 1.2 3.1 0 4.4-2.7 5.4-5.3 5.7.4.4.8 1.1.8 2.2v3.3c0 .3.2.7.8.6A11.5 11.5 0 0 0 12 .5z"
      />
    </svg>
  )
}
