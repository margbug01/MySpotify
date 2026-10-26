import { Show } from 'solid-js'

// An album cover made of light: a soft gradient around a hue, in place of
// real artwork.
export function Cover(props: {
  hue: number
  size: number
  radius?: number
  label?: string
  class?: string
}) {
  return (
    <div
      class={`cover ${props.class ?? ''}`}
      style={{
        '--h': props.hue,
        width: `${props.size}px`,
        height: `${props.size}px`,
        'border-radius': `${props.radius ?? 8}px`,
      }}
    >
      <Show when={props.label}>
        <span class="cover-label">{props.label}</span>
      </Show>
    </div>
  )
}
