package player

import (
	"slices"
	"strings"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// A song nearing its end, with crossfade on and the next resolved ahead,
// has the next start fading in as this one fades out; it doesn't for the
// next on the same album, with crossfade off, or before the next is found.
func TestSongsOverlapAtTheEnd(t *testing.T) {
	song := func(id, album string) spotify.Track {
		return spotify.Track{ID: id, Album: spotify.Album{ID: album}}
	}
	near := func(p *Player) { p.out.(*webOut).event("near", 0, "", 0) }
	run := func(crossfade float64, queue []spotify.Track, wait bool) []string {
		p, _, until := aheadPlayer(t)
		p.SetCrossfade(crossfade)
		p.PlayQueue(queue, 0)
		until("track 1 playing", func() bool { return p.out != nil && p.track.ID == "1" })
		if wait {
			until("track 2 resolved ahead", func() bool { return p.upNext.id == "2" && p.upNext.m != nil })
		}
		sent(p.Web)
		near(p)
		done := false
		p.post(func() { done = true })
		until("near heard", func() bool { return done })
		until("the next opened or not", func() bool { return p.loading == false })
		return sent(p.Web)
	}

	got := strings.Join(run(6, []spotify.Track{song("1", "a"), song("2", "b")}, true), " ")
	if !strings.Contains(got, "vinyl.leave(1)") || !strings.Contains(got, "vinyl.load(2,null,6)") {
		t.Fatalf("overlapping: sent %q", got)
	}
	for name, c := range map[string]struct {
		secs  float64
		queue []spotify.Track
		wait  bool
	}{
		"same album": {6, []spotify.Track{song("1", "a"), song("2", "a")}, true},
		"off":        {0, []spotify.Track{song("1", "a"), song("2", "b")}, true},
	} {
		if cmds := run(c.secs, c.queue, c.wait); slices.ContainsFunc(cmds, func(s string) bool { return strings.HasPrefix(s, "vinyl.leave(") }) {
			t.Fatalf("%s: sent %q", name, cmds)
		}
	}
}

// A song paused in its last second goes on from there when songs overlap,
// where without crossfade it starts again.
func TestResumeNearTheEnd(t *testing.T) {
	for _, c := range []struct {
		crossfade float64
		at        time.Duration // the page's time, of a minute
		seeks     bool
	}{{0, 59500 * time.Millisecond, true}, {6, 59500 * time.Millisecond, false}, {6, time.Minute, false}} {
		p, _, until := aheadPlayer(t)
		p.SetCrossfade(c.crossfade)
		p.PlayQueue([]spotify.Track{{ID: "1", Album: spotify.Album{ID: "a"}}, {ID: "2", Album: spotify.Album{ID: "b"}}}, 0)
		until("track 1 playing", func() bool { return p.out != nil })
		out := p.out.(*webOut)
		out.event("time", c.at, "", 0)
		out.event("pause", c.at, "", 0)
		p.playing, out.playing = false, false
		sent(p.Web)
		p.TogglePlay()
		got := strings.Join(sent(p.Web), " ")
		if strings.Contains(got, "vinyl.seek(") != c.seeks {
			t.Fatalf("crossfade %v at %v: sent %q", c.crossfade, c.at, got)
		}
	}
}

// With Spotify Connect the song after is asked for as at the end of one, so
// that Spotify's state moves on too: loading it by itself left Spotify at the
// song before, which a pause then went back to.
func TestOverlapAdvancesSpotifyConnect(t *testing.T) {
	p, _, until := aheadPlayer(t)
	p.SetCrossfade(6)
	var asked []string
	p.Control = func(action string, value any) bool {
		asked = append(asked, action)
		if action == "advance" {
			p.Control = nil
			p.load(1)
		}
		return action == "advance"
	}
	p.PlayQueue([]spotify.Track{{ID: "1", Album: spotify.Album{ID: "a"}}, {ID: "2", Album: spotify.Album{ID: "b"}}}, 0)
	until("track 1 playing", func() bool { return p.out != nil })
	until("track 2 resolved ahead", func() bool { return p.upNext.id == "2" && p.upNext.m != nil })
	sent(p.Web)
	done := false
	p.out.(*webOut).event("near", 0, "", 0)
	p.post(func() { done = true })
	until("near heard", func() bool { return done })
	until("track 2 playing", func() bool { return p.out != nil && p.track.ID == "2" })
	if got := strings.Join(sent(p.Web), " "); !strings.Contains(got, "vinyl.leave(1)") || !slices.Contains(asked, "advance") {
		t.Fatalf("asked %v, sent %q", asked, got)
	}
}
