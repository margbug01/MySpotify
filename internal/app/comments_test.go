package app

import (
	"testing"
	"time"
)

// The count on the comments' tab holds the last song's while the next
// one's loads, for it to roll from one to the other, and goes once the
// next song turns out to have none.
func TestCommentCountHoldsWhileLoading(t *testing.T) {
	a, tt, _ := coverApp(t)
	a.comments = commentState{track: 2, loaded: true, total: 128}
	tt.Frame()
	a.pl.Next()
	a.comments = commentState{track: 3, loading: true}
	tt.Frame()
	if _, ok := tt.Find("评论 128"); !ok {
		t.Fatal("the count went as the next song's loads")
	}
	a.comments.loading, a.comments.loaded, a.comments.total = false, true, 96
	tt.Frame()
	if _, ok := tt.Find("评论 96"); !ok {
		t.Error("the count did not change once loaded")
	}
	a.pl.Next()
	a.comments = commentState{track: 1, loaded: true}
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("评论"); !ok {
		t.Error("a song without comments shows a count")
	}
}
