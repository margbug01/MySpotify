package app

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"vinyl/internal/player"
)

// The backdrop of Now Playing paints the cover's colors behind it, as
// NetEase's own player does: deep shades of its colors, up to three, each
// toward where it sits on the cover, lighter at the top, with the cover
// itself, blurred, faintly through it. It stays
// still, so it is painted once a song, into a small canvas an eighth of the
// window across, which shows scaled up as smooth as it is: what moves over
// it then draws one image under it.
//
// The experimental fog (settings.Fog) makes it alive, as NetEase's mobile
// player's is: fog of the cover's color, billowing as it rises slowly
// through a deep shade of it, with specks of dust glinting in it, brighter
// and thicker with the bass. A shader paints it into a canvas, half the
// window across on the GPU, thirty times a second while the music plays;
// the CPU paints it still. Paused, the fog glides to a stop, and the
// canvas stays as it is.
//
// Another song's backdrop fades in over the last one's, which stays as it
// was, in a canvas of its own, until it is gone.

// fogSource paints the fog: noise of five octaves, warped by noise of
// its own so that it billows rather than slides, rising with Time, Fog
// over Deep where it is thick; thickest a little above the middle, and
// thinning toward the top and the controls at the foot. Deep and Fog are
// each the cover's three colors: the first two toward Pos1 and Pos2,
// meeting along a line the fog bends, the third in broad swirls; mixed in
// Oklab, keeping their chroma, as two colors mixed in RGB go gray. Still,
// it paints no fog: the colors, lighter at the top.
const fogSource = `package main

var Time float
var Bass float
var Still float
var Deep1 vec4
var Fog1 vec4
var Deep2 vec4
var Fog2 vec4
var Deep3 vec4
var Fog3 vec4
var Pos1 vec2
var Pos2 vec2

func hash(p vec2) float {
	return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453)
}

func noise(p vec2) float {
	i := floor(p)
	f := fract(p)
	u := f * f * (3.0 - 2.0*f)
	a := hash(i)
	b := hash(i + vec2(1, 0))
	c := hash(i + vec2(0, 1))
	d := hash(i + vec2(1, 1))
	return mix(mix(a, b, u.x), mix(c, d, u.x), u.y)
}

func fbm(p vec2) float {
	v := 0.0
	amp := 0.5
	for i := 0; i < 5; i++ {
		v += amp * noise(p)
		p = p*2.03 + vec2(1.7, 9.2)
		amp *= 0.5
	}
	return v
}

func oklab(c vec3) vec3 {
	l := pow(max(c, vec3(0)), vec3(2.2))
	lms := vec3(
		0.41222147*l.r+0.53633254*l.g+0.05144599*l.b,
		0.21190350*l.r+0.68069955*l.g+0.10739696*l.b,
		0.08830246*l.r+0.28171884*l.g+0.62997870*l.b,
	)
	g := pow(max(lms, vec3(0)), vec3(1.0/3.0))
	return vec3(
		0.21045426*g.x+0.79361779*g.y-0.00407205*g.z,
		1.9779985*g.x-2.4285922*g.y+0.45059371*g.z,
		0.02590404*g.x+0.78277177*g.y-0.80867577*g.z,
	)
}

func fromOklab(lab vec3) vec3 {
	l := lab.x + 0.39633778*lab.y + 0.21580376*lab.z
	m := lab.x - 0.10556135*lab.y - 0.06385417*lab.z
	s := lab.x - 0.08948418*lab.y - 1.2914855*lab.z
	lms := vec3(l*l*l, m*m*m, s*s*s)
	lin := vec3(
		4.0767417*lms.x-3.3077116*lms.y+0.23096993*lms.z,
		-1.268438*lms.x+2.6097574*lms.y-0.3413194*lms.z,
		-0.0041960863*lms.x-0.7034186*lms.y+1.7076147*lms.z,
	)
	return clamp(pow(max(lin, vec3(0)), vec3(1.0/2.2)), vec3(0), vec3(1))
}

// mixVivid mixes a and b in Oklab, its chroma kept between theirs.
func mixVivid(a, b vec3, t float) vec3 {
	la := oklab(a)
	lb := oklab(b)
	m := mix(la, lb, t)
	want := mix(length(la.yz), length(lb.yz), t)
	cm := length(m.yz)
	if cm > 0.004 && cm < want {
		m = vec3(m.x, m.yz*min(want/cm, 1.35))
	}
	return fromOklab(m)
}

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	uv := pos / size
	p := pos / size.y * 2.4
	t := Time
	rise := vec2(0, t*0.1)
	q := vec2(0.5)
	cloud := 0.0
	if Still < 0.5 {
		q = vec2(fbm(p+rise+vec2(0, t*0.04)), fbm(p+rise+vec2(5.2, 1.3)-vec2(t*0.05, 0)))
		f := fbm(p + rise + 0.9*q)
		// Broad banks of it, and the puffs within them.
		bank := fbm(p*0.45 + rise*0.6 + vec2(3.7, 0))
		cloud = smoothstep(0.3-0.06*Bass, 0.78, f) * (0.45 + 0.75*smoothstep(0.3, 0.65, bank))
	} else {
		cloud = 0.32 * (1.0 - uv.y)
	}
	// Thickest a little above the middle.
	m := smoothstep(-0.1, 0.4, uv.y) * (1.0 - 0.5*smoothstep(0.65, 1.0, uv.y))
	if Still >= 0.5 {
		m = 1.0
	}
	d1 := length(uv - Pos1 + (q-vec2(0.5))*0.45)
	d2 := length(uv - Pos2 - (q-vec2(0.5))*0.45)
	w2 := smoothstep(-0.35, 0.45, d1-d2)
	swirl := smoothstep(0.35, 0.72, fbm(p*0.38-rise*0.4+vec2(8.3, 2.1)))
	deep := mixVivid(mixVivid(Deep1.rgb, Deep2.rgb, w2), Deep3.rgb, swirl*0.5)
	fog := mixVivid(mixVivid(Fog1.rgb, Fog2.rgb, w2), Fog3.rgb, swirl*0.6)
	c := mix(deep, fog, min(cloud*m*(0.85+0.35*Bass), 1.0))
	// A broad glow behind it all, darker at the corners.
	g := length((uv - vec2(0.5, 0.42)) * vec2(1.0, 1.25))
	c *= 1.08 - 0.45*smoothstep(0.2, 0.85, g)
	if Still >= 0.5 {
		return vec4(clamp(c, vec3(0), vec3(1)), 1)
	}
	// Specks of dust, rising with the fog, each glinting in its own time,
	// a little of its color.
	cell := size.y / 22
	sp := (pos + vec2(0, t*cell*0.9)) / cell
	id := floor(sp)
	h := hash(id + vec2(3.1, 7.7))
	if h > 0.93 {
		at := vec2(hash(id+vec2(1.3, 2.9)), hash(id+vec2(8.1, 4.3)))*0.8 + 0.1
		d := length((fract(sp)-at)*cell) / max(cell*0.05, 0.7)
		glint := 0.5 + 0.5*sin(t*(1.5+2.0*h)+h*40.0)
		spark := mix(vec3(0.78, 0.85, 1.0), clamp(fog*1.5, vec3(0), vec3(1)), 0.4)
		c += spark * exp(-d*d) * glint * (0.25 + 0.3*m)
	}
	return vec4(clamp(c, vec3(0), vec3(1)), 1)
}
`

var fogShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(fogSource) })

// backdropFrame is how often the fog is painted while it moves.
const backdropFrame = time.Second / 30

// backdropFade is how long another song's backdrop takes to fade in.
const backdropFade = 700 * time.Millisecond

// backdropCache is the backdrop as last painted, and where the fog's
// drift and its pulse are.
type backdropCache struct {
	canvas *ui.Canvas
	before *ui.Canvas // the song before's, fading out over it
	fadeAt time.Time  // when it began to
	art    *artwork
	tint   ui.Color
	fog    bool
	at     time.Time // when it was painted
	drift  float64   // the clouds' time, in seconds, which stops on pause
	speed  float64   // how fast drift goes: 1 playing, 0 paused
	level  barState
	last   time.Time
}

// backdrop paints the backdrop behind Now Playing, still or fog.
func (a *app) backdrop(c *ui.Context, art *artwork, tint ui.Color) {
	pl := a.pl
	fog, still := a.settings.Fog, c.Preferences().ReduceMotion
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		// The CPU, drawing the fog for want of a GPU, paints far fewer
		// pixels, and those once a song: the fog stays still.
		gpu := p.Accelerated()
		div := float32(8)
		if fog && gpu {
			div = 2
		}
		cw, ch := int(r.W/div)+1, int(r.H/div)+1
		bd := &a.backdropCache
		if bd.canvas != nil {
			if w, h := bd.canvas.Size(); w != cw || h != ch {
				bd.canvas, bd.before = nil, nil
			}
		}
		now := p.Now()
		if bd.canvas != nil && (bd.art != art || bd.tint != tint) {
			// Another song: the last one's stays as it was, fading out.
			bd.canvas, bd.before, bd.fadeAt = bd.before, bd.canvas, now
		}
		moving := fog && gpu && !still
		lv, live := pl.Levels()
		moved := bd.step(now, pl.Playing() && moving, lv, live && moving)
		due := now.Sub(bd.at) >= backdropFrame-2*time.Millisecond
		if bd.canvas == nil || bd.art != art || bd.tint != tint || bd.fog != fog || moved && due {
			if bd.canvas == nil {
				bd.canvas = ui.NewCanvas(cw, ch)
			}
			bd.art, bd.tint, bd.fog, bd.at = art, tint, fog, now
			bass := bd.level.group(0, 6)
			pal := tintPalette(tint)
			if art != nil && art.pal != nil {
				pal = *art.pal
			}
			bd.canvas.Draw(p, func(cp *ui.Painter) {
				box := ui.Rect{W: float32(cw), H: float32(ch)}
				if fog {
					drawFog(cp, box, pal, float32(bd.drift), bass, false)
				} else {
					drawBackdrop(cp, box, art, pal)
				}
			})
		}
		p.Image(bd.canvas.Bitmap(), r, ui.FillBox)
		if bd.before != nil {
			if k := float32(now.Sub(bd.fadeAt)) / float32(backdropFade); k < 1 {
				// Eased out, as it fades.
				k = 1 - (1-k)*(1-k)*(1-k)
				p.ImageRegion(bd.before.Bitmap(), ui.Rect{}, r, ui.ImageOptions{Tint: white.Alpha(1 - k)})
				p.AnimationFrame()
			} else {
				bd.before = nil
			}
		}
		if moved {
			p.After(backdropFrame)
		}
	})
}

