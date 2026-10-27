package player

import (
	"errors"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"slices"
	"time"

	"github.com/egoist/mygo"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// Output plays a track's audio: the web player's (see Web), or in tests
// package audio's.
type Output interface {
	Play()
	Pause()
	Playing() bool
	Seek(time.Duration)
	Position() time.Duration
	Duration() time.Duration
	OnEnd(func()) (off func())
	SetVolume(float64)
	Close()
}

// Bands is how many bands of pitch Levels splits a spectrum into.
const Bands = 24

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatAll
	RepeatOne
)

// Player plays a queue of tracks. Its methods run on the main thread;
// loading happens on goroutines, whose results post brings back there.
type Player struct {
	post func(func())
	// Web plays what loads; without it, as in tests, nothing does.
	Web     *Web
	Cache   *Cache
	NewWeb  func() (*Web, error)
	idle    *time.Timer
	idleGen int
	// Changed, if set, hears that what plays, where it is or whether it
	// plays changed.
	Changed func()
	Control func(string, any) bool

	queue   []spotify.Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  RepeatMode
	volume  float64

	// A radio goes on with the tracks its feed gives, as the queue nears
	// its end, for as long as nothing else is played.
	radio    string
	feed     func(got func([]spotify.Track))
	radioGen int  // counts radios, to drop what an old one gives
	feeding  bool // the feed is asked
	awaiting bool // at the queue's end, for what the feed gives

	track   *spotify.Track
	out     Output
	fade    fader
	loading bool
	playing bool // what the listener wants: false while paused
	err     string
	gen     int // counts loads, to drop the results of stale ones
	skipped int // the tracks skipped, as they could not play, before this one

	// Spotify turned the track away for asking too often: it loads again
	// once the wait is over, if it is to play then.
	waiting   bool
	throttles int // the times in a row, to wait longer each

	lyrics       []lyrics.Line
	lyricsLoaded bool
	// ahead are the lyrics of the track after this one, fetched as this
	// one plays: LRCLIB, busy, can take seconds.
	ahead struct {
		id    string
		lines []lyrics.Line
	}
	cover []byte // the cover the system shows of the track, once loaded

	// pending holds a seek asked for while the track loads.
	pending time.Duration
	// A seek playing fades out, then lands, then fades in: meanwhile the
	// position is where it goes.
	seeking bool
	seekTo  time.Duration
	seekGen int
}

func New(post func(func())) *Player {
	p := &Player{post: post, at: -1, volume: 0.8}
	// The media keys, and the controls of Control Center.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		switch cmd {
		case mygo.MediaPlay, mygo.MediaPause:
			if (cmd == mygo.MediaPlay) != p.playing {
				p.TogglePlay()
			}
		case mygo.MediaNext:
			p.Next()
		case mygo.MediaPrevious:
			p.Prev()
		case mygo.MediaSeek:
			p.Seek(pos)
		}
		p.post(func() {})
	})
	return p
}

// sync shows the system, and Changed, where playback is.
func (p *Player) sync() {
	p.updateIdle()
	if p.Changed != nil {
		defer p.Changed()
	}
	if p.track == nil {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.SetPlayback(state, p.Position())
}

// announce shows the system the track, and its cover once loaded.
func (p *Player) announce(t *spotify.Track, artwork []byte) {
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{
		Title:    t.Name,
		Artist:   t.ArtistNames(),
		Album:    t.Album.Name,
		Artwork:  artwork,
		Duration: t.Length(),
		Position: p.Position(),
		State:    state,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	})
}

func (p *Player) ended() {
	if f, ok := p.out.(interface {
		Fault() (string, int, time.Duration)
	}); ok {
		if msg, licence, wait := f.Fault(); msg != "" {
			p.failed(licence, wait)
			return
		}
	}
	if p.control("advance", nil) {
		return
	}
	p.throttles = 0
	if p.repeat == RepeatOne {
		p.out.Seek(0)
		p.out.Play()
		return
	}
	if p.at+1 >= len(p.order) && p.feed != nil {
		p.awaitFeed()
		return
	}
	if p.at+1 >= len(p.order) && p.repeat == RepeatOff {
		p.playing = false
		p.sync()
		return
	}
	p.Next()
}

