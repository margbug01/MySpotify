package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// likes are the songs the listener likes, which hearts show filled.

// likeSong likes or unlikes a song with Spotify; tests replace it.
var likeSong = spotify.Like

// loadLiked loads the listener's liked songs.
func (a *app) loadLiked(uid string) {
	go func() {
		ids, err := spotify.LikedIDs()
		a.post(func() {
			if err != nil || a.profile == nil || a.profile.UserID != uid {
				return
			}
			a.liked = make(map[string]bool, len(ids))
			for _, id := range ids {
				a.liked[id] = true
			}
		})
	}()
}

// toggleLike likes a song, or unlikes it, at once in the app and then with
// Spotify, which may refuse: the heart goes back then. Signed out, it asks
// the listener to sign in.
func (a *app) toggleLike(tr *spotify.Track) {
	if a.profile == nil {
		a.showMain()
		a.openLogin()
		return
	}
	if a.liked == nil {
		a.liked = map[string]bool{}
	}
	id, like := tr.ID, !a.liked[tr.ID]
	a.setLiked(id, like)
	go func() {
		err := likeSong(id, like)
		a.post(func() {
			if err != nil {
				a.setLiked(id, !like)
				a.toast = "操作失败，请稍后再试"
				return
			}
			// The liked songs' playlist changed: load it anew when shown.
			delete(a.playlists, spotify.LikedURI)
			if a.route.page == pagePlaylist && a.route.playlist == spotify.LikedURI {
				a.loadPlaylist(spotify.LikedURI, false)
			}
		})
	}()
}

// likeCurrent likes the song playing, or unlikes it, from the keyboard
// (⌘D), which shows no heart: a toast says which it did.
func (a *app) likeCurrent() {
	tr := a.pl.Current()
	if tr == nil {
		return
	}
	a.toggleLike(tr)
	if a.profile == nil {
		return // asked to sign in
	}
	if a.liked[tr.ID] {
		a.toast = "已添加到我喜欢的音乐"
	} else {
		a.toast = "已取消喜欢"
	}
}

func (a *app) setLiked(id string, like bool) {
	if like {
		a.liked[id] = true
	} else {
		delete(a.liked, id)
	}
}

// likeButton is a heart that likes tr: filled in red once liked. Liking
// pops it in on a spring while a ring swells around it and confetti flies
// out; unliking lets it give a little.
func (a *app) likeButton(c *ui.Context, tr *spotify.Track, size float32, color ui.Color) ui.Element {
	liked := a.liked[tr.ID]
	icon, label := iconHeart, "喜欢"
	if liked {
		icon, color, label = iconHeartFill, a.pal.accent, "取消喜欢"
	}
	// The burst goes on a wrapper, which the heart's pop does not scale.
	wrap := ui.Box(c).Center()
	var b ui.Element
	wrap.Children(func() { b = iconButton(c, icon, size, color, label) })
	scale := b.AnimateTo("pop", 1, ui.Animation{Spring: ui.SpringOptions{Duration: 550 * time.Millisecond, Bounce: 0.55}})
	b.Scale(scale, scale)
	burst := ui.Local(wrap, "burst", func() time.Time { return time.Time{} })
	if b.Clicked() {
		switch {
		case a.profile == nil:
		case liked:
			b.Jump("pop", 0.8)
		default:
			b.Jump("pop", 0.2)
			*burst = c.Now()
		}
		a.toggleLike(tr)
	}
	if !burst.IsZero() {
		start, accent := *burst, a.pal.accent
		wrap.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if drawBurst(p, r, size, p.Now().Sub(start), accent) {
				p.AnimationFrame()
			}
		})
	}
	return wrap
}

// burstColors are the confetti's colors, around the heart's red.
var burstColors = []ui.Color{
	ui.Hex("#ff4d6d"), ui.Hex("#ff9f1c"), ui.Hex("#ffd166"),
	ui.Hex("#c77dff"), ui.Hex("#4cc9f0"), ui.Hex("#ff70a6"),
}

const burstTime = 750 * time.Millisecond

// drawBurst draws the burst of a heart of size in r, at elapsed since the
// like, and reports whether it goes on.
func drawBurst(p *ui.Painter, r ui.Rect, size float32, elapsed time.Duration, accent ui.Color) bool {
	t := float32(elapsed) / float32(burstTime)
	if t >= 1 || t < 0 {
		return false
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	out := func(x float32) float32 { return 1 - (1-x)*(1-x)*(1-x) } // ease out

	// The ring swells from the heart and thins away, in the first half.
	if rt := t * 2; rt < 1 {
		radius := size * (0.35 + 0.65*out(rt))
		width := size * 0.22 * (1 - rt)
		ring := (&ui.Path{}).Circle(cx, cy, radius)
		p.StrokePath(ring, width, accent.Alpha(0.9*(1-rt)))
	}

	// Two rings of confetti fly out, slowing, shrinking and fading; the
	// inner one a little behind and turned, as if thrown at once.
	for ring := range 2 {
		lag := float32(ring) * 0.08
		pt := (t - 0.12 - lag) / (0.88 - lag)
		if pt <= 0 {
			continue
		}
		pt = min(pt, 1)
		const n = 7
		dist := size * (0.55 + (1.15-0.25*float32(ring))*out(pt))
		dot := size * (0.16 - 0.04*float32(ring)) * (1 - pt*pt)
		alpha := 1 - pt*pt
		for i := range n {
			angle := (float64(i)/n + float64(ring)*0.5/n - 0.25) * 2 * math.Pi
			x := cx + dist*float32(math.Cos(angle))
			y := cy + dist*float32(math.Sin(angle))
			color := burstColors[(i+ring*3)%len(burstColors)].Alpha(alpha)
			p.Fill(ui.Rect{X: x - dot, Y: y - dot, W: 2 * dot, H: 2 * dot}, color, dot)
		}
	}
	return true
}
