package player

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// videoPlayer sets up a player as licensed does, but for songs that have
// music videos, which play in a Web of their own, with no page either:
// tracks 1 and 2, the first loaded and playing. It returns what the player
// posts, the player, and the videos it resolved.
func videoPlayer(t *testing.T, licence http.HandlerFunc, tracks ...spotify.Track) (chan func(), *Player, func() []string) {
	t.Helper()
	srv := httptest.NewServer(licence)
	origin, res, resVideo := licenseOrigin, resolve, resolveVideo
	licenseOrigin = srv.URL
	resolve = func(uri string) (*spotify.Media, error) {
		return &spotify.Media{Manifest: "#EXTM3U\nhttps://cdn.test/a.mp4\n", Duration: 3 * time.Minute, Video: "video of " + uri}, nil
	}
	var mu sync.Mutex
	var videos []string
	resolveVideo = func(id string) (*spotify.Video, error) {
		mu.Lock()
		videos = append(videos, id)
		mu.Unlock()
		return &spotify.Video{
			Master:    "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\np1.m3u8\n",
			Playlists: map[string]string{"p1.m3u8": "#EXTM3U\n#EXTINF:4.000,\nhttps://video.test/1/0.mp4\n#EXT-X-ENDLIST\n"},
			Duration:  4 * time.Minute,
			License:   "/widevine-license/v1/video/license",
		}, nil
	}
	spotify.SetIdentity(spotify.Identity{Token: "old", Expires: time.Now().Add(time.Hour)})
	t.Cleanup(func() {
		srv.Close()
		licenseOrigin, resolve, resolveVideo = origin, res, resVideo
		spotify.SetIdentity(spotify.Identity{})
	})
	web := func() *Web {
		return &Web{addr: "127.0.0.1:1", cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	}
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Web = web()
	p.NewVideo = func() (*Web, error) { return web(), nil }
	if tracks == nil {
		tracks = []spotify.Track{{ID: "1"}, {ID: "2"}}
	}
	p.PlayQueue(tracks, 0)
	until(t, ch, func() bool { return p.out != nil })
	if p.out == nil || p.ShowsVideo() || !p.HasVideo() {
		t.Fatal("the song did not load")
	}
	return ch, p, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(videos)
	}
}

// until runs what the player posts until done.
func until(t *testing.T, ch chan func(), done func() bool) {
	t.Helper()
	for !done() {
		next(t, ch)
	}
}

// sent returns what w asked its page to do since last asked.
func sent(w *Web) []string {
	var cmds []string
	for {
		select {
		case c := <-w.cmds:
			cmds = append(cmds, c)
		default:
			return cmds
		}
	}
}

// ask asks w what its page would, and returns the answer.
func ask(w *Web, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	w.ServeHTTP(rec, httptest.NewRequest(method, "http://"+w.addr+path, strings.NewReader(body)))
	return rec
}

// Turned on, the music video plays from where the song is, in the video's
// page, and the progress runs to the video's end; turned off, the song
// plays on from where the video is.
func TestVideoPicksUpWhereTheSongIs(t *testing.T) {
	ch, p, _ := videoPlayer(t, func(http.ResponseWriter, *http.Request) {})
	p.out.Seek(30 * time.Second)
	p.SetVideo(true)
	until(t, ch, p.ShowsVideo)
	if !p.ShowsVideo() || p.Video == nil || p.out != Output(p.Video.out(1)) {
		t.Fatal("the video does not play")
	}
	if p.Position() != 30*time.Second || p.Duration() != 4*time.Minute {
		t.Fatalf("video at %v of %v", p.Position(), p.Duration())
	}
	if cmds := sent(p.Video); !slices.Contains(cmds, "vinyl.seek(1,30.000)") || !slices.Contains(cmds, "vinyl.play(1)") {
		t.Fatalf("the video's page was asked %q", cmds)
	}
	p.out.Seek(50 * time.Second)
	p.SetVideo(false)
	if p.ShowsVideo() || p.out != Output(p.Web.out(2)) || p.Position() != 50*time.Second || p.Duration() != 0 {
		t.Fatalf("song: video %v, at %v of %v", p.ShowsVideo(), p.Position(), p.Duration())
	}
}