// step moves the drift and the pulse on to now, and reports whether
// either still moves.
func (bd *backdropCache) step(now time.Time, playing bool, lv [player.Bands]float32, live bool) bool {
	dt := now.Sub(bd.last).Seconds()
	if bd.last.IsZero() || dt > 0.2 {
		dt = 1.0 / 30
	}
	bd.last = now
	to := 0.0
	if playing {
		to = 1
	}
	// The drift eases in as the music plays, and glides to a stop.
	bd.speed += (to - bd.speed) * min(1, 2.5*dt)
	if math.Abs(to-bd.speed) < 0.005 {
		bd.speed = to
	}
	bd.drift += bd.speed * dt
	bd.level.step(now, lv, live)
	return bd.speed > 0 || bd.level.group(0, 6) > 0.004
}

// drawBackdrop paints the still backdrop of pal in r.
func drawBackdrop(p *ui.Painter, r ui.Rect, art *artwork, pal coverPalette) {
	drawFog(p, r, pal, 0, 0, true)
	if art != nil && art.backdrop != nil {
		// The cover, blurred, as a faint wash of its own colors.
		p.ImageRegion(art.backdrop, ui.Rect{W: 48, H: 48}, coverRect(r, 48, 48), ui.ImageOptions{Tint: white.Alpha(0.1)})
	}
	// Darker toward the foot, under the controls.
	p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: ui.RGBA(0, 0, 0, 0), At: 0.55}, {Color: ui.RGBA(0, 0, 0, 0.18), At: 1},
	}}, 0)
}

// coverRect returns where an image of w×h goes to cover r, centered.
func coverRect(r ui.Rect, w, h float32) ui.Rect {
	s := max(r.W/w, r.H/h)
	return ui.Rect{X: r.X + (r.W-w*s)/2, Y: r.Y + (r.H-h*s)/2, W: w * s, H: h * s}
}

// drawFog paints the fog of pal in r at drift seconds, lit by bass, or
// still, with no fog.
func drawFog(p *ui.Painter, r ui.Rect, pal coverPalette, drift, bass float32, still bool) {
	d1, f1 := fogColors(pal.c1, pal)
	d2, f2 := fogColors(pal.c2, pal)
	d3, f3 := fogColors(pal.c3, pal)
	var s float32
	if still {
		s = 1
	}
	fogShader().Draw(p, r, shader.Options{Uniforms: map[string]any{
		"Time": drift, "Bass": min(bass, 1), "Still": s,
		"Deep1": d1, "Fog1": f1, "Deep2": d2, "Fog2": f2, "Deep3": d3, "Fog3": f3,
		// The cover shows at the left: its colors lean that way.
		"Pos1": [2]float32{0.25 + pal.p1[0]*0.35, pal.p1[1]},
		"Pos2": [2]float32{0.45 + pal.p2[0]*0.45, pal.p2[1]},
	}})
}

// fogColors returns the deep shade behind the fog and the fog's own for
// the color c of a cover of pal: its hue, rich as stained glass on a cover
// with color, at about the same depths whatever the cover, a little darker
// for a dark one, and no lighter than white reads over. A gray cover stays
// gray.
func fogColors(c ui.Color, pal coverPalette) (deep, fog ui.Color) {
	h, s, v := hsv(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	rich := min(max(s*1.62, 0.68), 0.88)
	// A gray's saturation as if it were at least half bright: a warm black
	// has much of it but little color, and would light up brown.
	gray := s * v / max(v, 0.5)
	sd := gray*0.85 + (rich-gray*0.85)*pal.vivid
	night := 0.62 + 0.38*min(1, math.Pow(pal.lightness/0.35, 0.55))
	return dimTo(hsvColor(h, sd, 0.33*night), 0.28), dimTo(hsvColor(h, sd*0.7, 0.74*night), 0.59)
}

// dimTo returns c darkened to a luma of y, if it is lighter.
func dimTo(c ui.Color, y float64) ui.Color {
	l := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
	if l <= y {
		return c
	}
	k := y / l
	return ui.RGB(uint8(float64(c.R)*k), uint8(float64(c.G)*k), uint8(float64(c.B)*k))
}

// hsvColor is the color of hue h (degrees), saturation s and value v.
func hsvColor(h, s, v float64) ui.Color {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := v - c
	return ui.RGB(uint8((r+m)*255+0.5), uint8((g+m)*255+0.5), uint8((b+m)*255+0.5))
}