// failed goes on from a track the page could not play: licence is how the
// licence service last answered for it, wait how long it asked to wait.
func (p *Player) failed(licence int, wait time.Duration) {
	switch licence {
	case http.StatusTooManyRequests:
		p.throttled(wait)
	case http.StatusUnauthorized, http.StatusForbidden:
		// Turned down: the rest of the queue would be too, one after
		// another.
		p.err = "这首歌暂时无法播放"
		p.playing = false
		p.sync()
	default:
		// The page tried again, and could not: on to the next.
		p.err = "这首歌暂时无法播放"
		p.skip(p.at, p.skipped)
	}
}

// throttled waits as Spotify asked, wait, or longer each time if it did not
// say, then loads the track again from where it was.
func (p *Player) throttled(wait time.Duration) {
	if wait <= 0 {
		wait = 30 * time.Second << min(p.throttles, 4)
	}
	wait = min(wait, 30*time.Minute)
	p.throttles++
	from := p.Position()
	p.stop()
	p.pending, p.waiting = from, true
	p.err = "Spotify 暂时限流，稍后重试"
	log.Printf("player: throttled, again in %v", wait)
	gen := p.gen
	afterFunc(wait, func() {
		p.post(func() {
			if gen != p.gen || !p.waiting {
				return
			}
			p.waiting = false
			if p.playing {
				p.loadSkipping(p.at, p.skipped, true, p.pending)
			}
		})
	})
	p.sync()
}

// afterFunc runs fn after d, on a goroutine; tests run it when they like.
var afterFunc = func(d time.Duration, fn func()) { time.AfterFunc(d, fn) }

// skip goes on from the track at order[at], which cannot play, unless
// nothing in the queue can: skipped of them so far.
func (p *Player) skip(at, skipped int) {
	if skipped+1 >= len(p.order) {
		p.playing = false
		p.sync()
		return
	}
	p.loadSkipping((at+1)%len(p.order), skipped+1, p.playing, 0)
}

// PlayQueue replaces the queue with tracks and plays the one at start.
func (p *Player) PlayQueue(tracks []spotify.Track, start int) {
	if len(tracks) > 0 && p.control("play", QueueRequest{tracks, min(max(start, 0), len(tracks)-1)}) {
		return
	}
	if len(tracks) == 0 {
		return
	}
	p.endRadio()
	start = min(max(start, 0), len(tracks)-1)
	p.queue = append([]spotify.Track(nil), tracks...)
	p.order = make([]int, len(p.queue))
	for i := range p.order {
		p.order[i] = i
	}
	if p.shuffle {
		p.shuffleOrder(start)
		p.load(0)
		return
	}
	p.load(start)
}

// PlayRadio plays tracks in their order, then goes on with those feed
// gives as the queue nears its end, until something else is played. The
// radio's name says which it is. feed calls got on the main thread.
func (p *Player) PlayRadio(name string, tracks []spotify.Track, feed func(got func([]spotify.Track))) {
	if len(tracks) == 0 {
		return
	}
	shuffle := p.shuffle
	p.shuffle = false
	p.PlayQueue(tracks, 0)
	p.shuffle = shuffle
	p.AttachRadio(name, feed)
}

// RadioFrom makes the track playing the first of a radio's queue, which
// goes on with tracks, then with those feed gives: without starting it
// over.
func (p *Player) RadioFrom(name string, tracks []spotify.Track, feed func(got func([]spotify.Track))) {
	if p.track == nil {
		return
	}
	p.queue, p.order, p.at = []spotify.Track{*p.track}, []int{0}, 0
	p.appendTracks(tracks)
	p.AttachRadio(name, feed)
}

// AttachRadio makes the queue the radio name's, which feed goes on with,
// as after restoring a session that played it.
func (p *Player) AttachRadio(name string, feed func(got func([]spotify.Track))) {
	p.endRadio()
	p.radio, p.feed = name, feed
	p.refill()
}

// EndRadio stops the radio playing: the song playing plays on, but the
// songs the radio queued after it go, and it adds no more.
func (p *Player) EndRadio() {
	if p.radio == "" && p.feed == nil {
		return
	}
	awaiting := p.awaiting
	p.endRadio()
	if p.at >= 0 && p.at+1 < len(p.order) {
		kept := make([]spotify.Track, p.at+1)
		for i, k := range p.order[:p.at+1] {
			kept[i] = p.queue[k]
		}
		p.queue, p.order = kept, make([]int, len(kept))
		for i := range p.order {
			p.order[i] = i
		}
	}
	if awaiting {
		// At its end, waiting for more: nothing plays on.
		p.playing = false
		p.sync()
	}
}

