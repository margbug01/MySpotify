package l10n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

type qaViolation struct {
	rule string
	msg  string
}

type blacklistedTerm struct {
	Term       string `json:"term"`
	Suggestion string `json:"suggestion"`
}

var loadZhHantBlacklist = sync.OnceValue(func() []blacklistedTerm {
	data, err := os.ReadFile(filepath.Join("testdata", "zh_hant_blacklist.json"))
	if err != nil {
		panic(err)
	}
	var terms []blacklistedTerm
	if err := json.Unmarshal(data, &terms); err != nil {
		panic(err)
	}
	if len(terms) == 0 {
		panic("zh_hant_blacklist.json is empty")
	}
	return terms
})

// loadStrictSimplified maps each character OpenCC's STCharacters.txt converts
// to something other than itself, so is never written in Traditional Chinese.
var loadStrictSimplified = sync.OnceValue(func() map[rune]rune {
	data, err := os.ReadFile(filepath.Join("..", "lyrics", "STCharacters.txt"))
	if err != nil {
		panic(err)
	}
	m := make(map[rune]rune, 4096)
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		to := strings.Fields(v)
		if kr := []rune(k); ok && len(kr) == 1 && len(to) > 0 && !slices.Contains(to, k) {
			m[kr[0]] = []rune(to[0])[0]
		}
	}
	return m
})

var (
	rePrintfVerb      = regexp.MustCompile(`%(?:\[[0-9]+\])?[-+ #0]*(?:[0-9]+)?(?:\.[0-9]+)?(?:\[[0-9]+\])?[a-zA-Z]`)
	reICUPlaceholder  = regexp.MustCompile(`\{[a-zA-Z0-9_]+\}`)
	reURL             = regexp.MustCompile(`(?:https?|socks5?)://[^\s、，。；：！？）」』]+`)
	reCurlyQuotes     = regexp.MustCompile(`[\x{201c}\x{201d}\x{2018}\x{2019}]`)
	reHan             = regexp.MustCompile(`\p{Han}`)
	reHalfWidthParens = regexp.MustCompile(`[()]`)
	reHalfWidthPunct  = regexp.MustCompile(`[\p{Han}][,?!;:]|[,?!;:][\p{Han}]|[a-zA-Z0-9][,?!;](?:\s|[\p{Han}]|$)|[a-zA-Z0-9]:(?:\s|$)`)
	rePanguHanASCII   = regexp.MustCompile(`[\p{Han}][a-zA-Z0-9]|[a-zA-Z0-9][\p{Han}]`)
	reASCIIWord       = regexp.MustCompile(`[a-zA-Z]{3,}`)
)

func extractPlaceholders(s string) []string {
	cleaned := strings.ReplaceAll(s, "%%", "")
	var out []string
	out = append(out, rePrintfVerb.FindAllString(cleaned, -1)...)
	out = append(out, reICUPlaceholder.FindAllString(cleaned, -1)...)
	slices.Sort(out)
	return out
}

func stripForTextChecks(s string) string {
	cleaned := strings.ReplaceAll(s, "%%", "")
	cleaned = rePrintfVerb.ReplaceAllString(cleaned, "")
	cleaned = reICUPlaceholder.ReplaceAllString(cleaned, "")
	cleaned = reURL.ReplaceAllString(cleaned, "")
	return cleaned
}

func hasBareProxyTerm(target string) bool {
	rest := target
	for {
		idx := strings.Index(rest, "代理")
		if idx < 0 {
			return false
		}
		after := rest[idx+len("代理"):]
		if !strings.HasPrefix(after, "伺服器") {
			return true
		}
		rest = after
	}
}

func hasLoneEllipsis(target string) bool {
	runes := []rune(target)
	for i := 0; i < len(runes); {
		if runes[i] != '…' {
			i++
			continue
		}
		j := i
		for j < len(runes) && runes[j] == '…' {
			j++
		}
		if j-i != 2 {
			return true
		}
		i = j
	}
	return false
}

type simplifiedPair struct {
	simp rune
	trad rune
}

