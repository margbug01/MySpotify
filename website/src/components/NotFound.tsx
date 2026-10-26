import { Link } from '@tanstack/solid-router'
import { Layout } from './Layout'
import { Icon } from './Icon'

export function NotFound() {
  return (
    <Layout>
      <section class="lost">
        <div class="lost-record" aria-hidden="true">
          <div class="vinyl" />
        </div>
        <p class="eyebrow">404</p>
        <h1 class="display">这首歌找不到了</h1>
        <p class="lede">也许它已下架，也许链接写错了。回到首页，换一首听听。</p>
        <Link to="/" class="btn btn-primary">
          回到首页
          <Icon name="arrow" size={16} />
        </Link>
      </section>
    </Layout>
  )
}