func (p *Player) endRadio() {
	p.radioGen++
	p.radio, p.feed, p.feeding, p.awaiting = "", nil, false, false
}

// Radio returns the name of the radio playing, or "".
func (p *Player) Radio() string { return p.radio }

// refill asks the radio for more once the queue is near its end.
func (p *Player) refill() {
	if p.feed == nil || p.feeding || p.at < len(p.order)-2 {
		return
	}
	p.feeding = true
	gen := p.radioGen
	p.feed(func(more []spotify.Track) {
		if gen != p.radioGen {
			return
		}
		p.feeding = false
		p.appendTracks(more)
		switch {
		case !p.awaiting:
		case p.at+1 < len(p.order):
			p.awaiting = false
			p.load(p.at + 1)
		default:
			// The radio has nothing more: it stops at its end.
			p.awaiting, p.playing = false, false
			p.err = "电台暂时没有更多歌曲"
			p.sync()
		}
	})
}

// awaitFeed waits at the end of a radio's queue for what its feed gives.
func (p *Player) awaitFeed() {
	p.awaiting = true
	p.refill()
}

// appendTracks adds tracks to the end of the queue, but those in it.
func (p *Player) appendTracks(tracks []spotify.Track) {
	in := make(map[string]bool, len(p.queue))
	for _, t := range p.queue {
		in[t.ID] = true
	}
	for _, t := range tracks {
		if !in[t.ID] {
			in[t.ID] = true
			p.order = append(p.order, len(p.queue))
			p.queue = append(p.queue, t)
		}
	}
}

// PlayNext puts tr after the track playing, to play next: moved there if
// queued already. With nothing queued, it plays tr.
func (p *Player) PlayNext(tr spotify.Track) {
	if p.at < 0 || len(p.order) == 0 {
		p.PlayQueue([]spotify.Track{tr}, 0)
		return
	}
	if i := slices.IndexFunc(p.queue, func(t spotify.Track) bool { return t.ID == tr.ID }); i >= 0 {
		k := slices.Index(p.order, i)
		if k == p.at {
			return
		}
		p.order = slices.Delete(p.order, k, k+1)
		if k < p.at {
			p.at--
		}
		p.order = slices.Insert(p.order, p.at+1, i)
	} else {
		p.order = slices.Insert(p.order, p.at+1, len(p.queue))
		p.queue = append(p.queue, tr)
	}
	p.sync()
}

// AddToQueue puts tr at the end of the queue, and reports whether it did:
// not if queued already. With nothing queued, it plays tr.
func (p *Player) AddToQueue(tr spotify.Track) bool {
	if p.at < 0 || len(p.order) == 0 {
		p.PlayQueue([]spotify.Track{tr}, 0)
		return true
	}
	if slices.ContainsFunc(p.queue, func(t spotify.Track) bool { return t.ID == tr.ID }) {
		return false
	}
	p.order = append(p.order, len(p.queue))
	p.queue = append(p.queue, tr)
	p.sync()
	return true
}

// shuffleOrder shuffles the order, with first first.
func (p *Player) shuffleOrder(first int) {
	rand.Shuffle(len(p.order), func(i, j int) { p.order[i], p.order[j] = p.order[j], p.order[i] })
	for i, v := range p.order {
		if v == first {
			p.order[0], p.order[i] = p.order[i], p.order[0]
		}
	}
}

func (p *Player) ToggleShuffle() {
	if p.control("shuffle", !p.shuffle) {
		return
	}
	p.shuffle = !p.shuffle
	if len(p.order) == 0 {
		return
	}
	cur := p.order[max(p.at, 0)]
	if p.shuffle {
		p.shuffleOrder(cur)
		p.at = 0
	} else {
		for i := range p.order {
			p.order[i] = i
		}
		p.at = cur
	}
}

func (p *Player) CycleRepeat() {
	mode := (p.repeat + 1) % 3
	if p.control("repeat", mode) {
		return
	}
	p.repeat = mode
}