func validateZhHantEntry(source, target string) []qaViolation {
	var out []qaViolation
	if strings.TrimSpace(target) == "" {
		return []qaViolation{{rule: "EmptyTranslation", msg: "translation is empty"}}
	}

	strictSimp := loadStrictSimplified()
	var simp []simplifiedPair
	seenSimp := make(map[rune]bool)
	for _, r := range target {
		if trad, ok := strictSimp[r]; ok && !seenSimp[r] {
			seenSimp[r] = true
			simp = append(simp, simplifiedPair{simp: r, trad: trad})
		}
	}

	srcPH := extractPlaceholders(source)
	tgtPH := extractPlaceholders(target)
	if !slices.Equal(srcPH, tgtPH) {
		out = append(out, qaViolation{
			rule: "PlaceholderMismatch",
			msg:  fmt.Sprintf("placeholders %v != source %v", tgtPH, srcPH),
		})
	}

	if strings.TrimSpace(source) == strings.TrimSpace(target) {
		strippedSrc := stripForTextChecks(source)
		if (reASCIIWord.MatchString(strippedSrc) && !strings.Contains(strippedSrc, "_")) || len(simp) > 0 {
			out = append(out, qaViolation{
				rule: "Untranslated",
				msg:  "target equals source with untranslated text",
			})
		}
	}

	if len(simp) > 0 {
		parts := make([]string, 0, len(simp))
		for _, p := range simp {
			parts = append(parts, fmt.Sprintf("%c->%c", p.simp, p.trad))
		}
		out = append(out, qaViolation{
			rule: "SimplifiedCharacter",
			msg:  fmt.Sprintf("contains simplified characters: %s", strings.Join(parts, ", ")),
		})
	}

	for _, item := range loadZhHantBlacklist() {
		if strings.Contains(target, item.Term) {
			out = append(out, qaViolation{
				rule: "BlacklistedTerm",
				msg:  fmt.Sprintf("contains %q (use %q)", item.Term, item.Suggestion),
			})
		}
	}
	if hasBareProxyTerm(target) {
		out = append(out, qaViolation{
			rule: "BlacklistedTerm",
			msg:  `contains bare "代理" (use "代理伺服器")`,
		})
	}

	if reCurlyQuotes.MatchString(target) {
		out = append(out, qaViolation{
			rule: "InvalidQuotes",
			msg:  "contains curly quotes; use 「」 or 『』",
		})
	}

	stripped := stripForTextChecks(target)
	if reHan.MatchString(stripped) && (reHalfWidthParens.MatchString(stripped) || reHalfWidthPunct.MatchString(stripped)) {
		out = append(out, qaViolation{
			rule: "HalfWidthPunctuation",
			msg:  "contains half-width punctuation or parentheses in Chinese text",
		})
	}

	if strings.Contains(target, "...") || strings.Contains(target, "。。。") || hasLoneEllipsis(target) {
		out = append(out, qaViolation{
			rule: "InvalidEllipsis",
			msg:  "uses non-standard ellipsis; use ……",
		})
	}

	if rePanguHanASCII.MatchString(stripped) {
		out = append(out, qaViolation{
			rule: "PanguSpacingMissing",
			msg:  "missing Pangu space between Han and ASCII letter/digit",
		})
	}

	return out
}

// The zh-Hant catalog must pass every terminology, placeholder, quote, punctuation, ellipsis, and Pangu spacing QA rule.
func TestZhHantCatalogQA(t *testing.T) {
	source := readCatalog(t, "zh-Hans")
	target := readCatalog(t, "zh-Hant")

	for key, srcVal := range source {
		if strings.HasPrefix(key, "@") {
			continue
		}
		tgtVal, ok := target[key]
		if !ok {
			t.Errorf("missing zh-Hant key %q", key)
			continue
		}
		for _, v := range validateZhHantEntry(srcVal, tgtVal) {
			t.Errorf("key %q (%q): [%s] %s", key, tgtVal, v.rule, v.msg)
		}
	}
	for key := range target {
		if strings.HasPrefix(key, "@") {
			continue
		}
		if _, ok := source[key]; !ok {
			t.Errorf("extra zh-Hant key %q not in source catalog", key)
		}
	}
}

