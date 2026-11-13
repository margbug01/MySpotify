package spotify

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/egoist/mygo/i18n"
)

func buildTestMDNSPacket(instance, host string, port uint16, txt []string) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x00, 0x84, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00})

	writeName := func(b *bytes.Buffer, name string) {
		for _, part := range strings.Split(name, ".") {
			b.WriteByte(byte(len(part)))
			b.WriteString(part)
		}
		b.WriteByte(0)
	}

	writeName(&buf, instance)
	binary.Write(&buf, binary.BigEndian, uint16(33)) // SRV
	binary.Write(&buf, binary.BigEndian, uint16(1))  // IN
	binary.Write(&buf, binary.BigEndian, uint32(120))
	var srvData bytes.Buffer
	binary.Write(&srvData, binary.BigEndian, uint16(0))
	binary.Write(&srvData, binary.BigEndian, uint16(0))
	binary.Write(&srvData, binary.BigEndian, port)
	writeName(&srvData, host)
	binary.Write(&buf, binary.BigEndian, uint16(srvData.Len()))
	buf.Write(srvData.Bytes())

	writeName(&buf, instance)
	binary.Write(&buf, binary.BigEndian, uint16(16)) // TXT
	binary.Write(&buf, binary.BigEndian, uint16(1))  // IN
	binary.Write(&buf, binary.BigEndian, uint32(120))
	var txtData bytes.Buffer
	for _, entry := range txt {
		txtData.WriteByte(byte(len(entry)))
		txtData.WriteString(entry)
	}
	binary.Write(&buf, binary.BigEndian, uint16(txtData.Len()))
	buf.Write(txtData.Bytes())

	return buf.Bytes()
}

// Discovered Cast speakers keep a stable order across calls and read their SRV port (such as a Cast group's dynamic port) instead of randomizing map iteration on port 8009.
func TestCastSpeakersOrderAndSRVPort(t *testing.T) {
	reg := NewSpeakerRegistry()

	pktBedroom := buildTestMDNSPacket(
		"Google-Home-Mini-0001._googlecast._tcp.local",
		"bedroom.local",
		8009,
		[]string{"id=cast-speaker-0001", "fn=Bedroom", "md=Google Home Mini"},
	)
	pktGroup := buildTestMDNSPacket(
		"Google-Cast-Group-0001._googlecast._tcp.local",
		"bedroom.local",
		32072,
		[]string{"id=cast-group-0001", "fn=Home Group", "md=Google Cast Group"},
	)
	pktKitchen := buildTestMDNSPacket(
		"Google-Nest-Mini-0002._googlecast._tcp.local",
		"kitchen.local",
		8009,
		[]string{"id=cast-speaker-0002", "fn=Kitchen", "md=Google Nest Mini"},
	)

	reg.parseMDNSResponse(pktBedroom, &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 5353})
	reg.parseMDNSResponse(pktGroup, &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 5353})
	reg.parseMDNSResponse(pktKitchen, &net.UDPAddr{IP: net.ParseIP("192.0.2.11"), Port: 5353})

	var baseline string
	for i := 0; i < 30; i++ {
		got := reg.Speakers()
		if len(got) != 3 {
			t.Fatalf("got %d speakers, want 3", len(got))
		}
		names := make([]string, len(got))
		for j, s := range got {
			names[j] = s.Name + ":" + strconv.Itoa(s.Port)
			if s.Name == "Home Group" && s.Port != 32072 {
				t.Fatalf("Cast group Home Group port = %d, want 32072 from SRV record", s.Port)
			}
		}
		order := strings.Join(names, "|")
		if i == 0 {
			baseline = order
		} else if order != baseline {
			t.Fatalf("speaker order jumped on call #%d: %q vs %q", i, order, baseline)
		}
	}
}

