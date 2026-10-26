package app

import (
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// A login picks up the last stretch of listening on one device, oldest
// first: it ends where the device changes or the listener stopped for half
// an hour, and leaves out songs they may not play and songs played again.
func TestLastStretch(t *testing.T) {
	now := time.Date(2026, 10, 26, 22, 0, 0, 0, time.Local)
	play := func(id int64, ago time.Duration, device string, fee int) netease.Play {
		return netease.Play{Track: netease.Track{ID: id, Fee: fee}, At: now.Add(-ago), Device: device}
	}
	plays := []netease.Play{
		play(5, 3*time.Minute, "android", 0),
		play(4, 6*time.Minute, "android", 1), // VIP only
		play(3, 9*time.Minute, "android", 0),
		play(5, 12*time.Minute, "android", 0), // played again since
		play(2, 30*time.Minute, "android", 0),
		play(1, 70*time.Minute, "android", 0), // after a break
	}
	songs, last, ok := lastStretch(plays, now)
	var ids []int64
	for _, s := range songs {
		ids = append(ids, s.ID)
	}
	if !ok || len(ids) != 3 || ids[0] != 2 || ids[1] != 3 || ids[2] != 5 || last.Track.ID != 5 {
		t.Fatalf("picked %v, last %d", ids, last.Track.ID)
	}
	if got := heardAt(last, now); got != "3分钟前在手机上听过" {
		t.Errorf("heard %q", got)
	}

	plays[1] = play(4, 6*time.Minute, "osx", 0)
	if songs, _, _ := lastStretch(plays, now); len(songs) != 1 || songs[0].ID != 5 {
		t.Errorf("crossed devices: %v", songs)
	}
	plays[0] = play(5, 4, "android", 1)
	if songs, last, _ := lastStretch(plays[:1], now); len(songs) != 0 || last.Track.ID != 0 {
		t.Errorf("picked a song that may not play: %v", songs)
	}
	if _, _, ok := lastStretch([]netease.Play{play(1, 8*24*time.Hour, "iphone", 0)}, now); ok {
		t.Error("picked up a week-old song")
	}
}
