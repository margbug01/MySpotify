package player

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"vinyl/internal/spotify"
)

// systemEvent reports, as the page does, what the system did to out.
func systemEvent(p *Player, out *webOut, kind string, at float64, seq int) {
	page(p, http.MethodPost, "/event", fmt.Sprintf(`{"gen":%d,"type":%q,"t":%g,"seq":%d}`, out.gen, kind, at, seq))
}

// A pause from the system (Mami pausing what plays while one talks, the
// media keys, Control Center), which WebKit carries out on the page itself,
// left Vinyl showing the song playing and never letting the page go: the
// player now follows the page, and plays again with it.
func TestSystemPauseAndPlay(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	out := p.out.(*webOut)
	if !p.Playing() {
		t.Fatal("not playing to begin with")
	}
	changed := 0
	p.Changed = func() { changed++ }
	systemEvent(p, out, "playing", 10, 0)
	systemEvent(p, out, "system-pause", 12.5, out.asked)
	next(t, ch)
	if p.Playing() || out.Playing() || p.Position() != 12500*time.Millisecond || changed == 0 || p.idle == nil {
		t.Fatalf("after the system's pause: playing %v, page playing %v, at %v, changed %d, idle timer %v", p.Playing(), out.Playing(), p.Position(), changed, p.idle != nil)
	}
	asked := out.asked
	systemEvent(p, out, "system-play", 12.5, out.asked)
	next(t, ch)
	if !p.Playing() || !out.Playing() || p.idle != nil {
		t.Fatal("did not follow the system playing again")
	}
	if out.asked != asked {
		t.Fatal("asked the page to play what already plays")
	}
}

// Vinyl's own pause fades the song out first: played again by the system,
// it played on silent.
func TestSystemPlayAfterVinylsPause(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	out := p.out.(*webOut)
	p.TogglePlay()
	for out.Playing() {
		next(t, ch)
	}
	if !p.fade.silent() {
		t.Fatal("Vinyl's pause did not fade out")
	}
	systemEvent(p, out, "system-play", 3, out.asked)
	next(t, ch)
	p.fade.mu.Lock()
	to := p.fade.to
	p.fade.mu.Unlock()
	if !p.Playing() || to != 1 {
		t.Fatalf("played by the system: playing %v, fading to %v", p.Playing(), to)
	}
}

// What the page reports of the system after Vinyl asked it to play or pause
// is stale: Vinyl's ask stands.
func TestSystemPauseAfterVinylAsked(t *testing.T) {
	_, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	out := p.out.(*webOut)
	stale := out.asked
	out.Pause()
	out.Play()
	systemEvent(p, out, "system-pause", 4, stale)
	if !out.Playing() {
		t.Fatal("a stale pause of the system's stopped what Vinyl asked to play")
	}
}

// The media keys' and Control Center's skips, which WebKit passes the page,
// skip.
func TestSystemSkips(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	systemEvent(p, p.out.(*webOut), "system-next", 1, 0)
	next(t, ch)
	for p.Loading() {
		next(t, ch)
	}
	if p.at != 1 {
		t.Fatalf("next: at %d", p.at)
	}
	systemEvent(p, p.out.(*webOut), "system-previous", 1, 0)
	next(t, ch)
	for p.Loading() {
		next(t, ch)
	}
	if p.at != 0 {
		t.Fatalf("previous: at %d", p.at)
	}
}

// Control Center's progress bar, which WebKit passes the page once the page
// handles seekto (#30), moves Vinyl there, as its own seek bar does.
func TestSystemSeeks(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	systemEvent(p, p.out.(*webOut), "system-seek", 42.5, 0)
	next(t, ch)
	if p.Position() != 42500*time.Millisecond {
		t.Fatalf("after the system's seek: at %v", p.Position())
	}
}

// WebKit shows the system the page playing as a media session of its own,
// which the app's stood beside, titled Vinyl and naming no song (#27): the
// page names the track while it plays it and the app shows nothing, till
// the page is let go and the app shows it again.
func TestPageShowsWhatPlays(t *testing.T) {
	defer func(was bool) { pageNowPlaying = was }(pageNowPlaying)
	pageNowPlaying = true
	oldVisibility := nativeNowPlayingVisible
	defer func() { nativeNowPlayingVisible = oldVisibility }()
	var visible bool
	nativeNowPlayingVisible = func(v bool) { visible = v }
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer func() { p.CloseWeb(); mygo.NowPlaying.Clear() }()
	p.PlayQueue([]spotify.Track{{ID: "1", Name: "曲终人散", Artists: []spotify.Artist{{Name: "张宇"}}}}, 0)
	next(t, ch)
	named := false
	for len(p.Web.cmds) > 0 {
		cmd := <-p.Web.cmds
		named = named || strings.HasPrefix(cmd, "vinyl.meta(") && strings.Contains(cmd, `"title":"曲终人散"`) && strings.Contains(cmd, `"artist":"张宇"`)
	}
	if _, shown := mygo.NowPlaying.Info(); shown || visible || !named {
		t.Fatalf("while the page plays: app shows it %v, native visible %v, page names it %v", shown, visible, named)
	}
	// Paused five minutes, the page is let go (releaseIdle).
	p.playing = false
	p.sync()
	p.releaseIdle(p.idleGen)
	if info, shown := mygo.NowPlaying.Info(); !shown || !visible || info.Title != "曲终人散" {
		t.Fatalf("once the page is let go: app shows %q (%v)", info.Title, shown)
	}
}