// Waking a Cast speaker opens receiver-0, launches CC32E753, exchanges getInfo for a device-auth token, and sends addUser with a multi-byte varint frame.
func TestWakeupCastSpeakerHandshake(t *testing.T) {
	oldID, _ := CurrentIdentity()
	defer func() {
		SetIdentity(oldID)
	}()
	SetIdentity(Identity{Token: "web-player-token", ClientToken: "client-tok", Expires: time.Now().Add(time.Hour)})

	longCastToken := strings.Repeat("cast-access-token-", 25) // 450 bytes to verify multi-byte protobuf varint
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/device-auth/v1/refresh" {
				body, _ := json.Marshal(map[string]string{"accessToken": longCastToken})
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     http.Header{"Content-Type": []string{"application/json"}},
				}, nil
			}
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		}),
	}

	addUserReceived := make(chan string, 1)
	dialer := func(addr string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		go func() {
			defer serverConn.Close()
			receiverConnected := false
			appConnected := false
			for {
				var length uint32
				if err := binary.Read(serverConn, binary.BigEndian, &length); err != nil {
					return
				}
				frame := make([]byte, length)
				if _, err := io.ReadFull(serverConn, frame); err != nil {
					return
				}
				s := string(frame)
				if !receiverConnected {
					if strings.Contains(s, "urn:x-cast:com.google.cast.tp.connection") && strings.Contains(s, `"CONNECT"`) {
						receiverConnected = true
						continue
					}
					_ = writeCastFrame(serverConn, "receiver-0", "sender-0", "urn:x-cast:com.google.cast.tp.connection", []byte(`{"type":"CLOSE"}`))
					return
				}
				if strings.Contains(s, "urn:x-cast:com.google.cast.receiver") && strings.Contains(s, `"LAUNCH"`) {
					if !strings.Contains(s, `"CC32E753"`) {
						_ = writeCastFrame(serverConn, "receiver-0", "sender-0", "urn:x-cast:com.google.cast.receiver", []byte(`{"type":"LAUNCH_ERROR","reason":"NOT_FOUND","requestId":1}`))
						return
					}
					_ = writeCastFrame(serverConn, "receiver-0", "sender-0", "urn:x-cast:com.google.cast.receiver", []byte(`{"type":"RECEIVER_STATUS","requestId":1,"status":{"applications":[{"appId":"CC32E753","transportId":"transport-spotify-1"}]}}`))
					continue
				}
				if strings.Contains(s, "transport-spotify-1") && strings.Contains(s, "urn:x-cast:com.google.cast.tp.connection") && strings.Contains(s, `"CONNECT"`) {
					appConnected = true
					continue
				}
				if appConnected && strings.Contains(s, "urn:x-cast:com.spotify.chromecast.secure.v1") && strings.Contains(s, `"getInfo"`) {
					_ = writeCastFrame(serverConn, "transport-spotify-1", "sender-0", "urn:x-cast:com.spotify.chromecast.secure.v1", []byte(`{"type":"getInfoResponse","payload":{"deviceID":"spotify-cast-device-0001","clientID":"d7df0887fb71494ea994202cb473eae7","tokenType":"accesstoken","status":101,"statusString":"OK"}}`))
					continue
				}
				if appConnected && strings.Contains(s, "urn:x-cast:com.spotify.chromecast.secure.v1") && strings.Contains(s, `"addUser"`) {
					if strings.Contains(s, longCastToken) {
						addUserReceived <- "spotify-cast-device-0001"
						_ = writeCastFrame(serverConn, "transport-spotify-1", "sender-0", "urn:x-cast:com.spotify.chromecast.secure.v1", []byte(`{"type":"addUserResponse","payload":{"status":101,"statusString":"OK","deviceId":"spotify-cast-device-0001"}}`))
					} else {
						_ = writeCastFrame(serverConn, "transport-spotify-1", "sender-0", "urn:x-cast:com.spotify.chromecast.secure.v1", []byte(`{"type":"addUserError","payload":{"status":202,"statusString":"ERROR-LOGIN-FAILED"}}`))
					}
					return
				}
			}
		}()
		return clientConn, nil
	}

	client := NewCastWakeupClient(
		WithCastHTTPClient(httpClient),
		WithCastDialer(dialer),
		WithCastNow(time.Now),
	)
	defer client.Close()

	gotDevID, err := client.ConnectSpeaker(context.Background(), &CastSpeaker{
		ID:    "cast-group-0001",
		Name:  "Home Group",
		Model: "Google Cast Group",
		IP:    "127.0.0.1",
		Port:  32072,
	})
	if err != nil {
		t.Fatalf("ConnectSpeaker failed: %v", err)
	}
	if gotDevID != "spotify-cast-device-0001" {
		t.Fatalf("ConnectSpeaker deviceID = %q, want spotify-cast-device-0001", gotDevID)
	}
	select {
	case gotID := <-addUserReceived:
		if gotID != "spotify-cast-device-0001" {
			t.Fatalf("got deviceID %q", gotID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ConnectSpeaker returned without completing getInfo + device-auth + addUser handshake")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakePacketConn struct {
	reads  atomic.Int64
	closed atomic.Bool
	pkt    []byte
	src    net.Addr
}

func (f *fakePacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if f.closed.Load() {
		return 0, nil, net.ErrClosed
	}
	time.Sleep(2 * time.Millisecond)
	f.reads.Add(1)
	n := copy(p, f.pkt)
	return n, f.src, nil
}

func (f *fakePacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if f.closed.Load() {
		return 0, net.ErrClosed
	}
	return len(p), nil
}

func (f *fakePacketConn) Close() error {
	f.closed.Store(true)
	return nil
}

func (f *fakePacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4zero, Port: 5353}
}

func (f *fakePacketConn) SetDeadline(t time.Time) error      { return nil }
func (f *fakePacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (f *fakePacketConn) SetWriteDeadline(t time.Time) error { return nil }

// Decoding a serialized CastMessage protobuf frame extracts namespace and UTF-8 payload while skipping unknown varint, fixed, and bytes fields.
func TestCastDecodeProtobufFrame(t *testing.T) {
	var frame []byte
	frame = append(frame, encodeCastVarint(1, 0)...)
	frame = append(frame, encodeCastVarint(99, 300)...) // unknown multi-byte varint field
	frame = binary.AppendUvarint(frame, uint64((98<<3)|1))
	frame = append(frame, 1, 2, 3, 4, 5, 6, 7, 8) // unknown fixed64
	frame = binary.AppendUvarint(frame, uint64((97<<3)|5))
	frame = append(frame, 1, 2, 3, 4) // unknown fixed32
	frame = append(frame, encodeCastBytes(6, []byte(`{"type":"PONG"}`))...)
	frame = append(frame, encodeCastBytes(4, []byte(castNSHeartbeat))...)
	frame = append(frame, encodeCastBytes(2, []byte("receiver-0"))...)
	frame = append(frame, encodeCastBytes(3, []byte("sender-0"))...)
	frame = append(frame, encodeCastVarint(5, 0)...)

	msg, err := decodeCastMessage(frame)
	if err != nil {
		t.Fatalf("decodeCastMessage failed: %v", err)
	}
	if msg.namespace != castNSHeartbeat {
		t.Fatalf("namespace = %q, want %q", msg.namespace, castNSHeartbeat)
	}
	if msg.payloadUTF8 != `{"type":"PONG"}` {
		t.Fatalf("payloadUTF8 = %q, want PONG JSON", msg.payloadUTF8)
	}
	if msg.sourceID != "receiver-0" || msg.destinationID != "sender-0" {
		t.Fatalf("unexpected endpoints %q -> %q", msg.sourceID, msg.destinationID)
	}
}

// Handshaking with a Cast speaker handles whitespace-padded, key-reordered JSON payloads, multi-app RECEIVER_STATUS entries, and heartbeat PINGs.
func TestCastHandshakeWhitespaceAndReorderedJSON(t *testing.T) {
	oldID, _ := CurrentIdentity()
	defer func() {
		SetIdentity(oldID)
	}()

	SetIdentity(Identity{Token: "web-tok", ClientToken: "client-tok", Expires: time.Now().Add(time.Hour)})
	reg := NewSpeakerRegistry()
	reg.Seed(&CastSpeaker{
		ID:        "spk-1",
		Name:      "Living Room",
		Model:     "Google Home",
		IP:        "127.0.0.1",
		Port:      8009,
		ExpiresAt: time.Now().Add(time.Minute),
	})

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(map[string]string{"accessToken": "cast-tok-123"})
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}

	pongSeen := make(chan struct{}, 1)
	dialer := func(addr string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		go func() {
			defer serverConn.Close()
			var transportConnected bool
			for {
				msg, err := readCastFrame(serverConn)
				if err != nil {
					return
				}
				var env struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal([]byte(msg.payloadUTF8), &env)
				switch {
				case msg.namespace == castNSHeartbeat && env.Type == "PONG":
					select {
					case pongSeen <- struct{}{}:
					default:
					}
					_ = writeCastFrame(serverConn, "receiver-0", "sender-0", castNSReceiver, []byte(`{
						"status": {
							"applications": [
								{ "transportId": "backdrop-transport", "appId": "E8C28D3C", "displayName": "Backdrop" },
								{ "displayName": "Spotify", "transportId" : "spotify-transport-42", "appId" : "CC32E753" }
							]
						},
						"requestId": 1,
						"type": "RECEIVER_STATUS"
					}`))
				case msg.namespace == castNSReceiver && env.Type == "LAUNCH":
					_ = writeCastFrame(serverConn, "receiver-0", "sender-0", castNSHeartbeat, []byte("{\n  \"extra\": 1,\n  \"type\" : \"PING\"\n}"))
				case msg.namespace == castNSConnection && msg.destinationID == "spotify-transport-42" && env.Type == "CONNECT":
					transportConnected = true
				case transportConnected && msg.namespace == castNSSpotifySecure && env.Type == "getInfo":
					_ = writeCastFrame(serverConn, "spotify-transport-42", "sender-0", castNSSpotifySecure, []byte(`{
						"payload" : {
							"statusString": "OK",
							"clientID": "d7df0887fb71494ea994202cb473eae7",
							"deviceID": "cloud-dev-42",
							"status": 101
						},
						"extraField": true,
						"type" : "getInfoResponse"
					}`))
				case transportConnected && msg.namespace == castNSSpotifySecure && env.Type == "addUser":
					_ = writeCastFrame(serverConn, "spotify-transport-42", "sender-0", castNSSpotifySecure, []byte(`{
						"payload": { "status": 101 },
						"type" : "addUserResponse"
					}`))
					return
				}
			}
		}()
		return clientConn, nil
	}

	client := NewCastWakeupClient(
		WithCastHTTPClient(httpClient),
		WithCastDialer(dialer),
	)
	defer client.Close()

	gotID, err := client.ConnectSpeaker(context.Background(), &CastSpeaker{ID: "spk-1", Name: "Living Room", IP: "127.0.0.1", Port: 8009})
	if err != nil {
		t.Fatalf("ConnectSpeaker failed: %v", err)
	}
	if gotID != "cloud-dev-42" {
		t.Fatalf("deviceID = %q, want cloud-dev-42", gotID)
	}
	select {
	case <-pongSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("expected client to respond with PONG to whitespace-padded PING")
	}
	reg.SetSpotifyDeviceID("spk-1", gotID)
	speakers := reg.Speakers()
	if len(speakers) != 1 || speakers[0].SpotifyDeviceID != "cloud-dev-42" {
		t.Fatalf("expected SpotifyDeviceID to be recorded on speaker, got %+v", speakers)
	}
}

// Cast handshake and device-auth return localized errors without leaking raw English transport errors into Simplified Chinese messages.
func TestCastHandshakeErrorPayloads(t *testing.T) {
	previousLocale := i18n.Locale()
	i18n.SetLocaleFunc(func() string {
		if v := os.Getenv("MYGO_LOCALE"); v != "" {
			return v
		}
		return "zh-Hans"
	})
	defer i18n.SetLocaleFunc(func() string { return previousLocale })

	oldID, _ := CurrentIdentity()
	defer func() {
		SetIdentity(oldID)
	}()
	SetIdentity(Identity{Token: "web-tok", ClientToken: "client-tok", Expires: time.Now().Add(time.Hour)})

	okClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(map[string]string{"accessToken": "cast-tok-123"})
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}

	hasRawEnglishWord := func(s string) bool {
		cleaned := strings.NewReplacer("Spotify", "", "Cast", "").Replace(s)
		for _, r := range cleaned {
			if r <= unicode.MaxASCII && unicode.IsLetter(r) {
				return true
			}
		}
		return false
	}

	dialSentinel := errors.New("tls dial failed")
	cases := []struct {
		name      string
		dialErr   error
		client    *http.Client
		responder func(net.Conn)
		wantEnSub string
	}{
		{
			name:      "castDial",
			dialErr:   dialSentinel,
			wantEnSub: "connect",
		},
		{
			name:      "LAUNCH_ERROR",
			wantEnSub: "refused to launch",
			responder: func(serverConn net.Conn) {
				defer serverConn.Close()
				for {
					msg, err := readCastFrame(serverConn)
					if err != nil {
						return
					}
					if msg.namespace == castNSReceiver {
						_ = writeCastFrame(serverConn, "receiver-0", "sender-0", castNSReceiver, []byte(`{"reason":"NOT_FOUND","type" : "LAUNCH_ERROR"}`))
						return
					}
				}
			},
		},
		{
			name:      "getInfoError",
			wantEnSub: "rejected Spotify authentication",
			responder: func(serverConn net.Conn) {
				defer serverConn.Close()
				for {
					msg, err := readCastFrame(serverConn)
					if err != nil {
						return
					}
					switch msg.namespace {
					case castNSReceiver:
						_ = writeCastFrame(serverConn, "receiver-0", "sender-0", castNSReceiver, []byte(`{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"CC32E753","transportId":"tr-1"}]}}`))
					case castNSSpotifySecure:
						_ = writeCastFrame(serverConn, "tr-1", "sender-0", castNSSpotifySecure, []byte(`{"payload":{"status":400},"type" : "getInfoError"}`))
						return
					}
				}
			},
		},
		{
			name:      "addUserError",
			wantEnSub: "rejected Spotify authentication",
			responder: func(serverConn net.Conn) {
				defer serverConn.Close()
				for {
					msg, err := readCastFrame(serverConn)
					if err != nil {
						return
					}
					var env struct {
						Type string `json:"type"`
					}
					_ = json.Unmarshal([]byte(msg.payloadUTF8), &env)
					switch {
					case msg.namespace == castNSReceiver && env.Type == "LAUNCH":
						_ = writeCastFrame(serverConn, "receiver-0", "sender-0", castNSReceiver, []byte(`{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"CC32E753","transportId":"tr-1"}]}}`))
					case msg.namespace == castNSSpotifySecure && env.Type == "getInfo":
						_ = writeCastFrame(serverConn, "tr-1", "sender-0", castNSSpotifySecure, []byte(`{"type":"getInfoResponse","payload":{"deviceID":"dev-1","clientID":"cli-1"}}`))
					case msg.namespace == castNSSpotifySecure && env.Type == "addUser":
						_ = writeCastFrame(serverConn, "tr-1", "sender-0", castNSSpotifySecure, []byte(`{"payload":{"status":202},"type" : "addUserError"}`))
						return
					}
				}
			},
		},
		{
			name: "deviceAuthHTTP500",
			client: &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(strings.NewReader("server error")),
						Header:     http.Header{},
					}, nil
				}),
			},
			wantEnSub: "500",
			responder: func(serverConn net.Conn) {
				defer serverConn.Close()
				for {
					msg, err := readCastFrame(serverConn)
					if err != nil {
						return
					}
					var env struct {
						Type string `json:"type"`
					}
					_ = json.Unmarshal([]byte(msg.payloadUTF8), &env)
					switch {
					case msg.namespace == castNSReceiver && env.Type == "LAUNCH":
						_ = writeCastFrame(serverConn, "receiver-0", "sender-0", castNSReceiver, []byte(`{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"CC32E753","transportId":"tr-1"}]}}`))
					case msg.namespace == castNSSpotifySecure && env.Type == "getInfo":
						_ = writeCastFrame(serverConn, "tr-1", "sender-0", castNSSpotifySecure, []byte(`{"type":"getInfoResponse","payload":{"deviceID":"dev-1","clientID":"cli-1"}}`))
						return
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpClient := okClient
			if tc.client != nil {
				httpClient = tc.client
			}
			client := NewCastWakeupClient(
				WithCastHTTPClient(httpClient),
				WithCastDialer(func(addr string) (net.Conn, error) {
					if tc.dialErr != nil {
						return nil, tc.dialErr
					}
					c, s := net.Pipe()
					go tc.responder(s)
					return c, nil
				}),
			)
			defer client.Close()

			t.Setenv("MYGO_LOCALE", "zh-Hans")
			_, zhErr := client.ConnectSpeaker(context.Background(), &CastSpeaker{ID: "spk-err", Name: "Speaker", IP: "127.0.0.1", Port: 8009})
			if zhErr == nil {
				t.Fatal("expected error in zh-Hans")
			}
			if strings.Contains(zhErr.Error(), "%") {
				t.Fatalf("zh-Hans error contains unformatted verb: %q", zhErr.Error())
			}
			if hasRawEnglishWord(zhErr.Error()) {
				t.Fatalf("zh-Hans error mixes raw English text: %q", zhErr.Error())
			}
			if tc.dialErr != nil && !errors.Is(zhErr, tc.dialErr) {
				t.Fatalf("expected dial error to remain unwrappable via errors.Is")
			}

			t.Setenv("MYGO_LOCALE", "en")
			_, enErr := client.ConnectSpeaker(context.Background(), &CastSpeaker{ID: "spk-err", Name: "Speaker", IP: "127.0.0.1", Port: 8009})
			if enErr == nil {
				t.Fatal("expected error in en")
			}
			if !strings.Contains(strings.ToLower(enErr.Error()), strings.ToLower(tc.wantEnSub)) {
				t.Fatalf("en error = %q, want substring %q", enErr.Error(), tc.wantEnSub)
			}
			if zhErr.Error() == enErr.Error() {
				t.Fatalf("expected distinct localized errors for zh-Hans vs en, got %q", zhErr.Error())
			}
		})
	}
}

