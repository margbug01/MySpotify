package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

// readmeScene saves the pages the README shows, with real data, signed in
// as someone named Alex: nothing plays, Now Playing shows a session picked
// up where it was left. A new account has no daily mix: a featured list
// stands in for it.
func readmeScene(a *app, shot func(string, *mygo.Window), step func(time.Duration, func())) {
	const (
		album = "spotify:album:1dtg7IOdq2Q6zA4nIJ5Qsk" // 陳奕迅 What's Going On...?
		song  = "3cthRwQ2X0SNTaauD7tjn5"               // 富士山下
		eason = "spotify:artist:2QcZxAgcs2I1q7CtCkl6MI"
		show  = "spotify:show:3SAEaxDocqSHYNxvtDQ9b1"
	)
	theme := func(s string) {
		step(0, func() {
			a.settings.Appearance = s
			a.settings.applyAppearance()
		})
		time.Sleep(time.Second)
	}
	// Spotify names the listener in some of the home's titles too ("根据 …
	// 的喜好推荐"): Alex there as well, again before each page, the home
	// being fetched again meanwhile.
	var real string
	mygo.RunOnMain(func() { real = a.profile.Nickname })
	alias := func() {
		for i := range a.homeSections {
			if real != "" {
				a.homeSections[i].Title = strings.ReplaceAll(a.homeSections[i].Title, real, "Alex")
			}
		}
	}
	page := func(name string, r route, wait time.Duration) {
		step(0, func() { alias(); a.nowPlaying = false; a.open(r) })
		time.Sleep(wait)
		shot(name, a.win)
	}
	mygo.RunOnMain(func() {
		a.win.SetSize(1280, 820)
		a.profile = &spotify.Profile{UserID: a.profile.UserID, Nickname: "Alex", AvatarColor: 0x1ed760}
	})
	theme("light")

	al, _ := spotify.PlaylistDetail(album)
	var featured []spotify.Playlist
	mygo.RunOnMain(func() {
		for _, s := range a.homeSections {
			for _, it := range s.Items {
				if it.List != nil {
					featured = append(featured, *it.List)
				}
			}
		}
	})
	var daily []spotify.Track
	for _, f := range featured {
		if pl, err := spotify.PlaylistDetail(f.ID); err == nil && len(pl.Tracks) >= 10 {
			daily = pl.Tracks
			break
		}
	}
	_, lists, shows, _ := spotify.SearchLists("陈奕迅", 4)
	step(0, func() {
		if len(a.daily) == 0 {
			a.daily = daily
		}
		var saved []spotify.Playlist
		if al != nil {
			saved = append(saved, *al)
		}
		saved = append(saved, lists[:min(2, len(lists))]...)
		saved = append(saved, shows[:min(1, len(shows))]...)
		for i := range saved {
			saved[i].Tracks = nil
		}
		a.saved = saved
		if al != nil {
			for i, t := range al.Tracks {
				if t.ID == song {
					order := make([]int, len(al.Tracks))
					for k := range order {
						order[k] = k
					}
					a.pl.Restore(player.Session{Queue: al.Tracks, Order: order, At: i, Position: 76000})
				}
			}
		}
	})
	time.Sleep(3 * time.Second)

	page("r1-home", route{page: pageHome}, 5*time.Second)
	step(0, func() { a.nowPlaying, a.npPane = true, paneLyrics })
	time.Sleep(6 * time.Second)
	shot("r2-playing", a.win)
	page("r3-album", route{page: pagePlaylist, playlist: album}, 4*time.Second)
	page("r4-artist", route{page: pageArtist, artist: eason}, 6*time.Second)
	step(0, func() { a.nowPlaying = false; a.search("陈奕迅") })
	time.Sleep(6 * time.Second)
	shot("r5-search", a.win)
	page("r6-browse", route{page: pageBrowse}, 5*time.Second)
	page("r7-podcast", route{page: pagePlaylist, playlist: show}, 6*time.Second)
	page("r8-track", route{page: pageTrack, track: song}, 6*time.Second)
	var pop string
	mygo.RunOnMain(func() {
		for _, c := range a.cats {
			if c.Name == "流行" {
				pop = c.ID
			}
		}
	})
	if pop != "" {
		page("r9-category", route{page: pageCategory, category: pop}, 6*time.Second)
	}
	page("r10-profile", route{page: pageUser, user: "spotify"}, 6*time.Second)
	theme("dark")
	page("r11-home-dark", route{page: pageHome}, 3*time.Second)
	step(0, func() { a.nowPlaying, a.npPane = true, paneLyrics })
	time.Sleep(3 * time.Second)
	shot("r12-playing-dark", a.win)
	page("r13-podcast-dark", route{page: pagePlaylist, playlist: show}, 2*time.Second)
	theme("system")
}
