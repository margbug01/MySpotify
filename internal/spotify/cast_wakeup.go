package spotify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"vinyl/internal/l10n"
)

const castSenderID = "sender-0"

// castHeartbeatTimeout is how long a woken receiver may go without a frame
// (it pings every few seconds) before its connection is let go; tests
// shorten it.
var castHeartbeatTimeout = 20 * time.Second

// CastWakeupClient performs CastV2 receiver launch, Spotify auth handshakes, and heartbeat maintenance.
type CastWakeupClient struct {
	mu         sync.Mutex
	dial       func(addr string) (net.Conn, error)
	httpClient *http.Client
	now        func() time.Time
	closed     bool
	heartbeats map[net.Conn]context.CancelFunc
	wg         sync.WaitGroup
}

// CastWakeupOption configures a CastWakeupClient instance.
type CastWakeupOption func(*CastWakeupClient)

// WithCastDialer overrides the TLS dialer used to connect to Cast speakers.
func WithCastDialer(fn func(addr string) (net.Conn, error)) CastWakeupOption {
	return func(c *CastWakeupClient) {
		if fn != nil {
			c.dial = fn
		}
	}
}

// WithCastHTTPClient overrides the HTTP client used for Spotify device-auth token exchange.
func WithCastHTTPClient(hc *http.Client) CastWakeupOption {
	return func(c *CastWakeupClient) {
		c.httpClient = hc
	}
}

// WithCastNow overrides the clock function used for connection deadlines.
func WithCastNow(fn func() time.Time) CastWakeupOption {
	return func(c *CastWakeupClient) {
		if fn != nil {
			c.now = fn
		}
	}
}

func defaultCastDial(addr string) (net.Conn, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	return tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
}

// NewCastWakeupClient constructs a CastWakeupClient with default TLS dialing and lifecycle tracking.
func NewCastWakeupClient(opts ...CastWakeupOption) *CastWakeupClient {
	c := &CastWakeupClient{
		dial:       defaultCastDial,
		now:        time.Now,
		heartbeats: make(map[net.Conn]context.CancelFunc),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// ConnectSpeaker wakes the Spotify Cast Receiver (CC32E753) on spk, completes getInfo/addUser auth,
// keeps the receiver alive with a background heartbeat, and returns the Spotify Connect device ID.
func (c *CastWakeupClient) ConnectSpeaker(ctx context.Context, spk *CastSpeaker) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	addr, err := validateCastSpeakerAddr(spk)
	if err != nil {
		return "", err
	}

	conn, err := c.dialAndLaunchReceiver(ctx, addr)
	if err != nil {
		return "", err
	}

	transportID, spotifyDeviceID, err := c.performHandshake(ctx, conn, spk, addr)
	if err != nil {
		_ = conn.Close()
		return "", err
	}

	c.startHeartbeat(conn, transportID)
	return spotifyDeviceID, nil
}

// Close cancels all active heartbeat goroutines, closes their connections, and waits for them to exit.
func (c *CastWakeupClient) Close() {
	c.mu.Lock()
	c.closed = true
	for conn, cancel := range c.heartbeats {
		cancel()
		_ = conn.Close()
	}
	c.heartbeats = make(map[net.Conn]context.CancelFunc)
	c.mu.Unlock()
	c.wg.Wait()
}

func validateCastSpeakerAddr(spk *CastSpeaker) (string, error) {
	if spk == nil {
		return "", errors.New(l10n.T("未指定 Cast 音响"))
	}
	ip := strings.TrimSpace(spk.IP)
	if ip == "" {
		return "", errors.New(l10n.T("Cast 音响地址为空"))
	}
	port := spk.Port
	if port <= 0 {
		port = 8009
	}
	return net.JoinHostPort(ip, strconv.Itoa(port)), nil
}

func (c *CastWakeupClient) dialContext(ctx context.Context, addr string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type dialResult struct {
		conn net.Conn
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := c.dial(addr)
		ch <- dialResult{conn: conn, err: err}
	}()

	select {
	case res := <-ch:
		return res.conn, res.err
	case <-ctx.Done():
		go func() {
			if res := <-ch; res.conn != nil {
				_ = res.conn.Close()
			}
		}()
		return nil, ctx.Err()
	}
}

func (c *CastWakeupClient) sendCastCommand(ctx context.Context, conn net.Conn, destID, namespace string, payload []byte, addr, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeCastFrame(conn, castSenderID, destID, namespace, payload); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		log.Printf("cast: %s %s: %v", action, addr, err)
		return &castError{msg: l10n.T("向 Cast 音响发送指令失败"), cause: err}
	}
	return nil
}

