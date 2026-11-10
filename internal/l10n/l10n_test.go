package l10n

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
)

// Catalogs must parse and keep every source message and formatting argument.
func TestCatalogArguments(t *testing.T) {
	read := func(name string) map[string]string {
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
	source, english := read("zh-Hans"), read("en")
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
}