// Expired speakers are evicted and subscribers fire on the background ticker even when non-Cast mDNS packets keep arriving without read timeouts.
func TestCastBackgroundExpirationCleansDuringTraffic(t *testing.T) {
	reg := NewSpeakerRegistry()
	reg.Seed(
		&CastSpeaker{
			ID:        "expired-spk",
			Name:      "Offline Speaker",
			IP:        "192.0.2.50",
			Port:      8009,
			ExpiresAt: time.Now().Add(-time.Second),
		},
		&CastSpeaker{
			ID:        "active-spk",
			Name:      "Online Speaker",
			IP:        "192.0.2.51",
			Port:      8009,
			ExpiresAt: time.Now().Add(time.Minute),
		},
	)

	notified := make(chan struct{}, 4)
	unsub := reg.Subscribe(func() {
		select {
		case notified <- struct{}{}:
		default:
		}
	})
	defer unsub()

	conn := &fakePacketConn{
		pkt: buildTestMDNSPacket("AppleTV._airplay._tcp.local", "atv.local", 7000, []string{"model=AppleTV"}),
		src: &net.UDPAddr{IP: net.ParseIP("192.0.2.99"), Port: 5353},
	}

	stop := make(chan struct{})
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		reg.runCastDiscovery(conn, 50*time.Millisecond, 10*time.Millisecond, 45*time.Second, stop)
	}()

	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		close(stop)
		_ = conn.Close()
		<-loopDone
		t.Fatal("timed out waiting for subscriber notification from background expiration cleanup")
	}

	if conn.reads.Load() == 0 {
		close(stop)
		_ = conn.Close()
		<-loopDone
		t.Fatal("expected continuous mDNS reads alongside background cleanup")
	}

	close(stop)
	_ = conn.Close()
	<-loopDone

	got := reg.Speakers()
	if len(got) != 1 || got[0].ID != "active-spk" {
		t.Fatalf("discovered speakers = %+v, want only active-spk", got)
	}
	reg.mu.RLock()
	_, expiredStillPresent := reg.speakers["expired-spk"]
	reg.mu.RUnlock()
	if expiredStillPresent {
		t.Fatal("expected expired-spk to be deleted from registry")
	}
}

