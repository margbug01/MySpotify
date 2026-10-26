import { HeadContent, Scripts, createRootRoute } from '@tanstack/solid-router'
import { HydrationScript } from 'solid-js/web'
import { Suspense, type JSX } from 'solid-js'
import { NotFound } from '../components/NotFound'
import { site } from '../site'
import { themeScript } from '../theme'
import css from '../styles.css?url'

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: 'utf-8' },
      { name: 'viewport', content: 'width=device-width, initial-scale=1, viewport-fit=cover' },
      { title: site.title },
      { name: 'description', content: site.description },
      { name: 'theme-color', content: '#fbf7f1', media: '(prefers-color-scheme: light)' },
      { name: 'theme-color', content: '#141214', media: '(prefers-color-scheme: dark)' },
      { property: 'og:type', content: 'website' },
      { property: 'og:title', content: site.title },
      { property: 'og:description', content: site.description },
      { property: 'og:image', content: '/icon.png' },
      { name: 'twitter:card', content: 'summary' },
    ],
    links: [
      { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' },
      { rel: 'apple-touch-icon', href: '/icon.png' },
      { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
      { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossOrigin: 'anonymous' },
      {
        rel: 'stylesheet',
        href: 'https://fonts.googleapis.com/css2?family=Noto+Serif+SC:wght@600;900&display=swap',
      },
      { rel: 'stylesheet', href: css },
    ],
  }),
  shellComponent: RootDocument,
  notFoundComponent: NotFound,
})

function RootDocument(props: { children: JSX.Element }) {
  return (
    <html lang="zh-CN">
      <head>
        <script innerHTML={themeScript} />
        <HydrationScript />
        {/* Analytics, only on the built site, so visits to the dev server aren't counted. */}
        {import.meta.env.PROD && (
          <script
            defer
            src="https://u.egoist.dev/script.js"
            data-website-id="e2a744e5-3326-41e2-8ff3-1a5e80bcc4ac"
          />
        )}
      </head>
      <body>
        <HeadContent />
        <Suspense>{props.children}</Suspense>
        <Scripts />
      </body>
    </html>
  )
}
