package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// genieTime is how long Now Playing takes to pour out of the player's cover
// or back into it, within npSettle.
const genieTime = 500 * time.Millisecond

// The cover of the player bar, which is 48 DIPs and centered in the bar, 20
// from the left of the page, whose border takes 1.
const (
	genieCoverSize = 48
	genieCoverLeft = sidebarWidth + 1 + 20
	genieCoverRise = (playerBarHeight + genieCoverSize) / 2
)

// genieOnCPU warps where no GPU draws too, for tests: a shader over the whole
// window costs too much on the CPU otherwise.
var genieOnCPU = false

// genieShader funnels its content into Target (x, y, width, height), as
// macOS's Genie does: first the bottom narrows to Target along an S-curve,
// then the content slides down through the curve.
var genieShader = shader.MustCompile(`package main

var Progress float
var Target vec4

func LayerEffect(pos vec2) vec4 {
	size := rectSize()
	squeeze := smoothstep(0.0, 0.5, Progress)
	slide := smoothstep(0.2, 1.0, Progress)
	top := mix(0.0, Target.y, slide)
	bottom := mix(size.y, Target.y+Target.w, squeeze)
	s := squeeze * smoothstep(0.0, Target.y, pos.y)
	left := mix(0.0, Target.x, s)
	right := mix(size.x, Target.x+Target.z, s)
	u := (pos.x - left) / (right - left)
	v := (pos.y - top) / (bottom - top)
	edge := clamp(pos.x-left+0.5, 0.0, 1.0) * clamp(right-pos.x+0.5, 0.0, 1.0) *
		clamp(pos.y-top+0.5, 0.0, 1.0) * clamp(bottom-pos.y+0.5, 0.0, 1.0)
	return layerSample(vec2(u*size.x, v*size.y)) * edge
}
`)

// genieDraw draws the content funneled by amount, which tests swap out, as the
// CPU draws the shader slowly.
var genieDraw = func(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point, amount float32) {
	genieShader.Effect(shader.Options{Uniforms: map[string]any{
		"Progress": amount,
		"Target":   [4]float32{genieCoverLeft, box.H - genieCoverRise, genieCoverSize, genieCoverSize},
	}}).PaintEffect(p, box, content, inset)
}

// genieAmount is how far Now Playing has gone into the cover at now: 0 open,
// 1 gone. It starts from npFrom, so that a turn halfway goes back from there.
func (a *app) genieAmount(now time.Time) float32 {
	to := float32(1)
	if a.npOpen {
		to = 0
	}
	q := min(max(float32(now.Sub(a.npChangedAt))/float32(genieTime), 0), 1)
	return a.npFrom + (to-a.npFrom)*q*q*(3-2*q)
}

// npGenie draws Now Playing through genieShader. The amount moves as it
// paints, in frames that build nothing.
type npGenie struct{ a *app }

func (g npGenie) EffectMargin() ui.Point { return ui.Point{} }

func (g npGenie) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	a := g.a
	if p.Preferences().ReduceMotion || !p.Accelerated() && !genieOnCPU {
		// No warp: open it shows, and closed it does not.
		if a.npOpen {
			p.ImageRegion(content, ui.Rect{}, box, ui.ImageOptions{})
		}
		return
	}
	now := p.Now()
	genieDraw(p, box, content, inset, a.genieAmount(now))
	if now.Sub(a.npChangedAt) < genieTime {
		p.AnimationFrame()
	}
}