// Opening the mDNS listener falls back through ListenMulticastUDP and ListenUDP when binding port 5353 fails.
func TestCastListenMDNSFallback(t *testing.T) {
	var order []string
	reg := NewSpeakerRegistry(withRegistryListeners(
		func(lc *net.ListenConfig, ctx context.Context, network, address string) (net.PacketConn, error) {
			order = append(order, "listenPacket:"+address)
			return nil, errors.New("port 5353 busy")
		},
		func(network string, ifi *net.Interface, gaddr *net.UDPAddr) (*net.UDPConn, error) {
			order = append(order, "listenMulticast:"+gaddr.String())
			return nil, errors.New("multicast bind failed")
		},
		func(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
			order = append(order, "listenUDP:"+laddr.String())
			return &net.UDPConn{}, nil
		},
	))

	conn, err := reg.listenMDNSPacketConn()
	if err != nil {
		t.Fatalf("listenMDNSPacketConn failed: %v", err)
	}
	if conn == nil {
		t.Fatal("expected fallback PacketConn")
	}
	want := []string{
		"listenPacket:0.0.0.0:5353",
		"listenMulticast:224.0.0.251:5353",
		"listenUDP:0.0.0.0:0",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("fallback order = %v, want %v", order, want)
	}
}

func buildTestMDNSPacketWithTTL(instance, host string, port uint16, ttl uint32, includeSRV, includeTXT bool, txt []string) []byte {
	var buf bytes.Buffer
	rrCount := uint16(0)
	if includeSRV {
		rrCount++
	}
	if includeTXT {
		rrCount++
	}
	buf.Write([]byte{0x00, 0x00, 0x84, 0x00, 0x00, 0x00})
	binary.Write(&buf, binary.BigEndian, rrCount)
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00})

	writeName := func(b *bytes.Buffer, name string) {
		for _, part := range strings.Split(name, ".") {
			b.WriteByte(byte(len(part)))
			b.WriteString(part)
		}
		b.WriteByte(0)
	}

	if includeSRV {
		writeName(&buf, instance)
		binary.Write(&buf, binary.BigEndian, uint16(33)) // SRV
		binary.Write(&buf, binary.BigEndian, uint16(1))  // IN
		binary.Write(&buf, binary.BigEndian, ttl)
		var srvData bytes.Buffer
		binary.Write(&srvData, binary.BigEndian, uint16(0))
		binary.Write(&srvData, binary.BigEndian, uint16(0))
		binary.Write(&srvData, binary.BigEndian, port)
		writeName(&srvData, host)
		binary.Write(&buf, binary.BigEndian, uint16(srvData.Len()))
		buf.Write(srvData.Bytes())
	}

	if includeTXT {
		writeName(&buf, instance)
		binary.Write(&buf, binary.BigEndian, uint16(16)) // TXT
		binary.Write(&buf, binary.BigEndian, uint16(1))  // IN
		binary.Write(&buf, binary.BigEndian, ttl)
		var txtData bytes.Buffer
		for _, entry := range txt {
			txtData.WriteByte(byte(len(entry)))
			txtData.WriteString(entry)
		}
		binary.Write(&buf, binary.BigEndian, uint16(txtData.Len()))
		buf.Write(txtData.Bytes())
	}

	return buf.Bytes()
}

func buildTestPTRPacket(service, instance string, ttl uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x00, 0x84, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00})
	writeName := func(b *bytes.Buffer, name string) {
		for _, part := range strings.Split(name, ".") {
			b.WriteByte(byte(len(part)))
			b.WriteString(part)
		}
		b.WriteByte(0)
	}
	writeName(&buf, service)
	binary.Write(&buf, binary.BigEndian, uint16(12)) // PTR
	binary.Write(&buf, binary.BigEndian, uint16(1))  // IN
	binary.Write(&buf, binary.BigEndian, ttl)
	var ptrData bytes.Buffer
	writeName(&ptrData, instance)
	binary.Write(&buf, binary.BigEndian, uint16(ptrData.Len()))
	buf.Write(ptrData.Bytes())
	return buf.Bytes()
}

