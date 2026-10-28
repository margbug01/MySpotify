package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"vinyl/internal/lyrics"
)

// An episode's transcript is what the web player shows as it plays, read
// along: its sentences in the episode's own language, each with when each
// of its words is said. Speakers' names and chapters' titles come between
// them, and music is captioned: none of those are said.

var episodeID = regexp.MustCompile(`^[0-9A-Za-z]+$`)

// Transcript returns episode id's transcript as lines to show as it plays,
// nil if it has none.
func Transcript(id string) ([]lyrics.Line, error) {
	if !episodeID.MatchString(id) {
		return nil, fmt.Errorf("transcript: bad episode ID %q", id)
	}
	raw, err := spGet("/transcript-read-along/v2/episode/"+id, "application/json")
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return transcriptLines(raw)
}

type transcriptSentence struct {
	StartMs   int64  `json:"startMs"`
	Text      string `json:"text"`
	Highlight []struct {
		StartMs  int64 `json:"startMs"`
		NumChars int   `json:"numChars"`
	} `json:"highlight"`
}

// transcriptLast is how long the last word of a sentence is taken to last,
// at most: no time is given of its end, and the next sentence may come
// after a while.
const transcriptLast = 1500 * time.Millisecond

// transcriptLines turns read-along's JSON into lines, each word timed.
func transcriptLines(raw []byte) ([]lyrics.Line, error) {
	var body struct {
		Section []struct {
			StartMs int64 `json:"startMs"`
			Text    *struct {
				Sentence *transcriptSentence `json:"sentence"`
			} `json:"text"`
		} `json:"section"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("transcript: %w", err)
	}
	var sentences []transcriptSentence
	for _, s := range body.Section {
		if s.Text == nil || s.Text.Sentence == nil {
			continue
		}
		sentence := *s.Text.Sentence
		if sentence.StartMs == 0 {
			sentence.StartMs = s.StartMs
		}
		sentences = append(sentences, sentence)
	}
	sort.SliceStable(sentences, func(i, j int) bool { return sentences[i].StartMs < sentences[j].StartMs })
	timed := false
	var lines []lyrics.Line
	for _, s := range sentences {
		at := time.Duration(s.StartMs) * time.Millisecond
		timed = timed || at > 0
		text, words := sentenceWords(s)
		if text = strings.TrimSpace(text); text == "" {
			continue
		}
		lines = append(lines, lyrics.Line{At: at, Text: text, Words: words, Timed: true})
	}
	if !timed {
		for i := range lines {
			lines[i] = lyrics.Line{Text: lines[i].Text, Unsynced: true}
		}
		return lines, nil
	}
	for i := range lines {
		l := &lines[i]
		end := l.At + 6*time.Second
		if n := len(l.Words); n > 0 {
			end = max(l.Words[n-1].Start, l.At) + transcriptLast
		}
		if i+1 < len(lines) {
			end = min(end, lines[i+1].At)
		}
		l.End = max(end, l.At)
		if len(l.Words) == 0 {
			l.Words, l.Timed = lyrics.SpreadWords(l.Text, l.At, l.End), false
			continue
		}
		for j := range l.Words {
			if j+1 < len(l.Words) {
				l.Words[j].End = l.Words[j+1].Start
			} else {
				l.Words[j].End = l.End
			}
			l.Words[j].End = max(l.Words[j].End, l.Words[j].Start)
		}
	}
	return lines, nil
}

// sentenceWords returns s's text and its words as its highlights say, a
// run of its characters from a time each: the spaces after a word kept
// with it, but those between Chinese or Japanese characters, which
// read-along writes between every two, dropped. Without highlights, no
// words: the line's time is shared among them later.
func sentenceWords(s transcriptSentence) (string, []lyrics.Word) {
	text := []rune(s.Text)
	if len(s.Highlight) == 0 {
		return closeUp(text), nil
	}
	var words []lyrics.Word
	pos := 0
	for _, h := range s.Highlight {
		if pos >= len(text) {
			break
		}
		n := min(max(h.NumChars, 0), len(text)-pos)
		run := text[pos : pos+n]
		pos += n
		if strings.TrimSpace(string(run)) == "" {
			// A space: kept after the word before it, between words.
			if len(words) > 0 && pos < len(text) && !(wide(text[pos-n-1]) && wide(text[pos])) {
				words[len(words)-1].Text += " "
			}
			continue
		}
		words = append(words, lyrics.Word{Text: string(run), Start: time.Duration(h.StartMs) * time.Millisecond})
	}
	if pos < len(text) && len(words) > 0 {
		// Characters no highlight covered go with the last word.
		words[len(words)-1].Text += closeUp(text[pos:])
	}
	var b strings.Builder
	for _, w := range words {
		b.WriteString(w.Text)
	}
	return b.String(), words
}

// closeUp is text without the spaces between Chinese or Japanese
// characters.
func closeUp(text []rune) string {
	var b strings.Builder
	for i, r := range text {
		if unicode.IsSpace(r) && i > 0 && i+1 < len(text) && wide(text[i-1]) && wide(text[i+1]) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// wide reports whether r is written without spaces around it: a Chinese,
// Japanese or Korean character, or their punctuation.
func wide(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) ||
		r >= 0x3000 && r <= 0x303f || r >= 0xff00 && r <= 0xffef
}
