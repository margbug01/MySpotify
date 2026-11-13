package spotify

import "time"

type castError struct {
	msg   string
	cause error
}

func (e *castError) Error() string { return e.msg }
func (e *castError) Unwrap() error { return e.cause }

const (
	SpotifyCastAppID    = "CC32E753"
	SpotifyCastClientID = "d7df0887fb71494ea994202cb473eae7"
	castMDNSService     = "_googlecast._tcp.local"
	castNSConnection    = "urn:x-cast:com.google.cast.tp.connection"
	castNSHeartbeat     = "urn:x-cast:com.google.cast.tp.heartbeat"
	castNSReceiver      = "urn:x-cast:com.google.cast.receiver"
	castNSSpotifySecure = "urn:x-cast:com.spotify.chromecast.secure.v1"
	castSpeakerTTL      = 75 * time.Second
)

// CastSpeaker 代表在區域網路上透過 mDNS 探索到的 Google Cast 音響
type CastSpeaker struct {
	ID              string    `json:"id"`
	SpotifyDeviceID string    `json:"spotify_device_id,omitempty"`
	Name            string    `json:"name"`
	Model           string    `json:"model"`
	IP              string    `json:"ip"`
	Port            int       `json:"port"`
	ExpiresAt       time.Time `json:"expires_at"`
}