// Cached Cast speakers stay active for 74 seconds and expire at 75 seconds per ADR-0001.
func TestCastSpeaker75sTTLExpiry(t *testing.T) {
	base := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	now := base
	reg := NewSpeakerRegistry(withRegistryNow(func() time.Time { return now }))

	pkt := buildTestMDNSPacket(
		"Google-Home-Mini-0001._googlecast._tcp.local",
		"bedroom.local",
		8009,
		[]string{"id=spk-75s", "fn=Living Room", "md=Google Home Mini"},
	)
	reg.parseMDNSResponse(pkt, &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 5353})

	now = base.Add(74 * time.Second)
	if got := reg.Speakers(); len(got) != 1 || got[0].ID != "spk-75s" {
		t.Fatalf("at +74s got %+v, want spk-75s present", got)
	}

	notified := false
	reg.Subscribe(func() { notified = true })
	now = base.Add(75 * time.Second)
	if got := reg.Speakers(); len(got) != 0 {
		t.Fatalf("at +75s got %+v, want expired speaker hidden", got)
	}
	reg.cleanExpired()
	if !notified {
		t.Fatal("expected cleanExpired to notify subscribers at 75s")
	}
	reg.mu.RLock()
	_, stillCached := reg.speakers["spk-75s"]
	reg.mu.RUnlock()
	if stillCached {
		t.Fatal("expected spk-75s to be deleted from cache at 75s")
	}
}

// Receiving a TTL=0 mDNS Goodbye via TXT (without fn=), SRV, or PTR immediately removes the matching Cast speaker and notifies subscribers without re-adding it.
func TestCastMDNSGoodbyeImmediateRemoval(t *testing.T) {
	cases := []struct {
		name       string
		goodbyePkt func() []byte
	}{
		{
			name: "TXT-id-only",
			goodbyePkt: func() []byte {
				return buildTestMDNSPacketWithTTL(
					"Google-Home-Mini-0001._googlecast._tcp.local",
					"bedroom.local",
					8009,
					0,
					false,
					true,
					[]string{"id=spk-bye"},
				)
			},
		},
		{
			name: "SRV-only",
			goodbyePkt: func() []byte {
				return buildTestMDNSPacketWithTTL(
					"Google-Home-Mini-0001._googlecast._tcp.local",
					"bedroom.local",
					8009,
					0,
					true,
					false,
					nil,
				)
			},
		},
		{
			name: "PTR-only",
			goodbyePkt: func() []byte {
				return buildTestPTRPacket(
					"_googlecast._tcp.local",
					"Google-Home-Mini-0001._googlecast._tcp.local",
					0,
				)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewSpeakerRegistry()
			var changes atomic.Int32
			reg.Subscribe(func() { changes.Add(1) })

			src := &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 5353}
			announce := buildTestMDNSPacket(
				"Google-Home-Mini-0001._googlecast._tcp.local",
				"bedroom.local",
				8009,
				[]string{"id=spk-bye", "fn=Kitchen", "md=Google Home Mini"},
			)
			reg.parseMDNSResponse(announce, src)
			if got := reg.Speakers(); len(got) != 1 {
				t.Fatalf("setup got %d speakers, want 1", len(got))
			}

			before := changes.Load()
			reg.parseMDNSResponse(tc.goodbyePkt(), src)
			if got := reg.Speakers(); len(got) != 0 {
				t.Fatalf("after %s goodbye got %+v, want 0 speakers", tc.name, got)
			}
			if changes.Load() != before+1 {
				t.Fatalf("expected subscriber notification on %s goodbye", tc.name)
			}

			// A duplicate goodbye for an already removed speaker must not re-add or notify.
			reg.parseMDNSResponse(tc.goodbyePkt(), src)
			if changes.Load() != before+1 {
				t.Fatalf("unexpected notification on duplicate %s goodbye", tc.name)
			}
		})
	}
}

// TXT-only mDNS updates preserve an existing Cast group's dynamic port and SpotifyDeviceID while defaulting new speakers without SRV to 8009.
func TestCastTXTOnlyUpdatePreservesDynamicPort(t *testing.T) {
	reg := NewSpeakerRegistry()

	src := &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 5353}
	groupAnnounce := buildTestMDNSPacket(
		"Google-Cast-Group-0001._googlecast._tcp.local",
		"bedroom.local",
		32072,
		[]string{"id=grp-1", "fn=Home Group", "md=Google Cast Group"},
	)
	reg.parseMDNSResponse(groupAnnounce, src)

	reg.SetSpotifyDeviceID("grp-1", "cloud-grp-1")

	txtOnlyUpdate := buildTestMDNSPacketWithTTL(
		"Google-Cast-Group-0001._googlecast._tcp.local",
		"bedroom.local",
		0,
		120,
		false,
		true,
		[]string{"id=grp-1", "fn=客廳群組", "md=Google Cast Group"},
	)
	reg.parseMDNSResponse(txtOnlyUpdate, src)

	txtOnlyNew := buildTestMDNSPacketWithTTL(
		"Google-Nest-New._googlecast._tcp.local",
		"nest.local",
		0,
		120,
		false,
		true,
		[]string{"id=spk-new", "fn=書房", "md=Google Nest Mini"},
	)
	reg.parseMDNSResponse(txtOnlyNew, &net.UDPAddr{IP: net.ParseIP("192.0.2.12"), Port: 5353})

	got := reg.Speakers()
	if len(got) != 2 {
		t.Fatalf("got %d speakers, want 2", len(got))
	}
	byID := make(map[string]*CastSpeaker, len(got))
	for _, s := range got {
		byID[s.ID] = s
	}
	if byID["grp-1"].Port != 32072 {
		t.Fatalf("grp-1 port = %d, want preserved 32072", byID["grp-1"].Port)
	}
	if byID["grp-1"].Name != "客廳群組" {
		t.Fatalf("grp-1 name = %q, want updated name", byID["grp-1"].Name)
	}
	if byID["grp-1"].SpotifyDeviceID != "cloud-grp-1" {
		t.Fatalf("grp-1 SpotifyDeviceID = %q, want cloud-grp-1", byID["grp-1"].SpotifyDeviceID)
	}
	if byID["spk-new"].Port != 8009 {
		t.Fatalf("new speaker without SRV port = %d, want default 8009", byID["spk-new"].Port)
	}

	srvPortChange := buildTestMDNSPacket(
		"Google-Cast-Group-0001._googlecast._tcp.local",
		"bedroom.local",
		40123,
		[]string{"id=grp-1", "fn=客廳群組", "md=Google Cast Group"},
	)
	reg.parseMDNSResponse(srvPortChange, src)
	for _, s := range reg.Speakers() {
		if s.ID == "grp-1" && s.Port != 40123 {
			t.Fatalf("grp-1 port after SRV update = %d, want 40123", s.Port)
		}
	}
}

