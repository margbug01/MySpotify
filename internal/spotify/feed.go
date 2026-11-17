package spotify

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// directoryURL is Apple's podcast directory, which names a podcast's
// public feed, as Spotify does not; tests swap it.
var directoryURL = "https://itunes.apple.com/search"

// feedFresh is how long a feed read is used before it is read again, for
// an episode that came out since.
const feedFresh = 10 * time.Minute

var (
	feedMu    sync.Mutex
	feedOf    = map[string]string{} // a podcast's feed by its name and publisher, "" for none found
	feedsRead = map[string]feedRead{}
)

type feedRead struct {
	at    time.Time
	audio map[string]string // an episode's audio by its title
}

// feedAudio returns where the podcast show, by publisher, keeps the audio of
// its episode title, as its own feed names it: the podcast found by name in
// Apple's directory, the episode by its title, which Spotify takes from the
// same feed. It is "" where either is not found.
func feedAudio(show, publisher, title string) (string, error) {
	show, title = strings.TrimSpace(show), strings.TrimSpace(title)
	if show == "" || title == "" {
		return "", nil
	}
	key := show + "\x00" + publisher
	feedMu.Lock()
	feed, known := feedOf[key]
	feedMu.Unlock()
	if !known {
		var err error
		if feed, err = findFeed(show, publisher); err != nil {
			return "", err
		}
		feedMu.Lock()
		feedOf[key] = feed
		feedMu.Unlock()
	}
	if feed == "" {
		return "", nil
	}
	feedMu.Lock()
	read, ok := feedsRead[feed]
	feedMu.Unlock()
	if !ok || time.Since(read.at) > feedFresh {
		audio, err := readFeed(feed)
		if err != nil {
			return "", err
		}
		read = feedRead{time.Now(), audio}
		feedMu.Lock()
		feedsRead[feed] = read
		feedMu.Unlock()
	}
	return read.audio[title], nil
}

// findFeed returns the feed of the podcast named show in Apple's directory:
// the one by publisher, or else the first of that name.
func findFeed(show, publisher string) (string, error) {
	res, err := getWeb(directoryURL + "?" + url.Values{"media": {"podcast"}, "entity": {"podcast"}, "limit": {"10"}, "term": {show}}.Encode())
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var found struct {
		Results []struct {
			Name      string `json:"collectionName"`
			Publisher string `json:"artistName"`
			Feed      string `json:"feedUrl"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&found); err != nil {
		return "", fmt.Errorf("directory: %w", err)
	}
	feed := ""
	for _, r := range found.Results {
		if strings.TrimSpace(r.Name) != show || !webURL(r.Feed) {
			continue
		}
		if strings.TrimSpace(r.Publisher) == publisher {
			return r.Feed, nil
		}
		if feed == "" {
			feed = r.Feed
		}
	}
	return feed, nil
}

// readFeed returns the audio of each episode of the RSS feed at u by its
// title, as the feed or iTunes names it.
func readFeed(u string) (map[string]string, error) {
	res, err := getWeb(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	audio := map[string]string{}
	d := xml.NewDecoder(io.LimitReader(res.Body, 64<<20))
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return audio, nil
		}
		if err != nil {
			return nil, fmt.Errorf("feed: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "item" {
			continue
		}
		var item struct {
			Titles    []string `xml:"title"` // RSS's, and iTunes's
			Enclosure struct {
				URL string `xml:"url,attr"`
			} `xml:"enclosure"`
		}
		if err := d.DecodeElement(&item, &start); err != nil {
			return nil, fmt.Errorf("feed: %w", err)
		}
		if !webURL(item.Enclosure.URL) {
			continue
		}
		for _, t := range item.Titles {
			if t = strings.TrimSpace(t); t != "" && audio[t] == "" {
				audio[t] = item.Enclosure.URL
			}
		}
	}
}

// getWeb gets u, from no Spotify service, and fails unless it answers 200.
func getWeb(u string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	res, err := Client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		cancel()
		return nil, fmt.Errorf("%s: %s", req.URL.Host, res.Status)
	}
	res.Body = cancelOnClose{res.Body, cancel}
	return res, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// webURL reports whether s is an address of the web.
func webURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
