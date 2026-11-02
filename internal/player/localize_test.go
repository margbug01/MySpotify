package player

import (
	"errors"
	"testing"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// Lyrics in another language take NetEase's lines with its translation,
// and keep their own when it has none or could not answer; Chinese ones
// are not looked up, and turn simplified unless the listener reads
// traditional characters.
func TestLyricsLocalized(t *testing.T) {
	defer func(old bool) { SimplifyLyrics = old }(SimplifyLyrics)
	english := "[00:01.00]The club isn't the best place\n[00:04.00]So the bar is where I go"
	netease := func(synced, translation string, err error) func(spotify.Track) (string, string, error) {
		return func(spotify.Track) (string, string, error) { return synced, translation, err }
	}
	for _, c := range []struct {
		name, lrc   string
		simplify    bool
		translated  func(spotify.Track) (string, string, error)
		text, trans string
	}{
		{"translated", english, true, netease("[00:01.10]The club isn't the best place\n[00:04.10]So the bar is where I go", "[00:01.10]俱乐部不是最好的地方\n[00:04.10]所以我去酒吧", nil), "The club isn't the best place", "俱乐部不是最好的地方"},
		// NetEase's song of that title and length was another version, in
		// Russian, which replaced Spotify's English lines.
		{"other-version", english, true, netease("[00:01.10]Снег легко лица коснулся,\n[00:04.10]И вернулись дни той зимы.", "[00:01.10]雪轻抚着脸颊\n[00:04.10]回映着那个冬季的画面", nil), "The club isn't the best place", ""},
		{"no-translation", english, true, netease("", "", nil), "The club isn't the best place", ""},
		{"netease-down", english, true, netease("", "", errors.New("down")), "The club isn't the best place", ""},
		{"chinese", "[00:01.00]誰都只得那雙手", true, nil, "谁都只得那双手", ""},
		{"traditional-reader", "[00:01.00]誰都只得那雙手", false, nil, "誰都只得那雙手", ""},
	} {
		SimplifyLyrics = c.simplify
		lines := localize(spotify.Track{ID: "song"}, lyrics.Build(c.lrc, "", ""), c.translated)
		if len(lines) == 0 || lines[0].Text != c.text || lines[0].Translation != c.trans {
			t.Errorf("%s: %+v", c.name, lines)
		}
	}
}
