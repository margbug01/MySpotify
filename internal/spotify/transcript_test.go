package spotify

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// transcriptFixture is an episode's transcript as read-along writes it, cut
// down: a chapter's title and a speaker's name, which are not said; a
// sentence in English, its words and the spaces after them each a
// highlight; one in Chinese, a space between every two characters, as
// read-along writes them, a mark in one with the character before it; one with no highlights; and music, captioned.
const transcriptFixture = `{
 "version": "1.0", "language": "en-us", "timeSyncedStatus": "SYLLABLE_SYNCED",
 "section": [
  {"startMs": 2280, "title": {"title": "Welcome & Why Asking Questions is Important"}},
  {"startMs": 2280, "text": {"sentence": {"startMs": 2280, "text": "English with Us podcast.", "highlight": [
   {"startMs": 2280, "numChars": 7}, {"startMs": 2800, "numChars": 1}, {"startMs": 2800, "numChars": 4}, {"startMs": 3080, "numChars": 1},
   {"startMs": 3120, "numChars": 2}, {"startMs": 3440, "numChars": 1}, {"startMs": 3440, "numChars": 8}]}}},
  {"startMs": 5000, "title": {"title": "Speaker 2"}},
  {"startMs": 5000, "title": {}},
  {"startMs": 5000, "text": {"sentence": {"startMs": 5000, "text": "各 位 听 众， 用 iPhone 听", "highlight": [
   {"startMs": 5000, "numChars": 1}, {"startMs": 5200, "numChars": 1}, {"startMs": 5200, "numChars": 1}, {"startMs": 5400, "numChars": 1},
   {"startMs": 5400, "numChars": 1}, {"startMs": 5600, "numChars": 1}, {"startMs": 5600, "numChars": 2}, {"startMs": 5800, "numChars": 1},
   {"startMs": 5800, "numChars": 1}, {"startMs": 6000, "numChars": 1}, {"startMs": 6000, "numChars": 6}, {"startMs": 6600, "numChars": 1},
   {"startMs": 6600, "numChars": 1}]}}},
  {"startMs": 20000, "text": {"sentence": {"startMs": 20000, "text": "No words timed."}}},
  {"startMs": 40000, "fallback": {"sentence": {"startMs": 0, "text": "Music playing", "highlight": [{"startMs": 40000, "numChars": 13}]}}, "musicClosedCaption": {"endMs": 47000, "text": "Music playing"}}
 ]
}`

// A transcript shows as lines, each word from when it is said: the spaces
// between Chinese characters gone, those between words and around Latin
// kept. A line lasts until the next, or a little past its last word if
// the next is a while away. Names, titles and music are not said.
func TestTranscriptLines(t *testing.T) {
	lines, err := transcriptLines([]byte(transcriptFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("%d lines: %+v", len(lines), lines)
	}
	en, zh, plain := lines[0], lines[1], lines[2]
	if en.Text != "English with Us podcast." || en.At != 2280*time.Millisecond || !en.Timed || en.Unsynced {
		t.Fatalf("english: %+v", en)
	}
	var texts []string
	for _, w := range en.Words {
		texts = append(texts, w.Text)
	}
	if strings.Join(texts, "|") != "English |with |Us |podcast." || en.Words[1].Start != 2800*time.Millisecond || en.Words[1].End != 3120*time.Millisecond {
		t.Fatalf("words %q, with from %v to %v", texts, en.Words[1].Start, en.Words[1].End)
	}
	// Its last word is said from 3.44 s: the line lasts 1.5 s more, short
	// of the next.
	if want := 3440*time.Millisecond + transcriptLast; en.End != want || en.Words[3].End != want {
		t.Fatalf("english ends %v, its last word %v; want %v", en.End, en.Words[3].End, want)
	}
	// A mark goes with the character before it, as read-along gives it.
	if zh.Text != "各位听众，用 iPhone 听" || len(zh.Words) != 7 || zh.Words[0].Text != "各" || zh.Words[3].Text != "众，" || zh.Words[4].Text != "用 " {
		t.Fatalf("chinese %q, words %+v", zh.Text, zh.Words)
	}
	if plain.Text != "No words timed." || plain.Timed || len(plain.Words) == 0 || plain.End != 26*time.Second {
		t.Fatalf("without highlights: %+v", plain)
	}
}

// A transcript with no times at all shows as text, unsynced.
func TestTranscriptWithoutTimes(t *testing.T) {
	lines, err := transcriptLines([]byte(`{"section": [{"text": {"sentence": {"text": "One."}}}, {"text": {"sentence": {"text": "Two."}}}]}`))
	if err != nil || len(lines) != 2 || !lines[0].Unsynced || lines[1].Text != "Two." {
		t.Fatalf("%v %+v", err, lines)
	}
}

// Transcript asks read-along for the episode's own: an episode without one
// has none, and an ID that is none is not asked for.
func TestTranscript(t *testing.T) {
	old := Client
	defer func() { Client = old; SetIdentity(Identity{}) }()
	SetIdentity(Identity{Token: "t", ClientToken: "c"})
	var asked []string
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		asked = append(asked, req.URL.Path)
		if req.URL.Path == "/transcript-read-along/v2/episode/5alKyFS45cSQQqO73zJZIb" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(transcriptFixture)), Header: http.Header{}}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})}
	lines, err := Transcript("5alKyFS45cSQQqO73zJZIb")
	if err != nil || len(lines) != 3 {
		t.Fatalf("%v, %d lines", err, len(lines))
	}
	if lines, err := Transcript("none"); err != nil || lines != nil {
		t.Fatalf("without one: %v, %v", err, lines)
	}
	if _, err := Transcript("../../color-lyrics"); err == nil || len(asked) != 2 {
		t.Fatalf("a bad ID: %v, asked %q", err, asked)
	}
}
