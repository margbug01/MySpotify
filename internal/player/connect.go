package player

import (
	"time"
	"vinyl/internal/spotify"
)

type QueueRequest struct {
	Tracks []spotify.Track
	Start  int
}

func (p *Player) ConnectQueue(tracks []spotify.Track) {
	if len(tracks) == 0 || p.track == nil || p.track.URI() != tracks[0].URI() {
		return
	}
	renamed := p.track.ArtistNames() != tracks[0].ArtistNames()
	p.queue = append([]spotify.Track(nil), tracks...)
	p.order = make([]int, len(tracks))
	for index := range p.order {
		p.order[index] = index
	}
	p.at = 0
	p.track = &p.queue[0]
	if renamed {
		// Its artist named only now: the system, and what was played, hear.
		p.announce(p.track, p.cover)
		p.sync()
	}
	// What comes next may be known only now.
	p.lyricsAhead()
}

func (p *Player) ConnectOptions(options spotify.ConnectOptions) {
	if options.Shuffle != nil {
		p.shuffle = *options.Shuffle
	}
	if options.RepeatContext != nil && options.RepeatTrack != nil {
		p.repeat = RepeatOff
		if *options.RepeatContext {
			p.repeat = RepeatAll
		}
		if *options.RepeatTrack {
			p.repeat = RepeatOne
		}
	}
}

func (p *Player) control(action string, value any) bool {
	return p.Control != nil && p.Control(action, value)
}

func (p *Player) ApplyConnect(track spotify.Track, position time.Duration, paused bool) {
	control := p.Control
	p.Control = nil
	defer func() { p.Control = control }()
	_, remote := p.out.(*remoteOutput)
	if p.track != nil && p.track.URI() == track.URI() && !remote && (p.out != nil || p.loading) {
		if position >= 0 {
			p.Seek(position)
		}
		if p.playing == paused {
			p.TogglePlay()
		}
		return
	}
	p.endRadio()
	p.queue = []spotify.Track{track}
	p.order = []int{0}
	p.loadSkipping(0, 0, !paused, max(0, position))
}

func (p *Player) MirrorConnect(track spotify.Track, position time.Duration, playing bool) {
	output, remote := p.out.(*remoteOutput)
	if !remote || p.track == nil || p.track.URI() != track.URI() {
		p.endRadio()
		p.queue = []spotify.Track{track}
		p.order = []int{0}
		p.cue(0, false, 0)
		output = &remoteOutput{duration: track.Length()}
		p.out = output
	}
	output.position, output.at, output.playing = position, time.Now(), playing
	p.playing, p.loading = playing, false
	p.sync()
}

func (p *Player) StopConnect() { p.playing = false; p.stop(); p.sync() }

func (p *Player) IsRemote() bool { _, ok := p.out.(*remoteOutput); return ok }

type remoteOutput struct {
	position, duration time.Duration
	at                 time.Time
	playing            bool
}

func (o *remoteOutput) Play()                   {}
func (o *remoteOutput) Pause()                  {}
func (o *remoteOutput) Seek(time.Duration)      {}
func (o *remoteOutput) SetVolume(float64)       {}
func (o *remoteOutput) Close()                  {}
func (o *remoteOutput) Playing() bool           { return o.playing }
func (o *remoteOutput) Duration() time.Duration { return o.duration }
func (o *remoteOutput) OnEnd(func()) func()     { return func() {} }
func (o *remoteOutput) Position() time.Duration {
	position := o.position
	if o.playing {
		position += time.Since(o.at)
	}
	return min(max(0, position), o.duration)
}
