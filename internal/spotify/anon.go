package spotify

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnonymousIdentity gets an identity from Spotify without anyone signing
// in: the web player asks /api/token for one, proving with a one-time code
// that it is the web player. It grants what an anonymous visitor sees —
// search, browsing, lyrics — but neither playback nor anything of a
// listener's own.
//
// The client token lasts a fortnight. A failure to get one is not fatal
// here: browsing goes through Pathfinder, which does not want one.
func AnonymousIdentity() (Identity, error) {
	r, err := apiToken("")
	if err != nil {
		return Identity{}, fmt.Errorf("spotify: anonymous token: %w", err)
	}
	id := r.id
	if ct, err := clientToken(r.clientID); err == nil {
		id.ClientToken = ct
	}
	return id, nil
}

// serverTimeOf reads Spotify's clock.
func serverTimeOf() (time.Time, error) {
	req, err := http.NewRequest(http.MethodGet, serverTimeURL, nil)
	if err != nil {
		return time.Time{}, err
	}
	req.Header.Set("User-Agent", UserAgent)
	res, err := Client.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("server-time: %s", res.Status)
	}
	var v struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&v); err != nil {
		return time.Time{}, err
	}
	if v.ServerTime <= 0 {
		return time.Time{}, fmt.Errorf("server-time: none in the reply")
	}
	return time.Unix(v.ServerTime, 0), nil
}

// clientTokenURL is where this device's client token comes from; tests swap
// it.
var clientTokenURL = "https://clienttoken.spotify.com/v1/clienttoken"

// clientVersion is the web player build the requests name themselves as.
// Spotify accepts older ones, and may not forever.
const clientVersion = "1.3.8.509.g26b6eac7300d"

// clientToken asks for this device's client token, as the web player does:
// naming its build, its client ID and what it runs on.
func clientToken(clientID string) (string, error) {
	payload := map[string]any{"client_data": map[string]any{
		"client_version": clientVersion,
		"client_id":      clientID,
		"js_sdk_data": map[string]any{
			"device_brand":        "unknown",
			"device_model":        "unknown",
			"os":                  "macos",
			"os_version":          "10.15.7",
			"container_version":   "unknown",
			"device_id":           deviceID(),
			"device_type":         "computer",
			"platform_identifier": "web_player",
		},
	}}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, clientTokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	out, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("clienttoken: %s", res.Status)
	}
	var v struct {
		Granted struct {
			Token string `json:"token"`
		} `json:"granted_token"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", err
	}
	if v.Granted.Token == "" {
		return "", fmt.Errorf("clienttoken: none granted")
	}
	return v.Granted.Token, nil
}

// deviceID is what clienttoken wants to see stay the same across requests
// from one machine. It need not be stable beyond a run of the app.
func deviceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "vinyl0000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}