// SpeakerRegistry.Start resets started state after a listen failure so subsequent calls can retry while still deduplicating active loops.
func TestStartCastDiscoveryRetryAfterListenFailure(t *testing.T) {
	var listenAttempts atomic.Int32
	var allowSuccess atomic.Bool
	discovered := make(chan struct{}, 1)
	conn := &fakePacketConn{
		pkt: buildTestMDNSPacket(
			"Google-Home-Retry._googlecast._tcp.local",
			"retry.local",
			8009,
			[]string{"id=retry-spk", "fn=Retry Speaker", "md=Google Home"},
		),
		src: &net.UDPAddr{IP: net.ParseIP("192.0.2.200"), Port: 5353},
	}

	reg := NewSpeakerRegistry(
		withRegistryIntervals(50*time.Millisecond, 50*time.Millisecond, 50*time.Millisecond),
		withRegistryListeners(
			func(lc *net.ListenConfig, ctx context.Context, network, address string) (net.PacketConn, error) {
				listenAttempts.Add(1)
				if !allowSuccess.Load() {
					return nil, errors.New("network unavailable")
				}
				return conn, nil
			},
			func(network string, ifi *net.Interface, gaddr *net.UDPAddr) (*net.UDPConn, error) {
				return nil, errors.New("multicast unavailable")
			},
			func(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
				return nil, errors.New("udp unavailable")
			},
		),
	)
	defer reg.Stop()

	reg.Start()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		reg.mu.RLock()
		started := reg.started
		reg.mu.RUnlock()
		if !started && listenAttempts.Load() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	reg.mu.RLock()
	startedAfterFail := reg.started
	reg.mu.RUnlock()
	if startedAfterFail {
		t.Fatal("expected started state to reset after listenMDNSPacketConn failure")
	}

	unsub := reg.Subscribe(func() {
		select {
		case discovered <- struct{}{}:
		default:
		}
	})
	defer unsub()

	allowSuccess.Store(true)
	reg.Start()
	reg.Start() // must be deduplicated while active

	select {
	case <-discovered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for retried Start to discover speaker")
	}
	if got := listenAttempts.Load(); got != 2 {
		t.Fatalf("listenAttempts = %d, want 2 (initial fail + single active retry)", got)
	}
}

type fakeErrorBackoffConn struct {
	calls     atomic.Int32
	failUntil int32
	pkt       []byte
	src       net.Addr
	onSuccess chan struct{}
}

func (f *fakeErrorBackoffConn) ReadFrom(p []byte) (int, net.Addr, error) {
	c := f.calls.Add(1)
	if c <= f.failUntil || c > f.failUntil+1 {
		return 0, nil, errors.New("persistent read error")
	}
	n := copy(p, f.pkt)
	select {
	case f.onSuccess <- struct{}{}:
	default:
	}
	return n, f.src, nil
}

func (f *fakeErrorBackoffConn) WriteTo(p []byte, addr net.Addr) (int, error) { return len(p), nil }
func (f *fakeErrorBackoffConn) Close() error                                 { return nil }
func (f *fakeErrorBackoffConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4zero, Port: 5353}
}
func (f *fakeErrorBackoffConn) SetDeadline(t time.Time) error      { return nil }
func (f *fakeErrorBackoffConn) SetReadDeadline(t time.Time) error  { return nil }
func (f *fakeErrorBackoffConn) SetWriteDeadline(t time.Time) error { return nil }

// Persistent non-timeout ReadFrom errors apply bounded backoff, reset on successful reads, and exit immediately when stop is closed.
func TestCastReadFromPersistentErrorBackoff(t *testing.T) {
	reg := NewSpeakerRegistry(withRegistryReadBackoff(15*time.Millisecond, 40*time.Millisecond))

	conn := &fakeErrorBackoffConn{
		failUntil: 4,
		pkt: buildTestMDNSPacket(
			"Google-Home-Backoff._googlecast._tcp.local",
			"backoff.local",
			8009,
			[]string{"id=backoff-spk", "fn=Backoff", "md=Google Home"},
		),
		src:       &net.UDPAddr{IP: net.ParseIP("192.0.2.210"), Port: 5353},
		onSuccess: make(chan struct{}, 1),
	}

	stop := make(chan struct{})
	loopDone := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(loopDone)
		reg.runCastDiscovery(conn, time.Second, time.Second, time.Second, stop)
	}()

	select {
	case <-conn.onSuccess:
	case <-time.After(2 * time.Second):
		close(stop)
		<-loopDone
		t.Fatal("timed out waiting for packet read after backoff")
	}

	elapsed := time.Since(start)
	// 4 failures at 15 + 30 + 40 + 40 = 125ms minimum; without backoff it would take <1ms.
	if elapsed < 90*time.Millisecond {
		close(stop)
		<-loopDone
		t.Fatalf("4 persistent errors completed too fast (%v), backoff not applied", elapsed)
	}

	// Wait slightly so it enters backoff again after the successful read, then stop must interrupt promptly.
	time.Sleep(5 * time.Millisecond)
	stopAt := time.Now()
	close(stop)
	select {
	case <-loopDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runCastDiscovery did not exit promptly when stop closed during backoff")
	}
	if stopDelay := time.Since(stopAt); stopDelay > 200*time.Millisecond {
		t.Fatalf("stop took %v, expected immediate interruption", stopDelay)
	}
}

// ConnectSpeaker validates non-empty speaker IPs and formats IPv4 and IPv6 addresses with net.JoinHostPort.
func TestCastConnectJoinHostPortAndEmptyIP(t *testing.T) {
	dialCalled := false
	var dialedAddrs []string
	dialErr := errors.New("stop after dial")
	client := NewCastWakeupClient(WithCastDialer(func(addr string) (net.Conn, error) {
		dialCalled = true
		dialedAddrs = append(dialedAddrs, addr)
		return nil, dialErr
	}))
	defer client.Close()

	if _, err := client.ConnectSpeaker(context.Background(), &CastSpeaker{ID: "empty", IP: "   ", Port: 8009}); err == nil {
		t.Fatal("expected error for empty speaker IP")
	}
	if dialCalled {
		t.Fatal("expected empty IP validation to fail before calling dialer")
	}

	if _, err := client.ConnectSpeaker(context.Background(), &CastSpeaker{ID: "v4", IP: "192.0.2.10", Port: 8009}); !errors.Is(err, dialErr) {
		t.Fatalf("unexpected error for IPv4: %v", err)
	}
	if _, err := client.ConnectSpeaker(context.Background(), &CastSpeaker{ID: "v6", IP: "fe80::1", Port: 32072}); !errors.Is(err, dialErr) {
		t.Fatalf("unexpected error for IPv6: %v", err)
	}

	want := []string{"192.0.2.10:8009", "[fe80::1]:32072"}
	if strings.Join(dialedAddrs, ",") != strings.Join(want, ",") {
		t.Fatalf("dialed addresses = %v, want %v", dialedAddrs, want)
	}
}

