package player

import (
	"errors"
	"log"
	"slices"
	"strings"
	"time"
	"unicode"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

func trackLyrics(track spotify.Track) []lyrics.Line {
	if track.Episode {
		return episodeLyrics(track, transcriptOf)
	}
	lines := findLyrics(track, spotify.Lyrics, func(t spotify.Track) (string, string, error) {
		return lookupByNames(t, spotify.Decorate, func(t spotify.Track) (string, string, error) {
			return firstTimed(t, spotify.NetEaseLyrics, spotify.LRCLyrics, spotify.LRCSearch)
		})
	})
	return localize(track, lines, spotify.NetEaseTranslation)
}

// SimplifyLyrics has Chinese lyrics show in simplified characters; the app
// turns it off for listeners who read traditional ones.
var SimplifyLyrics = true

// localize has Chinese lyrics in simplified characters (SimplifyLyrics),
// and timed ones in other languages come with NetEase's translation into
// Chinese, its own lines taken where it has one: they pair with it line
// for line, as Spotify's need not.
func localize(track spotify.Track, lines []lyrics.Line, translated func(spotify.Track) (string, string, error)) []lyrics.Line {
	if lyrics.Chinese(lines) {
		if SimplifyLyrics {
			lyrics.Simplify(lines)
		}
		return lines
	}
	if len(lines) == 0 || lines[0].Unsynced {
		return lines
	}
	synced, translation, err := translated(track)
	if err != nil {
		log.Printf("lyrics: translation of %s: %v", track.ID, err)
		return lines
	}
	if tl := lyrics.Build(synced, translation, ""); slices.ContainsFunc(tl, func(l lyrics.Line) bool { return l.Translation != "" }) && sameWords(lines, tl) {
		return tl
	}
	return lines
}

// sameWords reports whether at least half of a's lines are b's too, case,
// spaces and punctuation aside: NetEase can have another version of a song
// under its title and near its length (Voyaging Star's Farewell, Russian
// ver.), whose lines would replace Spotify's.
func sameWords(a, b []lyrics.Line) bool {
	fold := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, s)
	}
	have := map[string]bool{}
	for _, l := range b {
		have[fold(l.Text)] = true
	}
	n, same := 0, 0
	for _, l := range a {
		if k := fold(l.Text); k != "" {
			n++
			if have[k] {
				same++
			}
		}
	}
	return n > 0 && 2*same >= n
}

// firstTimed asks sources in turn for t's lyrics until one has them timed,
// or else gives the first plain text one had: fails only when none had
// any, with the error worth asking again for if one was.
func firstTimed(t spotify.Track, sources ...func(spotify.Track) (string, string, error)) (synced, plain string, err error) {
	for _, source := range sources {
		s, p, e := source(t)
		if s != "" {
			return s, "", nil
		}
		if plain == "" {
			plain = p
		}
		if e != nil && (err == nil || transient(e) && !transient(err)) {
			err = e
		}
	}
	if plain != "" {
		return "", plain, nil
	}
	return "", "", err
}

// episodeLyrics is an episode's lyrics: its transcript, which shows as
// subtitles over its video, and in Now Playing as a song's lyrics do. It
// is asked again, as the lyrics sites are, when it could not be had; not
// when Spotify asked to wait.
func episodeLyrics(track spotify.Track, transcript func(string) ([]lyrics.Line, error)) []lyrics.Line {
	lines, err := transcript(track.ID)
	for _, wait := range lyricsWaits {
		var thr *spotify.Throttled
		if err == nil || errors.As(err, &thr) {
			break
		}
		time.Sleep(wait)
		lines, err = transcript(track.ID)
	}
	if err != nil {
		log.Printf("lyrics: transcript of %s: %v", track.ID, err)
		return nil
	}
	return lines
}

// transcriptOf is spotify.Transcript; tests swap it.
var transcriptOf = spotify.Transcript

// lyricsOf is trackLyrics; tests swap it.
var lyricsOf = trackLyrics

// lookupByNames looks t's lyrics up by the names Spotify gives in Vinyl's
// language. Connect names some tracks without their artist, and others in
// another language (Yitai Wang, not 王以太), which the lyrics sites do not
// find, or find only as plain text: those are looked up again by Spotify's
// names, the plain text kept if that finds none in time.
func lookupByNames(t spotify.Track, decorate func([]spotify.Track) ([]spotify.Track, error), lookup func(spotify.Track) (string, string, error)) (string, string, error) {
	renamed := t.Unnamed()
	if renamed {
		t = rename(t, decorate)
	}
	synced, plain, err := lookup(t)
	if err != nil || synced != "" || renamed {
		return synced, plain, err
	}
	again := rename(t, decorate)
	if again.Name == t.Name && again.ArtistNames() == t.ArtistNames() {
		return synced, plain, err
	}
	if s, p, e := lookup(again); s != "" || plain == "" {
		return s, p, e
	}
	return synced, plain, err
}

// rename is t named as Spotify names it to Vinyl, or t as it is.
func rename(t spotify.Track, decorate func([]spotify.Track) ([]spotify.Track, error)) spotify.Track {
	bare := t
	bare.Artists = nil
	if named, err := decorate([]spotify.Track{bare}); err == nil && !named[0].Unnamed() {
		return named[0]
	}
	return t
}

// lyricsWaits are how long the lyrics sites are waited for before they are
// asked again, when one could not answer: LRCLIB busy (503) more often
// than down. A song they do not have, or a lookup turned down, is not
// asked again.
var lyricsWaits = []time.Duration{3 * time.Second, 10 * time.Second}

func transient(err error) bool {
	var status *spotify.LyricsError
	if errors.As(err, &status) {
		return status.Temporary()
	}
	return err != nil
}

func findLyrics(track spotify.Track, primary func(string) (string, error), fallback func(spotify.Track) (string, string, error)) []lyrics.Line {
	if track.Episode {
		return nil
	}
	lrc, err := primary(track.ID)
	if err == nil {
		if lines := lyrics.Build(lrc, "", ""); len(lines) > 0 {
			return lines
		}
	}
	synced, plain, err := fallback(track)
	for _, wait := range lyricsWaits {
		if !transient(err) {
			break
		}
		time.Sleep(wait)
		synced, plain, err = fallback(track)
	}
	if err != nil {
		log.Printf("lyrics: lookup failed for %s: %v", track.ID, err)
		return nil
	}
	if lines := lyrics.Build(synced, "", ""); len(lines) > 0 {
		return lines
	}
	var lines []lyrics.Line
	for _, text := range strings.Split(plain, "\n") {
		if text = strings.TrimSpace(text); text != "" {
			lines = append(lines, lyrics.Line{Text: text, Unsynced: true})
		}
	}
	return lines
}