func (c *CastWakeupClient) dialAndLaunchReceiver(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := c.dialContext(ctx, addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		log.Printf("cast: dial %s: %v", addr, err)
		return nil, &castError{msg: l10n.T("无法连接 Cast 音响"), cause: err}
	}

	deadline := c.now().Add(12 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	if err := c.sendCastCommand(ctx, conn, "receiver-0", castNSConnection, []byte(`{"type":"CONNECT"}`), addr, "connect receiver"); err != nil {
		_ = conn.Close()
		return nil, err
	}

	launchPayload, _ := json.Marshal(map[string]any{
		"type":      "LAUNCH",
		"appId":     SpotifyCastAppID,
		"requestId": 1,
	})
	if err := c.sendCastCommand(ctx, conn, "receiver-0", castNSReceiver, launchPayload, addr, "launch"); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

type castEnvelope struct {
	Type    string          `json:"type"`
	Status  json.RawMessage `json:"status"`
	Payload json.RawMessage `json:"payload"`
}

func (c *CastWakeupClient) performHandshake(ctx context.Context, conn net.Conn, spk *CastSpeaker, addr string) (string, string, error) {
	stopWatch := make(chan struct{})
	var watchWG sync.WaitGroup
	watchWG.Add(1)
	go func() {
		defer watchWG.Done()
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stopWatch:
		}
	}()
	defer func() {
		close(stopWatch)
		watchWG.Wait()
	}()

	var transportID string
	var spotifyDeviceID string
	sentAddUser := false

	for i := 0; i < 24; i++ {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		msg, err := readCastFrame(conn)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", "", ctxErr
			}
			log.Printf("cast: read frame %s: %v", addr, err)
			return "", "", &castError{msg: l10n.T("读取 Cast 音响响应失败"), cause: err}
		}
		if msg.payloadUTF8 == "" {
			continue
		}

		done, err := c.dispatchHandshakeMessage(ctx, conn, spk, addr, msg, &transportID, &spotifyDeviceID, &sentAddUser)
		if err != nil {
			return "", "", err
		}
		if done {
			return transportID, spotifyDeviceID, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	log.Printf("cast: handshake with %s timed out", addr)
	return "", "", errors.New(l10n.T("Cast 握手超时"))
}

func (c *CastWakeupClient) dispatchHandshakeMessage(
	ctx context.Context,
	conn net.Conn,
	spk *CastSpeaker,
	addr string,
	msg castMessage,
	transportID *string,
	spotifyDeviceID *string,
	sentAddUser *bool,
) (bool, error) {
	var env castEnvelope
	if err := json.Unmarshal([]byte(msg.payloadUTF8), &env); err != nil {
		return false, nil
	}

	switch msg.namespace {
	case castNSHeartbeat:
		if env.Type == "PING" {
			_ = writeCastFrame(conn, castSenderID, "receiver-0", castNSHeartbeat, []byte(`{"type":"PONG"}`))
		}
	case castNSReceiver:
		return false, c.handleReceiverMessage(ctx, conn, spk, addr, env, transportID)
	case castNSSpotifySecure:
		devID, done, err := c.handleSecureMessage(ctx, conn, *transportID, spk, addr, []byte(msg.payloadUTF8), sentAddUser)
		if devID != "" {
			*spotifyDeviceID = devID
		}
		return done, err
	}
	return false, nil
}

func (c *CastWakeupClient) handleReceiverMessage(ctx context.Context, conn net.Conn, spk *CastSpeaker, addr string, env castEnvelope, transportID *string) error {
	switch env.Type {
	case "LAUNCH_ERROR":
		log.Printf("cast: receiver %s refused to launch Spotify app", addr)
		return errors.New(l10n.T("音响拒绝启动 Spotify 应用"))
	case "RECEIVER_STATUS":
		if *transportID != "" || len(env.Status) == 0 {
			return nil
		}
		var status struct {
			Applications []struct {
				AppID       string `json:"appId"`
				TransportID string `json:"transportId"`
			} `json:"applications"`
		}
		if json.Unmarshal(env.Status, &status) != nil {
			return nil
		}
		for _, app := range status.Applications {
			if app.AppID != SpotifyCastAppID || app.TransportID == "" {
				continue
			}
			*transportID = app.TransportID
			if err := c.sendCastCommand(ctx, conn, *transportID, castNSConnection, []byte(`{"type":"CONNECT"}`), addr, "connect transport"); err != nil {
				return err
			}
			remoteName := spk.Name
			if remoteName == "" {
				remoteName = "Google Cast Speaker"
			}
			devID := spk.ID
			if devID == "" {
				devID = "myspotify-cast"
			}
			isGroup := strings.Contains(strings.ToLower(spk.Model), "group")
			getInfoBody, _ := json.Marshal(map[string]any{
				"type": "getInfo",
				"payload": map[string]any{
					"remoteName":        remoteName,
					"deviceID":          devID,
					"deviceAPI_isGroup": isGroup,
				},
			})
			if err := c.sendCastCommand(ctx, conn, *transportID, castNSSpotifySecure, getInfoBody, addr, "getInfo"); err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

func (c *CastWakeupClient) handleSecureMessage(
	ctx context.Context,
	conn net.Conn,
	transportID string,
	spk *CastSpeaker,
	addr string,
	payload []byte,
	sentAddUser *bool,
) (string, bool, error) {
	var env castEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return "", false, nil
	}

	switch env.Type {
	case "getInfoError", "addUserError":
		log.Printf("cast: speaker %s rejected Spotify auth (%s)", addr, env.Type)
		return "", false, errors.New(l10n.T("音响拒绝了 Spotify 认证"))
	case "getInfoResponse":
		if *sentAddUser {
			return "", false, nil
		}
		*sentAddUser = true
		var info struct {
			DeviceID string `json:"deviceID"`
			ClientID string `json:"clientID"`
		}
		if err := json.Unmarshal(env.Payload, &info); err != nil || info.DeviceID == "" {
			log.Printf("cast: invalid getInfoResponse from %s: %v", addr, err)
			return "", false, &castError{msg: l10n.T("音响未返回有效的设备信息"), cause: err}
		}
		castToken, err := c.fetchDeviceAuthToken(ctx, info.ClientID, info.DeviceID, true)
		if err != nil {
			return "", false, err
		}
		addUserBody, _ := json.Marshal(map[string]any{
			"type": "addUser",
			"payload": map[string]any{
				"tokenType": "accesstoken",
				"blob":      castToken,
			},
		})
		if err := c.sendCastCommand(ctx, conn, transportID, castNSSpotifySecure, addUserBody, addr, "addUser"); err != nil {
			return "", false, err
		}
		return info.DeviceID, false, nil
	case "addUserResponse":
		return "", true, nil
	}
	return "", false, nil
}

func (c *CastWakeupClient) startHeartbeat(conn net.Conn, transportID string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return
	}
	if c.heartbeats == nil {
		c.heartbeats = make(map[net.Conn]context.CancelFunc)
	}
	hbCtx, hbCancel := context.WithCancel(context.Background())
	c.heartbeats[conn] = hbCancel
	c.wg.Add(1)
	c.mu.Unlock()

	go func() {
		defer func() {
			hbCancel()
			_ = conn.Close()
			c.mu.Lock()
			delete(c.heartbeats, conn)
			c.mu.Unlock()
			c.wg.Done()
		}()

		for {
			if hbCtx.Err() != nil {
				return
			}
			_ = conn.SetDeadline(c.now().Add(castHeartbeatTimeout))
			m, err := readCastFrame(conn)
			if err != nil {
				return
			}
			if m.namespace == castNSHeartbeat && m.payloadUTF8 != "" {
				var hb struct {
					Type string `json:"type"`
				}
				if json.Unmarshal([]byte(m.payloadUTF8), &hb) == nil && hb.Type == "PING" {
					if err := writeCastFrame(conn, castSenderID, "receiver-0", castNSHeartbeat, []byte(`{"type":"PONG"}`)); err != nil {
						return
					}
				}
			}
		}
	}()
}

func (c *CastWakeupClient) fetchDeviceAuthToken(ctx context.Context, clientID, deviceID string, renew bool) (string, error) {
	id, err := CurrentIdentity()
	if err != nil {
		log.Printf("cast: device-auth identity: %v", err)
		return "", &castError{msg: l10n.T("Cast: 请重新登录"), cause: err}
	}
	if clientID == "" {
		clientID = SpotifyCastClientID
	}
	rawBody, err := json.Marshal(map[string]string{
		"clientId": clientID,
		"deviceId": deviceID,
	})
	if err != nil {
		log.Printf("cast: device-auth marshal: %v", err)
		return "", &castError{msg: l10n.T("Cast 授权请求无效"), cause: err}
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, spclientURL+"/device-auth/v1/refresh", bytes.NewReader(rawBody))
	if err != nil {
		log.Printf("cast: device-auth request build: %v", err)
		return "", &castError{msg: l10n.T("Cast 授权请求无效"), cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+id.Token)
	if id.ClientToken != "" {
		req.Header.Set("client-token", id.ClientToken)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Origin", "https://open.spotify.com")
	req.Header.Set("Referer", "https://open.spotify.com/")
	req.Header.Set("app-platform", "WebPlayer")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = Client
	}
	res, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		log.Printf("cast: device-auth request: %v", err)
		return "", &castError{msg: l10n.T("Cast 授权网络请求失败"), cause: err}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		log.Printf("cast: device-auth read response: %v", err)
		return "", &castError{msg: l10n.T("读取 Cast 授权响应失败"), cause: err}
	}
	if res.StatusCode == http.StatusUnauthorized && renew {
		if _, err := RenewedIdentity(); err != nil {
			log.Printf("cast: device-auth renew identity: %v", err)
			return "", &castError{msg: l10n.T("Cast 身份刷新失败，请重新登录"), cause: err}
		}
		return c.fetchDeviceAuthToken(ctx, clientID, deviceID, false)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		log.Printf("cast: device-auth HTTP %d", res.StatusCode)
		return "", fmt.Errorf(l10n.T("Cast 授权服务返回错误（%d）"), res.StatusCode)
	}
	var reply struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		log.Printf("cast: device-auth invalid json: %v", err)
		return "", &castError{msg: l10n.T("Cast 授权响应无效"), cause: err}
	}
	if reply.AccessToken == "" {
		log.Printf("cast: device-auth empty accessToken")
		return "", errors.New(l10n.T("Cast 授权响应无效"))
	}
	return reply.AccessToken, nil
}

func encodeCastBytes(fieldNum int, val []byte) []byte {
	out := make([]byte, 0, 6+len(val))
	out = append(out, byte((fieldNum<<3)|2))
	n := len(val)
	for n >= 0x80 {
		out = append(out, byte(n)|0x80)
		n >>= 7
	}
	out = append(out, byte(n))
	return append(out, val...)
}

func encodeCastVarint(fieldNum int, val uint64) []byte {
	out := make([]byte, 0, 12)
	out = binary.AppendUvarint(out, uint64((fieldNum<<3)|0))
	out = binary.AppendUvarint(out, val)
	return out
}

func writeCastFrame(conn net.Conn, sourceID, destID, namespace string, payload []byte) error {
	body := make([]byte, 0, 128+len(payload))
	body = append(body, encodeCastVarint(1, 0)...)
	body = append(body, encodeCastBytes(2, []byte(sourceID))...)
	body = append(body, encodeCastBytes(3, []byte(destID))...)
	body = append(body, encodeCastBytes(4, []byte(namespace))...)
	body = append(body, encodeCastVarint(5, 0)...)
	body = append(body, encodeCastBytes(6, payload)...)

	packet := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(packet[:4], uint32(len(body)))
	copy(packet[4:], body)
	_, err := conn.Write(packet)
	return err
}

type castMessage struct {
	sourceID      string
	destinationID string
	namespace     string
	payloadType   uint64
	payloadUTF8   string
	payloadBinary []byte
}

func decodeCastMessage(data []byte) (castMessage, error) {
	var msg castMessage
	pos := 0
	for pos < len(data) {
		tag, n := binary.Uvarint(data[pos:])
		if n <= 0 {
			return msg, errors.New("invalid cast protobuf tag")
		}
		pos += n
		fieldNum := tag >> 3
		wireType := tag & 0x7
		if fieldNum == 0 {
			return msg, errors.New("invalid cast protobuf field number 0")
		}

		switch wireType {
		case 0: // Varint
			val, vn := binary.Uvarint(data[pos:])
			if vn <= 0 {
				return msg, errors.New("invalid cast protobuf varint")
			}
			pos += vn
			if fieldNum == 5 {
				msg.payloadType = val
			}
		case 1: // 64-bit
			if len(data)-pos < 8 {
				return msg, errors.New("truncated cast protobuf fixed64")
			}
			pos += 8
		case 2: // Length-delimited
			l, ln := binary.Uvarint(data[pos:])
			if ln <= 0 {
				return msg, errors.New("invalid cast protobuf length")
			}
			pos += ln
			if l > uint64(len(data)-pos) {
				return msg, errors.New("truncated cast protobuf bytes")
			}
			val := data[pos : pos+int(l)]
			pos += int(l)
			switch fieldNum {
			case 2:
				msg.sourceID = string(val)
			case 3:
				msg.destinationID = string(val)
			case 4:
				msg.namespace = string(val)
			case 6:
				msg.payloadUTF8 = string(val)
			case 7:
				msg.payloadBinary = append([]byte(nil), val...)
			}
		case 5: // 32-bit
			if len(data)-pos < 4 {
				return msg, errors.New("truncated cast protobuf fixed32")
			}
			pos += 4
		default:
			return msg, fmt.Errorf("unsupported cast protobuf wire type %d", wireType)
		}
	}
	return msg, nil
}

func readCastFrame(conn net.Conn) (castMessage, error) {
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return castMessage{}, err
	}
	if length > 1<<20 {
		return castMessage{}, errors.New("cast frame too large")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return castMessage{}, err
	}
	return decodeCastMessage(buf)
}
