package spotify

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// NetEase is asked by name and artist only, and of its songs the one of
// the same title nearest in length gives the lyrics: not a cover a little
// shorter, a remix, or another song as long.
func TestNetEaseLyrics(t *testing.T) {
	old := Client
	defer func() { Client = old }()
	track := Track{Name: "梁山伯與茱麗葉", Artists: []Artist{{Name: "卓文萱"}}, Duration: 232896}
	songs := `{"code":200,"result":{"songs":[{"id":1,"name":"梁山伯与茱丽叶(Remix版)","dt":205213},{"id":2,"name":"梁山伯与茱丽叶（cover：曹格 卓文萱）","dt":230991},{"id":4,"name":"我的歌","dt":232896},{"id":3,"name":"梁山伯与茱丽叶","dt":232896}]}}`
	for _, c := range []struct {
		name, search, lyric string
		synced, plain       bool
		asked               string
		err                 error
	}{
		{"timed", songs, `{"code":200,"lrc":{"lyric":"[00:00.000] 作词 : 曹格\n[00:16.939]我的心想唱首歌给你听"}}`, true, false, "3", nil},
		{"plain", songs, `{"code":200,"lrc":{"lyric":"我的心想唱首歌给你听\n歌词是如此的甜蜜"}}`, false, true, "3", nil},
		{"pure", songs, `{"code":200,"pureMusic":true,"lrc":{"lyric":""}}`, false, false, "3", nil},
		{"far", `{"code":200,"result":{"songs":[{"id":1,"name":"梁山伯与茱丽叶","dt":240000}]}}`, "", false, false, "", nil},
		{"asked-too-often", `{"code":405,"message":"操作频繁"}`, "", false, false, "", &LyricsError{"netease", 405}},
		{"refused", `{"code":-460,"message":"Cheating"}`, "", false, false, "", &LyricsError{"netease", -460}},
	} {
		asked := ""
		Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != "music.163.com" || req.Header.Get("Referer") != "https://music.163.com/" || req.Header.Get("Authorization") != "" || req.Header.Get("client-token") != "" {
				t.Fatal("not as NetEase's page asks, or with Spotify's credentials")
			}
			body := c.search
			switch req.URL.Path {
			case "/api/cloudsearch/pc":
				if req.URL.Query().Get("s") != "梁山伯與茱麗葉 卓文萱" {
					t.Fatalf("searched %q", req.URL.Query().Get("s"))
				}
			case "/api/song/lyric":
				asked, body = req.URL.Query().Get("id"), c.lyric
			default:
				t.Fatal("asked", req.URL.Path)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		synced, plain, err := NetEaseLyrics(track)
		var le *LyricsError
		if (synced != "") != c.synced || (plain != "") != c.plain || asked != c.asked || (c.err == nil) != (err == nil) || c.err != nil && (!errors.As(err, &le) || *le != *c.err.(*LyricsError)) {
			t.Errorf("%s: synced %q plain %q, lyrics of %q, err %v", c.name, synced, plain, asked, err)
		}
	}
}

func TestSameTitle(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"梁山伯與茱麗葉", "梁山伯与茱丽叶", true},
		{"無緣", "无缘", true},
		{"周旋", "周旋 (Live)", true},
		{"Drenched", "drenched", true},
		{"Song - Remastered 2011", "Song", true},
		{"周旋", "旋转木马", false},
		{"Rain", "Pain", false},
		{"(Intro)", "(Intro)", false},
	} {
		if got := sameTitle(c.a, c.b); got != c.same {
			t.Errorf("sameTitle(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// NetEase asked too often, or a site busy, may answer later; a lookup
// turned down will not.
func TestLyricsErrorTemporary(t *testing.T) {
	for _, c := range []struct {
		err  LyricsError
		want bool
	}{
		{LyricsError{"lrclib", 503}, true},
		{LyricsError{"lrclib", 429}, true},
		{LyricsError{"netease", 405}, true},
		{LyricsError{"netease", 406}, true},
		{LyricsError{"lrclib", 400}, false},
		{LyricsError{"lrclib", 405}, false},
		{LyricsError{"netease", -460}, false},
	} {
		if got := c.err.Temporary(); got != c.want {
			t.Errorf("%v: temporary %v", c.err, got)
		}
	}
}
