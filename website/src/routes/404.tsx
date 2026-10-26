import { createFileRoute } from '@tanstack/solid-router'
import { NotFound } from '../components/NotFound'

export const Route = createFileRoute('/404')({
  head: () => ({ meta: [{ title: '找不到页面 · MyGo 云音乐' }] }),
  component: NotFound,
})
