package lyrics

import (
	_ "embed"
	"strings"
	"sync"
	"unicode/utf8"
)

// stPhrases and stCharacters are OpenCC's simplified-to-traditional phrase and
// character tables, the commonest candidate first (Apache-2.0, licenses/OpenCC-LICENSE).
//
//go:embed STPhrases.txt
var stPhrases string

//go:embed STCharacters.txt
var stCharacters string

// twVariants is OpenCC's table of the forms Taiwan writes (為, 著, 裡 rather
// than 爲, 着, 裏), applied after the two above as OpenCC's s2tw does.
//
//go:embed TWVariants.txt
var twVariants string

type s2tDict struct {
	chars         map[rune]rune
	phrases       map[string][]rune
	maxLenByFirst map[rune]int
	tw            map[rune]rune
}

func parseOpenCC(table string, yield func(from string, candidates []string)) {
	for _, rawLine := range strings.Split(table, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "\t")
		if !ok || k == "" {
			continue
		}
		to := strings.Fields(v)
		if len(to) == 0 {
			continue
		}
		yield(k, to)
	}
}

var loadS2T = sync.OnceValue(func() *s2tDict {
	d := &s2tDict{
		chars:         make(map[rune]rune, 4096),
		phrases:       make(map[string][]rune, 50000),
		maxLenByFirst: make(map[rune]int, 16384),
		tw:            make(map[rune]rune, 64),
	}
	parseOpenCC(twVariants, func(k string, to []string) {
		if kr, vr := []rune(k), []rune(to[0]); len(kr) == 1 && len(vr) == 1 {
			d.tw[kr[0]] = vr[0]
		}
	})
	parseOpenCC(stCharacters, func(k string, to []string) {
		if kr := []rune(k); len(kr) == 1 {
			if vr := []rune(to[0]); len(vr) == 1 {
				d.chars[kr[0]] = vr[0]
			}
		}
	})
	parseOpenCC(stPhrases, func(k string, to []string) {
		n := utf8.RuneCountInString(k)
		if n < 2 {
			return
		}
		d.phrases[k] = []rune(to[0])
		first, _ := utf8.DecodeRuneInString(k)
		if n > d.maxLenByFirst[first] {
			d.maxLenByFirst[first] = n
		}
	})
	return d
})

// Traditional converts Simplified Chinese text to Traditional Chinese as
// Taiwan writes it: OpenCC phrase-level longest match (STPhrases) with
// single-character fallback (STCharacters), then Taiwan's variant forms
// (TWVariants), without regional vocabulary substitution.
func Traditional(s string) string {
	if s == "" {
		return ""
	}
	return string(convertS2TRunes([]rune(s)))
}

func convertS2TRunes(in []rune) []rune {
	d := loadS2T()
	out := make([]rune, 0, len(in))
	for i := 0; i < len(in); {
		if maxL := d.maxLenByFirst[in[i]]; maxL >= 2 {
			limit := min(maxL, len(in)-i)
			matched := false
			for l := limit; l >= 2; l-- {
				if repl, ok := d.phrases[string(in[i:i+l])]; ok {
					out = append(out, repl...)
					i += l
					matched = true
					break
				}
			}
			if matched {
				continue
			}
		}
		if r, ok := d.chars[in[i]]; ok {
			out = append(out, r)
		} else {
			out = append(out, in[i])
		}
		i++
	}
	for i, r := range out {
		if v, ok := d.tw[r]; ok {
			out[i] = v
		}
	}
	return out
}

func convertWordsToTraditional(words []Word) {
	if len(words) == 0 {
		return
	}
	lengths := make([]int, len(words))
	total := 0
	for i, w := range words {
		n := utf8.RuneCountInString(w.Text)
		lengths[i] = n
		total += n
	}
	if total == 0 {
		return
	}
	all := make([]rune, 0, total)
	for _, w := range words {
		all = append(all, []rune(w.Text)...)
	}
	converted := convertS2TRunes(all)
	if len(converted) != len(all) {
		for i := range words {
			words[i].Text = Traditional(words[i].Text)
		}
		return
	}
	offset := 0
	for i, n := range lengths {
		words[i].Text = string(converted[offset : offset+n])
		offset += n
	}
}

// ToTraditional converts Chinese lyrics (line text, word tokens, and any
// translation) to Traditional Chinese characters via phrase-level OpenCC s2t.
func ToTraditional(lines []Line) {
	for i := range lines {
		lines[i].Text = Traditional(lines[i].Text)
		if lines[i].Translation != "" {
			lines[i].Translation = Traditional(lines[i].Translation)
		}
		convertWordsToTraditional(lines[i].Words)
	}
}

// ToTraditionalTranslations converts only Line.Translation to Traditional
// Chinese characters via phrase-level OpenCC s2t, leaving foreign-language
// lyrics intact.
func ToTraditionalTranslations(lines []Line) {
	for i := range lines {
		if lines[i].Translation != "" {
			lines[i].Translation = Traditional(lines[i].Translation)
		}
	}
}
