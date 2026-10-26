import { For } from 'solid-js'

// The app's icon, drawn from the layers in icon/AppIcon.icon: a record on
// a red card, seen so close it runs off the edges, its red label and white
// tonearm. The card follows the theme.
export function AppIcon(props: { size?: number; class?: string }) {
  const grooves = Array.from({ length: 45 }, (_, i) => 235.2 + i * 7)
  return (
    <svg
      class={props.class}
      width={props.size ?? 32}
      height={props.size ?? 32}
      viewBox="0 0 1024 1024"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id="icon-card" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" style={{ 'stop-color': 'var(--icon-top)' }} />
          <stop offset="1" style={{ 'stop-color': 'var(--icon-foot)' }} />
        </linearGradient>
        <radialGradient id="icon-disc" cx="330" cy="712" r="560" gradientUnits="userSpaceOnUse">
          <stop offset="0.35" stop-color="#2A2A30" />
          <stop offset="0.95" stop-color="#17171B" />
          <stop offset="1" stop-color="#0E0E11" />
        </radialGradient>
        <linearGradient id="icon-red" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stop-color="#FF5B5B" />
          <stop offset="1" stop-color="#E3122C" />
        </linearGradient>
        <linearGradient id="icon-arm" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stop-color="#FFFFFF" />
          <stop offset="1" stop-color="#E6E3E5" />
        </linearGradient>
        <linearGradient id="icon-sheen" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stop-color="#fff" stop-opacity="0" />
          <stop offset="0.5" stop-color="#fff" stop-opacity="0.2" />
          <stop offset="1" stop-color="#fff" stop-opacity="0" />
        </linearGradient>
        <clipPath id="icon-shape">
          <rect width="1024" height="1024" rx="230" />
        </clipPath>
      </defs>
      <g clip-path="url(#icon-shape)">
        <rect width="1024" height="1024" fill="url(#icon-card)" />
        <circle cx="330" cy="712" r="560" fill="url(#icon-disc)" />
        <circle cx="330" cy="712" r="552" fill="none" stroke="#fff" stroke-opacity="0.14" stroke-width="3" />
        <For each={grooves}>
          {(r, i) => (
            <circle
              cx="330"
              cy="712"
              r={r}
              fill="none"
              stroke="#fff"
              stroke-opacity={i() % 2 ? 0.07 : 0.04}
              stroke-width="1.4"
            />
          )}
        </For>
        <path d={wedge(2, 48)} fill="url(#icon-sheen)" />
        <path d={wedge(182, 228)} fill="url(#icon-sheen)" opacity="0.7" />
        <circle cx="330" cy="712" r="200.48" fill="url(#icon-red)" />
        <circle cx="330" cy="712" r="138.32" fill="none" stroke="#fff" stroke-opacity="0.28" stroke-width="3" />
        <circle cx="330" cy="712" r="29.12" fill="#FFF6F2" />
      </g>
      <g transform="rotate(127.52 820 214)">
        <rect x="716" y="188" width="64" height="52" rx="15" fill="url(#icon-arm)" />
        <rect x="820" y="197" width="355.83" height="34" rx="17" fill="url(#icon-arm)" />
        <rect
          x="1153.83"
          y="186"
          width="92"
          height="56"
          rx="14"
          fill="url(#icon-arm)"
          transform="rotate(28 1175.83 214)"
        />
      </g>
      <circle cx="820" cy="214" r="72" fill="url(#icon-arm)" />
      <circle cx="820" cy="214" r="27" fill="#D9D5D8" />
    </svg>
  )
}

// wedge is the path of the record's sheen between two angles, in degrees
// clockwise from 12 o'clock, from the label's edge to the rim.
function wedge(from: number, to: number) {
  const at = (deg: number, r: number) => {
    const a = ((deg - 90) * Math.PI) / 180
    return `${(330 + r * Math.cos(a)).toFixed(2)} ${(712 + r * Math.sin(a)).toFixed(2)}`
  }
  return `M${at(from, 210)}L${at(from, 550)}A550 550 0 0 1 ${at(to, 550)}L${at(to, 210)}A210 210 0 0 0 ${at(from, 210)}Z`
}
