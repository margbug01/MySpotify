package spotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// neteaseURL is NetEase Cloud Music's; tests swap it.
var neteaseURL = "https://music.163.com"

var lrcTimed = regexp.MustCompile(`(?m)^\[\d+:\d+`)

// NetEaseLyrics looks t up on NetEase Cloud Music by its name and first
// artist and gives the lyrics, timed or plain, of the song of t's title
// whose length is nearest t's, within 2 seconds: NetEase has most Chinese
// songs, of the same recordings as Spotify's, their lengths the same to the
// millisecond. Only the name, the artist and the length leave; NetEase's
// text is in simplified characters.
func NetEaseLyrics(t Track) (synced, plain string, err error) {
	if t.Episode || t.Name == "" || t.Length() <= 0 {
		return "", "", nil
	}
	q := t.Name
	if len(t.Artists) > 0 {
		q += " " + t.Artists[0].Name
	}
	var found struct {
		Result struct {
			Songs []struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Duration int64  `json:"dt"`
			} `json:"songs"`
		} `json:"result"`
	}
	// Not search/get/web, which turns a fifth search in a few seconds
	// down for minutes.
	if err := netease("/api/cloudsearch/pc?"+url.Values{"s": {q}, "type": {"1"}, "limit": {"10"}}.Encode(), &found); err != nil {
		return "", "", err
	}
	id, nearest := int64(0), int64(2001)
	for _, s := range found.Result.Songs {
		if d := max(s.Duration-t.Duration, t.Duration-s.Duration); d < nearest && sameTitle(s.Name, t.Name) {
			id, nearest = s.ID, d
		}
	}
	if id == 0 {
		return "", "", nil
	}
	var res struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
		PureMusic bool `json:"pureMusic"`
	}
	if err := netease("/api/song/lyric?id="+strconv.FormatInt(id, 10)+"&lv=1", &res); err != nil {
		return "", "", err
	}
	lyric := strings.TrimSpace(res.Lrc.Lyric)
	switch {
	case res.PureMusic || lyric == "":
		return "", "", nil
	case lrcTimed.MatchString(lyric):
		return lyric, "", nil
	}
	return "", lyric, nil
}

// netease gets path from NetEase into out, as its web page does, without
// signing in.
func netease(path string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, neteaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", "https://music.163.com/")
	res, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &LyricsError{"netease", res.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return err
	}
	// It turns some requests down with 200 and a code of its own.
	var status struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(b, &status); err != nil {
		return err
	}
	if status.Code != http.StatusOK {
		return &LyricsError{"netease", status.Code}
	}
	return json.Unmarshal(b, out)
}

// sameTitle reports whether two titles name the same song: the same
// letters, without what is in brackets or after " - ", or, in Chinese, as
// many characters, NetEase writing in simplified ones what Spotify may in
// traditional (梁山伯與茱麗葉, 梁山伯与茱丽叶).
func sameTitle(a, b string) bool {
	a, b = foldTitle(a), foldTitle(b)
	if a == "" || b == "" {
		return false
	}
	han := func(r rune) bool { return unicode.Is(unicode.Han, r) }
	return a == b || len([]rune(a)) == len([]rune(b)) && strings.IndexFunc(a, han) >= 0 && strings.IndexFunc(b, han) >= 0
}

func foldTitle(s string) string {
	s = strings.ToLower(s)
	if i := strings.Index(s, " - "); i > 0 {
		s = s[:i]
	}
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case strings.ContainsRune("([（【", r):
			depth++
		case strings.ContainsRune(")]）】", r):
			depth = max(depth-1, 0)
		case depth == 0 && (unicode.IsLetter(r) || unicode.IsNumber(r)):
			b.WriteRune(r)
		}
	}
	return b.String()
}