// Current returns the track playing or loading, or nil.
func (p *Player) Current() *spotify.Track { return p.track }

// Queue returns the tracks queued, in the order they were given.
func (p *Player) Queue() []spotify.Track { return p.queue }

// Playing reports what the listener wants: false while paused.
func (p *Player) Playing() bool { return p.playing }

// Loading reports whether the current track is still loading.
func (p *Player) Loading() bool { return p.loading || p.awaiting }

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool { return p.out != nil }

// Err returns why the current track did not play, or "".
func (p *Player) Err() string { return p.err }

func (p *Player) Shuffle() bool      { return p.shuffle }
func (p *Player) Repeat() RepeatMode { return p.repeat }

// Lyrics returns the current track's lyrics, and whether they have loaded.
func (p *Player) Lyrics() ([]lyrics.Line, bool) { return p.lyrics, p.lyricsLoaded }

// PlayOrder returns the queue's indices in play order, and where the
// current track is among them, -1 for none.
func (p *Player) PlayOrder() ([]int, int) { return p.order, p.at }

func (p *Player) Next() {
	if p.control("skip_next", nil) {
		return
	}
	if len(p.order) == 0 {
		return
	}
	at := p.at + 1
	if at >= len(p.order) {
		if p.feed != nil {
			p.stop()
			p.awaitFeed()
			return
		}
		at = 0
	}
	p.load(at)
}

func (p *Player) Prev() {
	if p.control("skip_prev", nil) {
		return
	}
	if len(p.order) == 0 {
		return
	}
	// As every player does: the start of this track, then the one before.
	if p.Position() > 3*time.Second {
		p.Seek(0)
		return
	}
	p.PrevTrack()
}

// PrevTrack plays the track before this one, however far this one has
// played, as flicking the cover back does.
func (p *Player) PrevTrack() {
	if p.control("skip_prev", nil) {
		return
	}
	if len(p.order) == 0 {
		return
	}
	at := p.at - 1
	if at < 0 {
		at = len(p.order) - 1
	}
	p.load(at)
}

// Jump plays the queue's track i.
func (p *Player) Jump(i int) {
	if i >= 0 && i < len(p.queue) && (p.control("jump", i) || p.control("play", QueueRequest{p.queue, i})) {
		return
	}
	for k, v := range p.order {
		if v == i {
			p.load(k)
			return
		}
	}
}

// TogglePlay pauses, fading out, or plays, fading in.
func (p *Player) TogglePlay() {
	if p.control("toggle", nil) {
		return
	}
	if p.out == nil {
		switch {
		case p.loading || p.waiting:
			// It plays once loaded, or not.
			p.playing = !p.playing
			p.sync()
		case len(p.order) > 0:
			p.loadSkipping(max(p.at, 0), 0, true, p.pending)
		}
		return
	}
	out := p.out
	if p.playing {
		p.playing = false
		p.fade.fade(0, fadeOut, func() {
			p.post(func() {
				if p.out == out && !p.playing {
					out.Pause()
					p.sync()
				}
			})
		})
		p.sync()
		return
	}
	p.playing = true
	if !p.seeking && !out.Playing() {
		if out.Position() >= out.Duration()-time.Second {
			out.Seek(0)
		}
		out.Play()
	}
	p.fade.fade(1, fadeIn, nil)
	p.sync()
}

// seekGap is how long a seek waits for what plays to fade out, which the
// audio player does over 5 ms, as it pauses: within the next buffer or
// two it sends the system.
const seekGap = 30 * time.Millisecond

// Seek moves to t. Playing, it pauses and plays again around the jump,
// which the audio player fades over a few milliseconds, rather than
// cutting from one sound to the other with a click.
func (p *Player) Seek(t time.Duration) {
	if p.control("seek", t) {
		return
	}
	if p.out == nil {
		p.pending = t
		return
	}
	out := p.out
	// Paused, it jumps at once; but a seek on its way paused it only for
	// the jump, and this one takes its place, playing on from there.
	if !out.Playing() && !p.seeking {
		out.Seek(t)
		p.sync()
		return
	}
	p.seekGen++
	gen := p.seekGen
	p.seeking, p.seekTo = true, t
	out.Pause()
	time.AfterFunc(seekGap, func() {
		p.post(func() {
			if gen != p.seekGen || p.out != out {
				return
			}
			p.seeking = false
			out.Seek(t)
			if p.playing || !p.fade.silent() {
				out.Play()
			}
			p.sync()
		})
	})
	p.sync()
}

