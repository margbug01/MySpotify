import { Link } from '@tanstack/solid-router'
import { createSignal, onCleanup, onMount, type JSX } from 'solid-js'
import { AppIcon } from './AppIcon'
import { GitHubMark } from './Icon'
import { ThemeSwitch } from '../theme'
import { repo, site } from '../site'

export function Layout(props: { children: JSX.Element }) {
  return (
    <>
      <Header />
      <main>{props.children}</main>
      <Footer />
    </>
  )
}

function Header() {
  // Hairline and frosted backing only once the page has scrolled under it.
  const [scrolled, setScrolled] = createSignal(false)
  onMount(() => {
    const on = () => setScrolled(window.scrollY > 8)
    on()
    window.addEventListener('scroll', on, { passive: true })
    onCleanup(() => window.removeEventListener('scroll', on))
  })
  return (
    <header class="nav" classList={{ scrolled: scrolled() }}>
      <div class="nav-inner">
        <Link to="/" class="brand" aria-label={site.name}>
          <AppIcon size={28} />
          <span>{site.name}</span>
        </Link>
        <div class="nav-end">
          <ThemeSwitch class="nav-theme" />
          <a class="icon-link" href={repo} aria-label="GitHub 上的源代码">
            <GitHubMark size={19} />
          </a>
        </div>
      </div>
    </header>
  )
}

function Footer() {
  return (
    <footer class="footer">
      <div class="footer-inner">
        <span>
          {site.name} · 以 <a href={`${repo}/blob/main/LICENSE`}>AGPL-3.0</a> 开源 · 与网易云音乐官方无关
        </span>
        <a class="icon-link" href={repo} aria-label="GitHub 上的源代码">
          <GitHubMark size={17} />
        </a>
      </div>
    </footer>
  )
}
