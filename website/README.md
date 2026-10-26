# 网站

MyGo 云音乐的主页，用 TanStack Start 与 Solid 写成。构建时预渲染为静态文件，部署到 Cloudflare（Workers 静态资源，不运行 Worker 脚本）。

```bash
bun install          # 在仓库根目录
bun run dev          # http://localhost:3000
bun run build        # 输出到 website/dist/client
bun run deploy       # 构建并用 wrangler 部署
```