// A music video that cannot play gives way to the song, from where it was,
// saying so; the queue doesn't skip on, as for a song that cannot play.
func TestVideoThatFailsGivesWayToTheSong(t *testing.T) {
	ch, p, _ := videoPlayer(t, func(http.ResponseWriter, *http.Request) {})
	p.SetVideo(true)
	until(t, ch, p.ShowsVideo)
	p.out.Seek(42 * time.Second)
	ask(p.Video, http.MethodPost, "/event", `{"gen":1,"type":"error","msg":"keySystemError keySystemLicenseRequestFailed http=403"}`)
	until(t, ch, func() bool { return !p.ShowsVideo() })
	if p.Current().ID != "1" || p.ShowsVideo() || p.VideoOn() || !p.Playing() {
		t.Fatalf("track %s, video %v, on %v, playing %v", p.Current().ID, p.ShowsVideo(), p.VideoOn(), p.Playing())
	}
	if p.Err() != "MV 暂时无法播放" || p.Position() != 42*time.Second {
		t.Fatalf("err %q, at %v", p.Err(), p.Position())
	}
}

// The video's page gets its playlists, their segments pointed at the
// server, and its licence comes from where the manifest says, not where a
// song's does.
func TestVideoPageGetsItsPlaylistsAndLicence(t *testing.T) {
	var paths []string
	ch, p, _ := videoPlayer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Write([]byte("licence"))
	})
	p.SetVideo(true)
	until(t, ch, p.ShowsVideo)
	if r := ask(p.Video, http.MethodGet, "/m/1/track.m3u8", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "\np1.m3u8\n") {
		t.Fatalf("master: %d %q", r.Code, r.Body)
	}
	if r := ask(p.Video, http.MethodGet, "/m/1/p1.m3u8", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "\n/m/1/0\n") {
		t.Fatalf("playlist: %d %q", r.Code, r.Body)
	}
	if r := ask(p.Video, http.MethodPost, "/license?gen=1", "challenge"); r.Code != 200 || r.Body.String() != "licence" {
		t.Fatalf("licence: %d %q", r.Code, r.Body)
	}
	if len(paths) != 1 || paths[0] != "/widevine-license/v1/video/license" {
		t.Fatalf("licence asked of %q", paths)
	}
}