// ConnectSpeaker with an empty or whitespace IP rejects before dialing and returns distinct localized errors for zh-Hans, en, and it.
func TestCastWakeupEmptyIPLocalization(t *testing.T) {
	previousLocale := i18n.Locale()
	i18n.SetLocaleFunc(func() string {
		if v := os.Getenv("MYGO_LOCALE"); v != "" {
			return v
		}
		return "zh-Hans"
	})
	defer i18n.SetLocaleFunc(func() string { return previousLocale })

	dialCalled := false
	client := NewCastWakeupClient(WithCastDialer(func(addr string) (net.Conn, error) {
		dialCalled = true
		return nil, errors.New("should not dial")
	}))
	defer client.Close()

	hasRawEnglishWord := func(s string) bool {
		cleaned := strings.NewReplacer("Spotify", "", "Cast", "").Replace(s)
		for _, r := range cleaned {
			if r <= unicode.MaxASCII && unicode.IsLetter(r) {
				return true
			}
		}
		return false
	}

	for _, ip := range []string{"", "   "} {
		gotByLocale := make(map[string]string, 3)
		for _, locale := range []string{"zh-Hans", "en", "it"} {
			t.Setenv("MYGO_LOCALE", locale)
			_, err := client.ConnectSpeaker(context.Background(), &CastSpeaker{ID: "empty", IP: ip, Port: 8009})
			if err == nil {
				t.Fatalf("ip=%q locale=%s: expected error for empty IP", ip, locale)
			}
			if err.Error() == "cast speaker IP is empty" {
				t.Fatalf("ip=%q locale=%s: got unlocalized fallback error %q", ip, locale, err.Error())
			}
			gotByLocale[locale] = err.Error()
		}
		if dialCalled {
			t.Fatalf("ip=%q: expected empty IP validation to fail before calling dialer", ip)
		}
		if hasRawEnglishWord(gotByLocale["zh-Hans"]) {
			t.Fatalf("ip=%q: zh-Hans error mixes raw English text: %q", ip, gotByLocale["zh-Hans"])
		}
		if gotByLocale["en"] == "" || gotByLocale["it"] == "" || gotByLocale["zh-Hans"] == "" {
			t.Fatalf("ip=%q: expected non-empty localized errors, got %v", ip, gotByLocale)
		}
		if gotByLocale["zh-Hans"] == gotByLocale["en"] || gotByLocale["zh-Hans"] == gotByLocale["it"] || gotByLocale["en"] == gotByLocale["it"] {
			t.Fatalf("ip=%q: expected distinct localized errors, got %v", ip, gotByLocale)
		}
	}
}

// SpeakerRegistry.Subscribe supports multiple independent subscribers and returns an idempotent unsubscribe function that removes only its own callback.
func TestSpeakerRegistrySubscribeMultipleAndIdempotentUnsubscribe(t *testing.T) {
	reg := NewSpeakerRegistry()
	var c1, c2, c3 atomic.Int32

	unsubNil := reg.Subscribe(nil)
	unsubNil()

	unsub1 := reg.Subscribe(func() { c1.Add(1) })
	unsub2 := reg.Subscribe(func() { c2.Add(1) })
	unsub3 := reg.Subscribe(func() { c3.Add(1) })

	reg.Upsert(&CastSpeaker{ID: "spk-1", Name: "One", IP: "192.0.2.10", Port: 8009})
	if c1.Load() != 1 || c2.Load() != 1 || c3.Load() != 1 {
		t.Fatalf("initial counts = (%d, %d, %d), want (1, 1, 1)", c1.Load(), c2.Load(), c3.Load())
	}

	// Removing subscriber 1 multiple times leaves subscribers 2 and 3 active.
	unsub1()
	unsub1()
	reg.Upsert(&CastSpeaker{ID: "spk-1", Name: "One Updated", IP: "192.0.2.10", Port: 8009})
	if c1.Load() != 1 || c2.Load() != 2 || c3.Load() != 2 {
		t.Fatalf("after unsub1 counts = (%d, %d, %d), want (1, 2, 2)", c1.Load(), c2.Load(), c3.Load())
	}

	unsub2()
	reg.Upsert(&CastSpeaker{ID: "spk-2", Name: "Two", IP: "192.0.2.11", Port: 8009})
	if c1.Load() != 1 || c2.Load() != 2 || c3.Load() != 3 {
		t.Fatalf("after unsub2 counts = (%d, %d, %d), want (1, 2, 3)", c1.Load(), c2.Load(), c3.Load())
	}

	unsub3()
	unsub3()
}

type blockingPacketConn struct {
	mu          sync.Mutex
	pkt         []byte
	src         net.Addr
	deliverOnce bool
	delivered   bool
	readingOnce sync.Once
	readingCh   chan struct{}
	closeOnce   sync.Once
	closedCh    chan struct{}
	closed      atomic.Bool
}

func newBlockingPacketConn(pkt []byte, src net.Addr, deliverOnce bool) *blockingPacketConn {
	return &blockingPacketConn{
		pkt:         pkt,
		src:         src,
		deliverOnce: deliverOnce,
		readingCh:   make(chan struct{}),
		closedCh:    make(chan struct{}),
	}
}

func (b *blockingPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	b.readingOnce.Do(func() { close(b.readingCh) })
	if b.deliverOnce {
		b.mu.Lock()
		if !b.delivered {
			b.delivered = true
			n := copy(p, b.pkt)
			b.mu.Unlock()
			return n, b.src, nil
		}
		b.mu.Unlock()
	}
	<-b.closedCh
	return 0, nil, net.ErrClosed
}

func (b *blockingPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if b.closed.Load() {
		return 0, net.ErrClosed
	}
	return len(p), nil
}

func (b *blockingPacketConn) Close() error {
	b.closed.Store(true)
	b.closeOnce.Do(func() { close(b.closedCh) })
	return nil
}

func (b *blockingPacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4zero, Port: 5353}
}

