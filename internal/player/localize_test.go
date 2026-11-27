package player

import (
	"errors"
	"os"
	"testing"

	"github.com/egoist/mygo/i18n"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// Lyrics in another language take NetEase's lines with its translation,
// and keep their own when it has none or could not answer; Chinese ones
// are not looked up, and turn simplified unless the listener reads
// traditional characters.
func TestLyricsLocalized(t *testing.T) {
	defer func(old bool) { SimplifyLyrics = old }(SimplifyLyrics)
	previous := i18n.Locale()
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	i18n.SetLocaleFunc(func() string { return os.Getenv("MYGO_LOCALE") })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
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

// Effective zh-TW converts both Chinese lyrics (line and words) and foreign-language
// NetEase translations via phrase-level s2t, reacts immediately to runtime locale
// changes, and keeps non-zh-TW locales unchanged.
func TestLyricsLocalizedTraditional(t *testing.T) {
	defer func(old bool) { SimplifyLyrics = old }(SimplifyLyrics)
	SimplifyLyrics = true
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return os.Getenv("MYGO_LOCALE") })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })

	english := "[00:01.00]The club isn't the best place"
	japanese := "[00:01.00]桜の夜"
	netease := func(synced, translation string) func(spotify.Track) (string, string, error) {
		return func(spotify.Track) (string, string, error) { return synced, translation, nil }
	}

	for _, locale := range []string{"zh-Hant", "zh-TW", "zh-Hant-TW", "zh-HK"} {
		t.Setenv("MYGO_LOCALE", locale)
		cn := localize(spotify.Track{ID: "cn"}, lyrics.Build("[00:01.00]皇后之后头发干燥", "", ""), nil)
		if len(cn) == 0 || cn[0].Text != "皇后之後頭髮乾燥" {
			t.Fatalf("%s chinese text = %+v", locale, cn)
		}
		if len(cn[0].Words) < 4 || cn[0].Words[1].Text != "后" || cn[0].Words[3].Text != "後" {
			t.Fatalf("%s chinese words = %+v", locale, cn[0].Words)
		}

		en := localize(spotify.Track{ID: "en"}, lyrics.Build(english, "", ""), netease(english, "[00:01.00]皇后之后在视频里发现头发干燥"))
		if len(en) == 0 || en[0].Text != "The club isn't the best place" || en[0].Translation != "皇后之後在視頻裡發現頭髮乾燥" {
			t.Fatalf("%s english translation = %+v", locale, en)
		}

		jp := localize(spotify.Track{ID: "jp"}, lyrics.Build(japanese, "", ""), netease(japanese, "[00:01.00]在视频里"))
		if len(jp) == 0 || jp[0].Text != "桜の夜" || jp[0].Translation != "在視頻裡" {
			t.Fatalf("%s japanese translation = %+v", locale, jp)
		}
	}

	// Runtime switch from zh-Hans to zh-Hant takes effect immediately.
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	before := localize(spotify.Track{ID: "switch"}, lyrics.Build(english, "", ""), netease(english, "[00:01.00]皇后之后在视频里"))
	if len(before) == 0 || before[0].Translation != "皇后之后在视频里" {
		t.Fatalf("zh-Hans translation = %+v", before)
	}
	t.Setenv("MYGO_LOCALE", "zh-Hant")
	after := localize(spotify.Track{ID: "switch"}, lyrics.Build(english, "", ""), netease(english, "[00:01.00]皇后之后在视频里"))
	if len(after) == 0 || after[0].Translation != "皇后之後在視頻裡" {
		t.Fatalf("after switching to zh-Hant, translation = %+v", after)
	}
}