// Restoring or loading local playback briefly created a native client
// before WebKit existed; its empty stopped card then survived the handoff.
func TestLocalLoadingDoesNotPublishNativeSession(t *testing.T) {
	defer func(was bool) { pageNowPlaying = was }(pageNowPlaying)
	pageNowPlaying = true
	mygo.NowPlaying.Clear()
	oldVisibility := nativeNowPlayingVisible
	defer func() { nativeNowPlayingVisible = oldVisibility }()
	calls := 0
	nativeNowPlayingVisible = func(bool) { calls++ }
	p := New(func(func()) {})
	p.Cue([]spotify.Track{{ID: "song", Name: "Song"}})
	p.cue(0, false, 0) // startup restores the queue paused
	p.playing, p.loading = true, true
	p.sync()
	p.announce(p.track, []byte("cover")) // the cover may beat the audio
	if _, shown := mygo.NowPlaying.Info(); shown || calls != 0 || p.nativeShown {
		t.Fatalf("local loading published a native client: shown %v, visibility calls %d", shown, calls)
	}
}

// Audio/MV reopening keeps the track generation: queued events from the
// replaced output must not pause, seek or advance the newly opened one.
func TestReplacedOutputEventsAreIgnored(t *testing.T) {
	for _, kind := range []string{"system-pause", "system-seek", "system-next", "ended"} {
		t.Run(kind, func(t *testing.T) {
			ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
			defer p.CloseWeb()
			old := p.out.(*webOut)
			seq := old.asked
			if kind == "ended" {
				seq = 1
			}
			systemEvent(p, old, kind, 12, seq)
			replacement := &webOut{w: p.Web, gen: old.gen + 1, dur: time.Minute, pos: 5 * time.Second}
			p.out = replacement
			generation := p.gen
			next(t, ch)
			if !p.playing || p.out != replacement || p.gen != generation || p.Current().ID != "1" || replacement.Position() != 5*time.Second {
				t.Fatal("a replaced output changed the current playback")
			}
		})
	}
}

// Switching audio and video pages used to keep pageShown=true and leave
// the new page showing an older song's metadata, or none at all.
func TestSwitchingPageNamesTheCurrentTrack(t *testing.T) {
	defer func(was bool) { pageNowPlaying = was }(pageNowPlaying)
	pageNowPlaying = true
	_, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	p.sync()
	oldPage := p.pageNamed
	nextPage := &Web{cmds: make(chan string, 16)}
	p.out = &webOut{w: nextPage}
	p.sync()
	if p.pageNamed == oldPage || p.pageNamed != nextPage {
		t.Fatal("the destination page did not change")
	}
	select {
	case cmd := <-nextPage.cmds:
		if !strings.HasPrefix(cmd, "vinyl.meta(") || !strings.Contains(cmd, `"title":`) {
			t.Fatalf("new page was not named: %s", cmd)
		}
	default:
		t.Fatal("new page received no track metadata")
	}
}

// Hiding the native client for WebKit must not disable its Connect remote
// controls or the native fallback when the playback page is released.
func TestNativeSessionReturnsForConnect(t *testing.T) {
	defer func(was bool) { pageNowPlaying = was }(pageNowPlaying)
	pageNowPlaying = true
	oldVisibility := nativeNowPlayingVisible
	defer func() { nativeNowPlayingVisible = oldVisibility; mygo.NowPlaying.Clear() }()
	var visible bool
	nativeNowPlayingVisible = func(v bool) { visible = v }
	p := New(func(func()) {})
	track := spotify.Track{ID: "song", Name: "Remote song", Duration: 60000}
	p.MirrorConnect(track, 12*time.Second, true)
	if info, shown := mygo.NowPlaying.Info(); !shown || !visible || info.Title != track.Name || info.State != mygo.PlaybackPlaying {
		t.Fatalf("remote playback did not publish a controllable native session: %+v, shown %v, visible %v", info, shown, visible)
	}
	p.out = &webOut{w: &Web{cmds: make(chan string, 16)}}
	p.sync()
	if _, shown := mygo.NowPlaying.Info(); shown || visible {
		t.Fatal("the native session stayed visible beside local WebKit playback")
	}
	p.MirrorConnect(track, 20*time.Second, false)
	if info, shown := mygo.NowPlaying.Info(); !shown || !visible || info.Title != track.Name || info.State != mygo.PlaybackPaused {
		t.Fatalf("remote playback did not regain its native session: %+v, shown %v, visible %v", info, shown, visible)
	}
	p.StopConnect()
}