// A licence the video's page asks for late, the video closed as the song
// took its place, isn't asked of Spotify: once, it went to the song's
// licence address, which turned the video's request down (400), one more
// request on an account Spotify was throttling.
func TestClosedVideoAsksNoLicence(t *testing.T) {
	var asked int
	ch, p, _ := videoPlayer(t, func(http.ResponseWriter, *http.Request) { asked++ })
	host := licenseHost
	licenseHost = licenseOrigin
	defer func() { licenseHost = host }()
	p.SetVideo(true)
	until(t, ch, p.ShowsVideo)
	p.SetVideo(false)
	for i := 0; p.Video.out(1) != nil; i++ {
		if i > 100 {
			t.Fatal("the video did not close")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r := ask(p.Video, http.MethodPost, "/license?gen=1", "challenge"); r.Code != http.StatusGone || asked != 0 {
		t.Fatalf("answered %d, Spotify asked %d times", r.Code, asked)
	}
}

// With music videos on, the next track plays its video too.
func TestVideoGoesOnWithTheNextTrack(t *testing.T) {
	ch, p, videos := videoPlayer(t, func(http.ResponseWriter, *http.Request) {})
	p.SetVideo(true)
	until(t, ch, p.ShowsVideo)
	p.Next()
	until(t, ch, func() bool { return !p.Loading() })
	if p.Current().ID != "2" || !p.ShowsVideo() || p.out != Output(p.Video.out(2)) {
		t.Fatalf("track %s, video %v", p.Current().ID, p.ShowsVideo())
	}
	if v := videos(); len(v) != 2 || v[1] != "video of "+(&spotify.Track{ID: "2"}).URI() {
		t.Fatalf("videos resolved: %q", v)
	}
}

// A music video a few seconds longer than its song is taken to open with
// half of them: it picks up that much further on than the song is, back to
// the song the song plays on that much less far, and the lyrics follow the
// song. A lead the listener set goes first.
func TestVideoRunsAheadOfTheSong(t *testing.T) {
	// Four seconds shorter than its video.
	ch, p, _ := videoPlayer(t, func(http.ResponseWriter, *http.Request) {}, spotify.Track{ID: "1", Duration: 236000}, spotify.Track{ID: "2"})
	p.out.Seek(30 * time.Second)
	p.SetVideo(true)
	until(t, ch, p.ShowsVideo)
	if p.VideoLead() != 2*time.Second || p.Position() != 32*time.Second || p.SongPosition() != 30*time.Second {
		t.Fatalf("video ahead by %v, at %v, the song at %v", p.VideoLead(), p.Position(), p.SongPosition())
	}
	if cmds := sent(p.Video); !slices.Contains(cmds, "vinyl.seek(1,32.000)") {
		t.Fatalf("the video's page was asked %q", cmds)
	}
	p.Leads = func(id string) (time.Duration, bool) { return 6 * time.Second, id == "1" }
	if p.VideoLead() != 6*time.Second || p.SongPosition() != 26*time.Second {
		t.Fatalf("the listener's lead: ahead by %v, the song at %v", p.VideoLead(), p.SongPosition())
	}
	p.out.Seek(50 * time.Second)
	p.SetVideo(false)
	if p.ShowsVideo() || p.Position() != 44*time.Second || p.SongPosition() != 44*time.Second || p.VideoLead() != 0 {
		t.Fatalf("song: video %v, at %v", p.ShowsVideo(), p.Position())
	}
}

// Only a video a few seconds longer than its song is taken to open with
// some of them, half.
func TestLeadGuess(t *testing.T) {
	song := &spotify.Track{Duration: 170000}
	for _, c := range []struct{ video, want time.Duration }{
		{172900 * time.Millisecond, 1450 * time.Millisecond},
		{180 * time.Second, 5 * time.Second},
		{170 * time.Second, 0},
		{165 * time.Second, 0},
		{200 * time.Second, 0},
	} {
		if got := leadGuess(song, &spotify.Video{Duration: c.video}); got != c.want {
			t.Errorf("a video of %v: %v ahead, want %v", c.video, got, c.want)
		}
	}
	if got := leadGuess(&spotify.Track{}, &spotify.Video{Duration: 3 * time.Second}); got != 0 {
		t.Errorf("a song of no known length: %v ahead", got)
	}
}

// Turned on for the track about to play, music videos leave the one
// playing as it plays, its video not loaded; the next one's video plays.
func TestVideoNextLeavesTheTrackPlaying(t *testing.T) {
	ch, p, videos := videoPlayer(t, func(http.ResponseWriter, *http.Request) {})
	song := p.out
	p.VideoNext()
	if p.out != song || p.ShowsVideo() || !p.VideoOn() || len(videos()) != 0 {
		t.Fatalf("the song playing: video %v, on %v, videos loaded %q", p.ShowsVideo(), p.VideoOn(), videos())
	}
	p.PlayQueue([]spotify.Track{{ID: "3"}}, 0)
	until(t, ch, p.ShowsVideo)
	if !p.ShowsVideo() || len(videos()) != 1 {
		t.Fatalf("the next track: video %v, videos loaded %q", p.ShowsVideo(), videos())
	}
}