// QA rules catch blacklisted terms, curly quotes, half-width punctuation, non-standard ellipses, missing Pangu spaces, and bad placeholders without false-alarming on URLs, brands, or protocols.
func TestZhHantQARules(t *testing.T) {
	negativeCases := []struct {
		name      string
		source    string
		target    string
		wantRules []string
	}{
		{"empty", "关闭", "   ", []string{"EmptyTranslation"}},
		{"untranslated", "Check for updates", "Check for updates", []string{"Untranslated"}},
		{"untranslated-simp-close", "关闭", "关闭", []string{"Untranslated", "SimplifiedCharacter"}},
		{"untranslated-simp-default", "默认", "默认", []string{"Untranslated", "SimplifiedCharacter"}},
		{"mixed-simplified", "默认设置", "預設设置", []string{"SimplifiedCharacter"}},
		{"placeholder-printf", "已用 %.1f GB", "已使用 %d GB", []string{"PlaceholderMismatch"}},
		{"placeholder-icu", "{count} 首", "{n} 首", []string{"PlaceholderMismatch"}},
		{"blacklist-default", "默认", "默認設定", []string{"BlacklistedTerm"}},
		{"blacklist-cache", "缓存", "緩存空間", []string{"BlacklistedTerm"}},
		{"blacklist-playlist", "歌单", "我的歌單", []string{"BlacklistedTerm"}},
		{"blacklist-queue", "播放队列", "佇列", []string{"BlacklistedTerm"}},
		{"blacklist-proxy-bare", "代理", "系統代理", []string{"BlacklistedTerm"}},
		{"blacklist-opencc", "界面", "使用者界麵", []string{"BlacklistedTerm"}},
		{"curly-quotes", "已添加", "已加入“已按讚的歌曲”", []string{"InvalidQuotes"}},
		{"half-width-comma", "你好，世界", "你好, 世界", []string{"HalfWidthPunctuation"}},
		{"half-width-colon", "来源：", "來源:", []string{"HalfWidthPunctuation"}},
		{"half-width-colon-ascii", "Connect: 请重新登录", "Connect: 請重新登入", []string{"HalfWidthPunctuation"}},
		{"half-width-parens", "高音质 (256k)", "高音質 (256k)", []string{"HalfWidthPunctuation"}},
		{"ascii-ellipsis", "正在加载…", "正在載入...", []string{"InvalidEllipsis"}},
		{"lone-ellipsis", "正在加载…", "正在載入…", []string{"InvalidEllipsis"}},
		{"triple-ellipsis", "正在加载…", "正在載入………", []string{"InvalidEllipsis"}},
		{"pangu-missing-word", "打开 Spotify", "開啟Spotify", []string{"PanguSpacingMissing"}},
		{"pangu-missing-num", "共 5 首", "共5首", []string{"PanguSpacingMissing"}},
	}
	for _, tc := range negativeCases {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			violations := validateZhHantEntry(tc.source, tc.target)
			for _, wantRule := range tc.wantRules {
				found := false
				for _, v := range violations {
					if v.rule == wantRule {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("validateZhHantEntry(%q, %q) = %+v, want rule %q", tc.source, tc.target, violations, wantRule)
				}
			}
		})
	}

	positiveCases := []struct {
		name   string
		source string
		target string
	}{
		{"brand-and-pangu", "已登录 Spotify", "已登入 Spotify"},
		{"url-and-protocol", "代理说明", "請使用 https://example.com:8443/v1?a=1,2 或 socks5://127.0.0.1:7890 代理伺服器"},
		{"time-and-version", "每天 6:00 更新", "每天 6:00 更新"},
		{"proxy-full-term", "代理地址", "代理伺服器位址"},
		{"printf-formats", "歌词%s %.1f 秒（偏移 %+.1f 秒）", "歌詞%s %.1f 秒（偏移 %+.1f 秒）"},
		{"printf-percent-literal", "%d%%，装好会自己重新打开", "%d%%，安裝完成後會自動重新啟動"},
		{"icu-count", "{count} 首", "{count} 首"},
		{"compact-units", "%.1f亿", "%.1f億"},
		{"corner-quotes-ellipsis", "正在删除…", "確定要刪除「最愛」嗎……"},
		{"parenthesized-bitrate", "高音质 (256k)", "高音質（256k）"},
		{"full-width-parens-shortcut", "歌词提前 0.5 秒 ([)", "歌詞提前 0.5 秒（[）"},
		{"full-width-colon-ascii", "Connect: 请重新登录", "Connect：請重新登入"},
		{"identical-play", "播放", "播放"},
		{"identical-saved", "已收藏", "已收藏"},
	}
	for _, tc := range positiveCases {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			if violations := validateZhHantEntry(tc.source, tc.target); len(violations) != 0 {
				t.Errorf("validateZhHantEntry(%q, %q) unexpected violations: %+v", tc.source, tc.target, violations)
			}
		})
	}
}
