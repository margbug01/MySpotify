package lyrics

import (
	_ "embed"
	"strings"
	"sync"
	"unicode"
)

// tsCharacters is OpenCC's table of traditional characters and their
// simplified forms, the commonest first (Apache-2.0, licenses/OpenCC-LICENSE).
//
//go:embed TSCharacters.txt
var tsCharacters string

var simplified = sync.OnceValue(func() map[rune]rune {
	m := map[rune]rune{}
	for _, line := range strings.Split(tsCharacters, "\n") {
		k, v, _ := strings.Cut(line, "\t")
		to := strings.Fields(v)
		if kr := []rune(k); len(kr) == 1 && len(to) > 0 && len([]rune(to[0])) == 1 {
			m[kr[0]] = []rune(to[0])[0]
		}
	}
	return m
})

// Chinese reports whether lyrics are in Chinese: Han characters without
// Japanese kana or Korean hangul among them.
func Chinese(lines []Line) bool {
	han := false
	for _, l := range lines {
		for _, r := range l.Text {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
				return false
			}
			han = han || unicode.Is(unicode.Han, r)
		}
	}
	return han
}

// Simplify writes lyrics in simplified characters, as Spotify's Chinese
// ones may be in traditional.
//
// shortcut: character by character, so the few that simplify two ways
// take the commoner (乾坤 becomes 干坤); OpenCC's phrase table fixes those
// if listeners notice.
func Simplify(lines []Line) {
	m := simplified()
	conv := func(r rune) rune {
		if s, ok := m[r]; ok {
			return s
		}
		return r
	}
	for i := range lines {
		lines[i].Text = strings.Map(conv, lines[i].Text)
		for j := range lines[i].Words {
			lines[i].Words[j].Text = strings.Map(conv, lines[i].Words[j].Text)
		}
	}
}
