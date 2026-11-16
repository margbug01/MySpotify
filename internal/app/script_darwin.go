package app

import (
	"github.com/ebitengine/purego/objc"

	"vinyl/internal/spotify"
)

// serveScripts lets AppleScript read what plays (#28), as Music's scripts
// do: tell application "MySpotify" to get {player state, player position,
// player rate, current track}. resources/darwin/MySpotify.sdef names the
// properties; Cocoa asks the application for each by its key, on the main
// thread, where the player is read.
func (a *app) serveScripts() {
	get := func(key string, fn func() objc.ID) {
		imp := objc.NewIMP(func(objc.ID, objc.SEL) objc.ID { return fn() })
		objc.GetClass("NSApplication").AddMethod(objc.RegisterName(key), imp, "@@:")
	}
	get("scriptPlayerState", func() objc.ID {
		state := "kPSS"
		switch {
		case a.pl.Current() == nil:
		case a.pl.Playing():
			state = "kPSP"
		default:
			state = "kPSp"
		}
		code := uint32(state[0])<<24 | uint32(state[1])<<16 | uint32(state[2])<<8 | uint32(state[3])
		return objc.Send[objc.ID](objc.ID(objc.GetClass("NSNumber")), objc.RegisterName("numberWithUnsignedInt:"), code)
	})
	get("scriptPlayerPosition", func() objc.ID {
		if a.pl.Current() == nil {
			return 0 // missing value
		}
		return scriptNumber(a.pl.SongPosition().Seconds())
	})
	get("scriptPlayerRate", func() objc.ID { return scriptNumber(a.pl.Rate()) })
	get("scriptCurrentTrack", func() objc.ID {
		t := a.pl.Current()
		if t == nil {
			return 0
		}
		d := objc.ID(objc.GetClass("NSMutableDictionary")).Send(objc.RegisterName("dictionary"))
		for k, v := range scriptTrack(t) {
			o := scriptNumber(0)
			switch v := v.(type) {
			case string:
				o = scriptText(v)
			case float64:
				o = scriptNumber(v)
			}
			d.Send(objc.RegisterName("setObject:forKey:"), o, scriptText(k))
		}
		return d
	})
}

// scriptTrack is t as scripts get it: the fields of the sdef's track info,
// by their keys.
func scriptTrack(t *spotify.Track) map[string]any {
	kind := "song"
	if t.Episode {
		kind = "episode"
	}
	return map[string]any{
		"id": t.URI(), "name": t.Name, "artist": t.ArtistNames(), "album": t.Album.Name,
		"duration": t.Length().Seconds(), "kind": kind,
	}
}

func scriptText(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), s+"\x00")
}

func scriptNumber(f float64) objc.ID {
	return objc.Send[objc.ID](objc.ID(objc.GetClass("NSNumber")), objc.RegisterName("numberWithDouble:"), f)
}
