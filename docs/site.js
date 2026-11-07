// The page works without this: downloads go to the releases page and every line stays lit.
const root = document.documentElement
root.classList.add("js")
if (/Win/.test(navigator.userAgent)) root.classList.add("win")

// Direct links to the installers, as the release page lists a dozen other files.
const installers = { mac: /macos-arm64\.dmg$/, win: /windows-amd64\.zip$/ }
const barButton = document.querySelector(".bar .btn")
barButton.dataset.dl = root.classList.contains("win") ? "win" : "mac"

fetch("https://api.github.com/repos/margbug01/MySpotify/releases/latest")
  .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
  .then((release) => {
    document.querySelector("[data-version]").textContent = release.tag_name
    for (const [os, name] of Object.entries(installers)) {
      const asset = release.assets.find((a) => name.test(a.name))
      if (!asset) continue
      for (const link of document.querySelectorAll(`[data-dl="${os}"]`)) {
        link.href = asset.browser_download_url
        const size = link.querySelector("[data-size]")
        if (size) size.textContent = `${size.dataset.size} · ${(asset.size / 1048576).toFixed(1)} MB`
      }
    }
  })
  .catch(() => {})

// The line at the middle of the window is the one being sung, and its screen shows.
const lyrics = document.querySelector(".lyrics")
const lines = [...document.querySelectorAll(".line")]
const screens = [...document.querySelectorAll(".stage img")]
const nowTitle = document.querySelector("[data-now]")

function sing(line) {
  for (const l of lines) l.classList.toggle("on", l === line)
  for (const s of screens) s.classList.toggle("on", s.dataset.shot === line.dataset.shot)
  lyrics.style.setProperty("--glow", line.dataset.glow)
}
sing(lines[0])

const atMiddle = new IntersectionObserver(
  (entries) => {
    for (const { isIntersecting, target } of entries) {
      if (!isIntersecting) continue
      if (target.classList.contains("line")) sing(target)
      const title = target.querySelector("h3, h2")
      if (title) nowTitle.textContent = title.textContent
    }
  },
  { rootMargin: "-50% 0px -50% 0px" },
)
for (const el of [...lines, ...document.querySelectorAll(".section")]) atMiddle.observe(el)

// The bar shows once the hero's own buttons have gone by, its track following the scroll.
const bar = document.querySelector(".bar")
new IntersectionObserver(([hero]) => bar.classList.toggle("on", !hero.isIntersecting)).observe(document.querySelector(".cta"))

const fill = document.querySelector(".bar-track i")
const elapsed = document.querySelector("[data-elapsed]")
const LENGTH = 4 * 60 + 12
function follow() {
  const progress = Math.min(1, scrollY / Math.max(1, document.body.scrollHeight - innerHeight))
  fill.style.scale = `${progress} 1`
  const seconds = Math.round(progress * LENGTH)
  elapsed.textContent = `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`
}
addEventListener("scroll", follow, { passive: true })
addEventListener("resize", follow)
follow()

// Each line's timestamp is when the bar reaches it, whatever the window's size.
function stamp() {
  const scrollable = Math.max(1, document.body.scrollHeight - innerHeight)
  for (const line of lines) {
    const box = line.getBoundingClientRect()
    const seconds = ((box.top + scrollY + box.height / 2 - innerHeight / 2) / scrollable) * LENGTH
    const minutes = String(Math.floor(seconds / 60)).padStart(2, "0")
    line.querySelector("time").textContent = `[${minutes}:${(seconds % 60).toFixed(2).padStart(5, "0")}]`
  }
}
addEventListener("resize", stamp)
addEventListener("load", stamp)