func (p *Player) SetVolume(v float64) {
	if p.control("volume", v) {
		return
	}
	p.volume = v
	p.applyGain()
}

// applyGain sets the volume of what plays: the listener's, which the ear
// hears in powers.
func (p *Player) applyGain() {
	p.fade.setGain(p.volume * p.volume)
}

func (p *Player) Position() time.Duration {
	switch {
	case p.out == nil:
		return p.pending
	case p.seeking:
		return p.seekTo
	}
	return p.out.Position()
}

func (p *Player) Duration() time.Duration {
	if p.track == nil {
		return 0
	}
	return p.track.Length()
}

// Levels returns the spectrum of what plays now. The page decrypts and
// plays the audio out of reach, so it is a stand-in, as Spotify's own apps
// animate theirs: waves across the bands, neighbors rising together,
// moving with the position, and still while paused.
func (p *Player) Levels() ([Bands]float32, bool) {
	var lv [Bands]float32
	if p.out == nil || !p.playing {
		return lv, false
	}
	t := p.Position().Seconds()
	for i := range lv {
		f := float64(i) / (Bands - 1) // from the lowest band to the highest
		v := 0.55 - 0.25*f
		v += 0.28 * math.Sin(2*math.Pi*(1.7*t+1.3*f))
		v += 0.16 * math.Sin(2*math.Pi*(2.9*t-0.7*f)+1)
		v += 0.08 * math.Sin(2*math.Pi*(5.3*t+3.1*f)+2)
		lv[i] = float32(min(1, max(0.05, v)))
	}
	return lv, true
}

func (p *Player) updateIdle() {
	if p.Web == nil || p.playing || p.loading {
		if p.idle != nil {
			p.idle.Stop()
			p.idle = nil
		}
		p.idleGen++
		return
	}
	if p.idle != nil {
		return
	}
	p.idleGen++
	gen := p.idleGen
	p.idle = time.AfterFunc(5*time.Minute, func() {
		p.post(func() { p.releaseIdle(gen) })
	})
}

func (p *Player) releaseIdle(gen int) {
	if gen != p.idleGen || p.playing || p.loading || p.Web == nil {
		return
	}
	p.pending = p.Position()
	p.CloseWeb()
	p.sync()
}

func (p *Player) CloseWeb() {
	if p.idle != nil {
		p.idle.Stop()
		p.idle = nil
	}
	p.idleGen++
	p.stop()
	if p.Web != nil {
		p.Web.Close()
		p.Web = nil
	}
}

func (p *Player) stop() {
	if out := p.out; out != nil {
		// Paused first, it fades out over a few milliseconds, rather than
		// stopping with a click.
		out.Pause()
		time.AfterFunc(100*time.Millisecond, out.Close)
	}
	p.out = nil
	p.seeking = false
	p.fade.attach(nil, 1)
}

// load plays the track at order[at], skipping those that cannot play.
func (p *Player) load(at int) {
	p.loadSkipping(at, 0, true, 0)
}

// loadSkipping loads the track at order[at], to play or not, from from,
// skipping those that cannot play: skipped of them so far.
func (p *Player) loadSkipping(at, skipped int, play bool, from time.Duration) {
	gen, t := p.cue(at, play, from)
	p.loading, p.skipped = true, skipped
	p.updateIdle()
	resolveMedia := resolve
	go func() {
		m, err := resolveMedia(t.URI())
		p.post(func() {
			if gen != p.gen {
				return
			}
			p.loading = false
			if err == nil && p.Web == nil && p.NewWeb != nil {
				p.Web, err = p.NewWeb()
			}
			if err == nil && p.Web == nil {
				err = errNoWeb
			}
			if err != nil {
				log.Printf("player: %s: %v", t.URI(), err)
				var thr *spotify.Throttled
				if errors.As(err, &thr) {
					p.throttled(thr.Wait)
					return
				}
				p.err = "这首歌暂时无法播放"
				p.skip(at, skipped)
				return
			}
			p.out = p.Web.Open(m)
			// From its middle, as a session picked up again, it fades in.
			level := 1.0
			if p.pending > 0 {
				level = 0
			}
			p.fade.attach(p.out, level)
			p.applyGain()
			p.out.OnEnd(func() {
				p.post(func() {
					if gen == p.gen {
						p.ended()
					}
				})
			})
			if p.pending > 0 {
				p.out.Seek(p.pending)
				p.pending = 0
			}
			if p.playing {
				p.out.Play()
				p.fade.fade(1, fadeIn, nil)
			}
			p.sync()
		})
	}()
}

