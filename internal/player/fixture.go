package player

import (
	"time"

	"github.com/egoist/mygo/audio"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// These set a player up as it would be after loading, without loading
// anything, for the tests of the views that show it.

// Cue makes tracks the queue, in order, with the first current.
func (p *Player) Cue(tracks []spotify.Track) {
	p.queue = tracks
	p.order = make([]int, len(tracks))
	for i := range p.order {
		p.order[i] = i
	}
	p.at = 0
	p.track = &p.queue[0]
}

// SetLyrics gives the current track lines, as loaded.
func (p *Player) SetLyrics(lines []lyrics.Line) {
	p.lyrics, p.lyricsLoaded = lines, true
}

// Hold makes the player play at pos with nothing to play: its position
// stays there.
func (p *Player) Hold(pos time.Duration) {
	p.pending = pos
	p.playing = true
}

// Attach makes out the current track's audio, playing.
func (p *Player) Attach(out *audio.Player) {
	p.out = audioOut{out}
	p.playing = true
}

// audioOut is package audio's player as an Output.
type audioOut struct{ *audio.Player }

func (o audioOut) SetVolume(v float64) { o.Player.SetVolume(v) }
