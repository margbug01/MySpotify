package l10n

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
)

// Catalogs must parse and keep every source message and formatting argument.
func TestCatalogArguments(t *testing.T) {
	for _, lang := range []string{"zh-Hant", "en", "it", "ja"} {
		t.Run(lang, func(t *testing.T) { testCatalog(t, lang) })
	}
}

func readCatalog(t *testing.T, name string) map[string]string {
	t.Helper()
	b, err := files.ReadFile(name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var messages map[string]string
	if err := json.Unmarshal(b, &messages); err != nil {
		t.Fatal(err)
	}
	return messages
}

func testCatalog(t *testing.T, lang string) {
	source, english := readCatalog(t, "zh-Hans"), readCatalog(t, lang)
	if len(source) != len(english) {
		t.Fatalf("catalog lengths differ: %d / %d", len(source), len(english))
	}
	printf := regexp.MustCompile(`%(?:\[[0-9]+\])?[-+ #0]*(?:[0-9]+)?(?:\.[0-9]+)?(?:\[[0-9]+\])?([a-zA-Z%])`)
	verbs := func(s string) []string {
		var out []string
		for _, match := range printf.FindAllStringSubmatch(s, -1) {
			if match[1] != "%" {
				out = append(out, match[1])
			}
		}
		slices.Sort(out)
		return out
	}
	for key, text := range source {
		if strings.HasPrefix(key, "@") {
			continue
		}
		translated, ok := english[key]
		if !ok {
			t.Errorf("missing English message %q", key)
			continue
		}
		zh, err := i18n.ParseMessage(text)
		if err != nil {
			t.Fatalf("Chinese %q: %v", key, err)
		}
		en, err := i18n.ParseMessage(translated)
		if err != nil {
			t.Errorf("English %q: %v", key, err)
			continue
		}
		names := func(message *i18n.Message) []string {
			var out []string
			for _, arg := range message.Args() {
				out = append(out, arg.Name)
			}
			slices.Sort(out)
			return out
		}
		if !reflect.DeepEqual(names(zh), names(en)) {
			t.Errorf("ICU arguments differ for %q: %v / %v", key, zh.Args(), en.Args())
		}
		if !reflect.DeepEqual(verbs(text), verbs(translated)) {
			t.Errorf("printf arguments differ for %q", key)
		}
	}
}

// English counts distinguish a single song or episode from zero and many.
func TestCountForms(t *testing.T) {
	for _, tc := range []struct {
		key  string
		n    int
		want string
	}{
		{"songCount", 0, "0 songs"}, {"songCount", 1, "1 song"}, {"songCount", 2, "2 songs"},
		{"episodeCount", 1, "1 episode"}, {"episodeCount", 2, "2 episodes"},
		{"publicPlaylistCount", 1, "1 public playlist"}, {"morePlaylistCount", 1, "1 more playlist on Spotify"},
	} {
		if got := catalog.Format("en", tc.key, map[string]any{"count": tc.n}); got != tc.want {
			t.Errorf("%s(%d) = %q, want %q", tc.key, tc.n, got, tc.want)
		}
	}
	if got := catalog.Format("it", "songCount", map[string]any{"count": 1}); got != "1 brano" {
		t.Errorf("it songCount(1) = %q", got)
	}
	if got := catalog.Format("it", "songCount", map[string]any{"count": 2}); got != "2 brani" {
		t.Errorf("it songCount(2) = %q", got)
	}
	if got := catalog.Format("zh-Hant", "publicPlaylistCount", map[string]any{"count": 3}); got != "3 個公開播放清單" {
		t.Errorf("zh-Hant publicPlaylistCount(3) = %q", got)
	}
}

// Traditional Chinese locales report zh-TW and match the zh-Hant catalog while sharing Chinese formatting.
func TestLanguageAndChinese(t *testing.T) {
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return os.Getenv("MYGO_LOCALE") })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })

	for _, tc := range []struct {
		locale   string
		wantLang string
		wantZh   bool
		wantEn   bool
	}{
		{"zh-Hant", "zh-TW", true, false},
		{"zh-TW", "zh-TW", true, false},
		{"zh-Hant-TW", "zh-TW", true, false},
		{"zh-HK", "zh-TW", true, false},
		{"zh-Hant-HK", "zh-TW", true, false},
		{"zh-MO", "zh-TW", true, false},
		{"zh-Hans", "zh-CN", true, false},
		{"zh-Hans-TW", "zh-CN", true, false},
		{"zh-Hans-HK", "zh-CN", true, false},
		{"zh-CN", "zh-CN", true, false},
		{"zh", "zh-CN", true, false},
		{"en", "en", false, true},
		{"en-US", "en", false, true},
		{"it", "it", false, false},
		{"ja", "ja", false, false},
		{"ja-JP", "ja", false, false},
		{"", "zh-CN", true, false},
	} {
		t.Setenv("MYGO_LOCALE", tc.locale)
		if got := Language(); got != tc.wantLang {
			t.Errorf("Language() for %q = %q, want %q", tc.locale, got, tc.wantLang)
		}
		if got := Chinese(); got != tc.wantZh {
			t.Errorf("Chinese() for %q = %v, want %v", tc.locale, got, tc.wantZh)
		}
		if got := English(); got != tc.wantEn {
			t.Errorf("English() for %q = %v, want %v", tc.locale, got, tc.wantEn)
		}
	}

	for _, locale := range []string{"zh-Hant", "zh-TW", "zh-Hant-TW", "zh-HK", "zh-MO"} {
		t.Setenv("MYGO_LOCALE", locale)
		if got := T("我喜欢的音乐"); got != "已按讚的歌曲" {
			t.Errorf("T(\"我喜欢的音乐\") for %q = %q, want \"已按讚的歌曲\"", locale, got)
		}
	}
	t.Setenv("MYGO_LOCALE", "zh-Hans-TW")
	if got := T("我喜欢的音乐"); got != "我喜欢的音乐" {
		t.Errorf("T(\"我喜欢的音乐\") for zh-Hans-TW = %q, want Simplified", got)
	}
}