// cue makes the track at order[at] the current one, at from, to play or
// not, and loads its cover and lyrics but not its audio, which
// loadSkipping goes on to load. Its generation and track are returned.
func (p *Player) cue(at int, play bool, from time.Duration) (int, spotify.Track) {
	p.stop()
	p.gen++
	gen := p.gen
	p.at = at
	t := p.queue[p.order[at]]
	p.track = &t
	p.loading, p.playing, p.err = false, play, ""
	p.awaiting, p.waiting = false, false
	p.lyrics, p.lyricsLoaded = nil, false
	p.pending = from
	p.cover = nil
	p.announce(&t, nil)
	p.refill()

	go func() {
		cover, err := spotify.Fetch(spotify.Sized(t.Album.PicURL, 600))
		if err == nil {
			p.post(func() {
				if gen == p.gen {
					// The track as it is now: Connect may have named its
					// artist since.
					p.cover = cover
					p.announce(p.track, cover)
				}
			})
		}
	}()

	if p.ahead.id == t.ID && len(p.ahead.lines) > 0 {
		p.lyrics, p.lyricsLoaded = p.ahead.lines, true
		p.lyricsAhead()
		return gen, t
	}
	go func() {
		lines := lyricsOf(t)
		p.post(func() {
			if gen == p.gen {
				p.lyrics = lines
				p.lyricsLoaded = true
				p.lyricsAhead()
			}
		})
	}()
	return gen, t
}

// lyricsAhead fetches the lyrics of the track after this one, once this
// one's are in, so they show as it begins.
func (p *Player) lyricsAhead() {
	if !p.lyricsLoaded || p.at+1 >= len(p.order) {
		return
	}
	next := p.queue[p.order[p.at+1]]
	if next.Episode || p.ahead.id == next.ID {
		return
	}
	p.ahead.id, p.ahead.lines = next.ID, nil
	go func() {
		lines := lyricsOf(next)
		p.post(func() {
			if p.ahead.id == next.ID {
				p.ahead.lines = lines
			}
		})
	}()
}

var errNoWeb = errors.New("no web player to play in")

// resolve finds where a track's audio is; tests swap it.
var resolve = spotify.ResolveMedia

// Session is where playback was left, to pick up again.
type Session struct {
	Queue    []spotify.Track `json:"queue"`
	Order    []int           `json:"order"`
	At       int             `json:"at"`
	Shuffle  bool            `json:"shuffle"`
	Repeat   RepeatMode      `json:"repeat"`
	Position int64           `json:"position"` // milliseconds
	Radio    string          `json:"radio,omitempty"`
}

// Session returns where playback is, or false with nothing queued.
func (p *Player) Session() (Session, bool) {
	if p.track == nil || p.at < 0 {
		return Session{}, false
	}
	return Session{
		Queue:    p.queue,
		Order:    p.order,
		At:       p.at,
		Shuffle:  p.shuffle,
		Repeat:   p.repeat,
		Position: p.Position().Milliseconds(),
		Radio:    p.radio,
	}, true
}

// Restore picks up a session where it was left, paused. Its track's audio
// loads once it plays: loading asks Spotify for a licence, and starts the
// web view's Widevine module, which takes 60 MB.
func (p *Player) Restore(s Session) {
	n := len(s.Queue)
	if n == 0 || len(s.Order) != n || s.At < 0 || s.At >= n {
		return
	}
	seen := make([]bool, n)
	for _, i := range s.Order {
		if i < 0 || i >= n || seen[i] {
			return
		}
		seen[i] = true
	}
	p.queue, p.order = s.Queue, s.Order
	p.shuffle, p.repeat = s.Shuffle, s.Repeat%3
	p.cue(s.At, false, time.Duration(s.Position)*time.Millisecond)
}
