import { For } from 'solid-js'
import { Icon } from './Icon'
import { liked, setLiked } from '../player'

// The heart beside the song; liking it bursts, as in the app.
export function Like() {
  return (
    <button
      type="button"
      class="like"
      classList={{ on: liked() }}
      aria-pressed={liked()}
      aria-label={liked() ? '取消喜欢' : '喜欢'}
      onClick={() => setLiked(!liked())}
    >
      <Icon name="heart" size={16} />
      <span class="burst" aria-hidden="true">
        <For each={[0, 1, 2, 3, 4, 5]}>{(i) => <i style={{ '--a': `${i * 60}deg` }} />}</For>
      </span>
    </button>
  )
}