func (b *blockingPacketConn) SetDeadline(t time.Time) error      { return nil }
func (b *blockingPacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (b *blockingPacketConn) SetWriteDeadline(t time.Time) error { return nil }

// SpeakerRegistry.Stop closes the active listener, waits for discovery goroutines to exit without races, and allows Start to run again cleanly.
func TestSpeakerRegistryStopAndRestartLifecycle(t *testing.T) {
	conn1 := newBlockingPacketConn(nil, nil, false)
	conn2 := newBlockingPacketConn(
		buildTestMDNSPacket(
			"Google-Nest-Restart._googlecast._tcp.local",
			"restart.local",
			8009,
			[]string{"id=restart-spk", "fn=Restart Speaker", "md=Google Nest Mini"},
		),
		&net.UDPAddr{IP: net.ParseIP("192.0.2.220"), Port: 5353},
		true,
	)

	var round atomic.Int32
	reg := NewSpeakerRegistry(
		withRegistryIntervals(50*time.Millisecond, 50*time.Millisecond, time.Second),
		withRegistryListeners(
			func(lc *net.ListenConfig, ctx context.Context, network, address string) (net.PacketConn, error) {
				if round.Add(1) == 1 {
					return conn1, nil
				}
				return conn2, nil
			},
			nil,
			nil,
		),
	)
	defer reg.Stop()

	reg.Start()
	select {
	case <-conn1.readingCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial discovery loop to enter ReadFrom")
	}

	reg.Stop()
	if !conn1.closed.Load() {
		t.Fatal("expected Stop to close active PacketConn")
	}
	reg.Stop() // idempotent when already stopped

	discovered := make(chan struct{}, 1)
	unsub := reg.Subscribe(func() {
		select {
		case discovered <- struct{}{}:
		default:
		}
	})
	defer unsub()

	reg.Start()
	select {
	case <-discovered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for restarted discovery loop to discover speaker")
	}

	got := reg.Speakers()
	if len(got) != 1 || got[0].ID != "restart-spk" {
		t.Fatalf("speakers after restart = %+v, want restart-spk", got)
	}

	reg.Stop()
	if !conn2.closed.Load() {
		t.Fatal("expected second Stop to close restarted PacketConn")
	}
}

type trackingConn struct {
	net.Conn
	closed    atomic.Bool
	closeOnce sync.Once
	closedCh  chan struct{}
}

func newTrackingConn(c net.Conn) *trackingConn {
	return &trackingConn{Conn: c, closedCh: make(chan struct{})}
}

func (t *trackingConn) Close() error {
	t.closed.Store(true)
	t.closeOnce.Do(func() { close(t.closedCh) })
	return t.Conn.Close()
}

// CastWakeupClient.Close cancels all active heartbeat goroutines and closes their connections cleanly.
func TestCastWakeupClientCloseStopsHeartbeats(t *testing.T) {
	oldID, _ := CurrentIdentity()
	defer SetIdentity(oldID)
	SetIdentity(Identity{Token: "web-tok", ClientToken: "client-tok", Expires: time.Now().Add(time.Hour)})

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(map[string]string{"accessToken": "cast-tok-close"})
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}

	var mu sync.Mutex
	var clientConns []*trackingConn
	var serverDones []chan struct{}
	client := NewCastWakeupClient(
		WithCastHTTPClient(httpClient),
		WithCastDialer(func(addr string) (net.Conn, error) {
			rawClient, serverConn := net.Pipe()
			tc := newTrackingConn(rawClient)
			done := make(chan struct{})
			mu.Lock()
			clientConns = append(clientConns, tc)
			serverDones = append(serverDones, done)
			mu.Unlock()

			go func() {
				defer close(done)
				defer serverConn.Close()
				for {
					msg, err := readCastFrame(serverConn)
					if err != nil {
						return
					}
					var env struct {
						Type string `json:"type"`
					}
					_ = json.Unmarshal([]byte(msg.payloadUTF8), &env)
					switch {
					case msg.namespace == castNSReceiver && env.Type == "LAUNCH":
						_ = writeCastFrame(serverConn, "receiver-0", "sender-0", castNSReceiver, []byte(`{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"CC32E753","transportId":"tr-1"}]}}`))
					case msg.namespace == castNSSpotifySecure && env.Type == "getInfo":
						_ = writeCastFrame(serverConn, "tr-1", "sender-0", castNSSpotifySecure, []byte(`{"type":"getInfoResponse","payload":{"deviceID":"dev-hb","clientID":"cli-1"}}`))
					case msg.namespace == castNSSpotifySecure && env.Type == "addUser":
						_ = writeCastFrame(serverConn, "tr-1", "sender-0", castNSSpotifySecure, []byte(`{"type":"addUserResponse","payload":{"status":101}}`))
					}
				}
			}()
			return tc, nil
		}),
	)

	for i := 0; i < 2; i++ {
		devID, err := client.ConnectSpeaker(context.Background(), &CastSpeaker{
			ID:   "spk-" + strconv.Itoa(i),
			Name: "Speaker",
			IP:   "127.0.0.1",
			Port: 8009 + i,
		})
		if err != nil {
			t.Fatalf("ConnectSpeaker #%d failed: %v", i, err)
		}
		if devID != "dev-hb" {
			t.Fatalf("ConnectSpeaker #%d deviceID = %q, want dev-hb", i, devID)
		}
	}

	client.Close()
	client.Close() // idempotent

	mu.Lock()
	defer mu.Unlock()
	if len(clientConns) != 2 {
		t.Fatalf("dialed %d connections, want 2", len(clientConns))
	}
	for i, tc := range clientConns {
		if !tc.closed.Load() {
			t.Fatalf("connection #%d was not closed by CastWakeupClient.Close", i)
		}
		select {
		case <-serverDones[i]:
		case <-time.After(2 * time.Second):
			t.Fatalf("server #%d did not observe connection close after CastWakeupClient.Close", i)
		}
	}
}

// CastWakeupClient.ConnectSpeaker aborts immediately when ctx is already canceled or canceled mid-dial or mid-handshake.
func TestCastWakeupClientConnectSpeakerContextCanceled(t *testing.T) {
	t.Run("pre-canceled", func(t *testing.T) {
		var dialCalled atomic.Bool
		client := NewCastWakeupClient(WithCastDialer(func(addr string) (net.Conn, error) {
			dialCalled.Store(true)
			return nil, errors.New("should not dial")
		}))
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := client.ConnectSpeaker(ctx, &CastSpeaker{ID: "spk-1", IP: "127.0.0.1", Port: 8009})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if dialCalled.Load() {
			t.Fatal("expected pre-canceled context to abort before dialing")
		}
	})

	t.Run("canceled-during-dial", func(t *testing.T) {
		dialStarted := make(chan struct{})
		releaseDial := make(chan struct{})
		cConn, sConn := net.Pipe()
		defer sConn.Close()
		tc := newTrackingConn(cConn)

		client := NewCastWakeupClient(WithCastDialer(func(addr string) (net.Conn, error) {
			close(dialStarted)
			<-releaseDial
			return tc, nil
		}))
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			_, err := client.ConnectSpeaker(ctx, &CastSpeaker{ID: "spk-dial", IP: "127.0.0.1", Port: 8009})
			errCh <- err
		}()

		<-dialStarted
		cancel()

		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			close(releaseDial)
			t.Fatal("ConnectSpeaker did not abort immediately on context cancellation during dial")
		}

		close(releaseDial)
		select {
		case <-tc.closedCh:
		case <-time.After(time.Second):
			t.Fatal("expected late dialed connection to be closed after context cancellation")
		}
	})

	t.Run("canceled-during-handshake", func(t *testing.T) {
		launchReceived := make(chan struct{})
		cConn, sConn := net.Pipe()
		tc := newTrackingConn(cConn)
		client := NewCastWakeupClient(WithCastDialer(func(addr string) (net.Conn, error) {
			go func() {
				defer sConn.Close()
				for {
					msg, err := readCastFrame(sConn)
					if err != nil {
						return
					}
					if msg.namespace == castNSReceiver {
						close(launchReceived)
					}
				}
			}()
			return tc, nil
		}))
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			_, err := client.ConnectSpeaker(ctx, &CastSpeaker{ID: "spk-hs", IP: "127.0.0.1", Port: 8009})
			errCh <- err
		}()

		<-launchReceived
		cancel()

		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("ConnectSpeaker did not abort immediately on context cancellation during handshake")
		}
		select {
		case <-tc.closedCh:
		case <-time.After(time.Second):
			t.Fatal("expected handshake connection to be closed on context cancellation")
		}
	})
}
