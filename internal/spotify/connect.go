package spotify

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type ConnectEvent struct {
	Kind string
	Data json.RawMessage
	Err  error
}

type Connect struct {
	ID     string
	Name   string
	ctx    context.Context
	cancel context.CancelFunc
	emit   func(ConnectEvent)
	mu     sync.Mutex
	wg     sync.WaitGroup
	seq    int64
	host   string
}

func NewConnect(id, name string, emit func(ConnectEvent)) *Connect {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connect{ID: id, Name: name, ctx: ctx, cancel: cancel, emit: emit, host: "spclient.wg.spotify.com"}
}

func (c *Connect) Start() {
	c.wg.Add(2)
	go func() { defer c.wg.Done(); c.loop(false) }()
	go func() { defer c.wg.Done(); c.loop(true) }()
}

func (c *Connect) Close() { c.cancel(); c.wg.Wait() }

func (c *Connect) notify(kind string, data json.RawMessage, err error) {
	if c.ctx.Err() == nil && c.emit != nil {
		c.emit(ConnectEvent{kind, data, err})
	}
}

func (c *Connect) request(ctx context.Context, method, path, connection string, body any) (json.RawMessage, error) {
	return c.requestOnce(ctx, method, path, connection, body, true)
}

func (c *Connect) requestOnce(ctx context.Context, method, path, connection string, body any, renew bool) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := CurrentIdentity()
	if err != nil {
		return nil, errors.New("Connect: 请重新登录")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, "https://"+c.host+path, bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("Connect: 请求地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+id.Token)
	req.Header.Set("client-token", id.ClientToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Origin", "https://open.spotify.com")
	req.Header.Set("Referer", "https://open.spotify.com/")
	req.Header.Set("app-platform", "WebPlayer")
	if connection != "" {
		req.Header.Set("X-Spotify-Connection-Id", connection)
	}
	res, err := Client.Do(req)
	if err != nil {
		return nil, errors.New("Connect: 网络请求失败")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized && renew {
		if _, err := RenewedIdentity(); err != nil {
			return nil, errors.New("Connect: 身份刷新失败，请重新登录")
		}
		return c.requestOnce(ctx, method, path, connection, body, false)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		turned := &ConnectError{Path: path, Status: res.StatusCode}
		var why struct {
			Type    string `json:"error_type"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &why) == nil {
			turned.Type, turned.Message = why.Type, why.Message
		}
		return nil, turned
	}
	return data, nil
}

func (c *Connect) loop(receiver bool) {
	delay := time.Second
	for c.ctx.Err() == nil {
		err := c.session(receiver)
		kind := "observer_offline"
		if receiver {
			kind = "receiver_offline"
		}
		c.notify(kind, nil, err)
		timer := time.NewTimer(delay)
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func (c *Connect) session(receiver bool) error {
	id, err := CurrentIdentity()
	if err != nil {
		return errors.New("Connect: 登录身份不可用")
	}
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	address := "wss://dealer.spotify.com/?access_token=" + url.QueryEscape(id.Token)
	conn, _, err := websocket.Dial(ctx, address, &websocket.DialOptions{HTTPClient: Client, HTTPHeader: http.Header{"User-Agent": {UserAgent}}})
	if err != nil {
		return errors.New("Connect: Dealer 连接失败")
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	go func() {
		timer := time.NewTicker(25 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(writeCtx, websocket.MessageText, []byte(`{"type":"ping"}`))
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	registered := false
	connection := ""
	defer func() {
		if !registered {
			return
		}
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if receiver {
			_, _ = c.request(cleanup, http.MethodDelete, "/track-playback/v1/devices/"+c.ID, "", map[string]any{"debug_source": "deregister"})
		} else {
			_, _ = c.request(cleanup, http.MethodDelete, "/connect-state/v1/devices/"+c.ID+"-observer", connection, nil)
		}
	}()
	for {
		readCtx, stop := context.WithTimeout(ctx, 60*time.Second)
		_, raw, err := conn.Read(readCtx)
		stop()
		if err != nil {
			return errors.New("Connect: Dealer 已断开")
		}
		var msg struct {
			Type     string            `json:"type"`
			URI      string            `json:"uri"`
			Headers  map[string]string `json:"headers"`
			Payloads []json.RawMessage `json:"payloads"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if msg.Type != "message" {
			continue
		}
		nextConnection := ""
		for key, value := range msg.Headers {
			if strings.EqualFold(key, "Spotify-Connection-Id") {
				nextConnection = value
			}
		}
		if strings.HasPrefix(msg.URI, "hm://pusher/v1/connections/") && nextConnection != "" {
			connection = nextConnection
			if receiver {
				err = c.registerReceiver(ctx, connection)
			} else {
				body := map[string]any{"member_type": "CONNECT_STATE", "device": map[string]any{"device_info": map[string]any{"capabilities": map[string]any{"can_be_player": false, "hidden": true, "needs_full_player_state": true}}}}
				var data json.RawMessage
				data, err = c.request(ctx, http.MethodPut, "/connect-state/v1/devices/"+c.ID+"-observer", connection, body)
				if err == nil {
					c.notify("cluster", data, nil)
				}
			}
			if err != nil {
				return err
			}
			registered = true
			kind := "observer_ready"
			if receiver {
				kind = "receiver_ready"
			}
			c.notify(kind, nil, nil)
			continue
		}
		for _, payload := range msg.Payloads {
			data, err := decodeConnectPayload(payload)
			if err != nil {
				continue
			}
			switch msg.URI {
			case "hm://connect-state/v1/cluster":
				if !receiver {
					c.notify("cluster", data, nil)
				}
			case "hm://track-playback/v1/command":
				if receiver {
					c.notify("command", data, nil)
				}
			}
		}
	}
}

func decodeConnectPayload(raw json.RawMessage) (json.RawMessage, error) {
	var encoded string
	if json.Unmarshal(raw, &encoded) != nil {
		return raw, nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		data, err = io.ReadAll(io.LimitReader(reader, (4<<20)+1))
		if err != nil {
			return nil, err
		}
	}
	if len(data) > 4<<20 || !json.Valid(data) {
		return nil, errors.New("invalid Connect payload")
	}
	return data, nil
}

func (c *Connect) registerReceiver(ctx context.Context, connection string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	body := map[string]any{
		"device": map[string]any{"brand": "spotify", "device_id": c.ID, "device_type": "computer", "model": "web_player", "name": c.Name, "metadata": map[string]any{}, "is_group": false, "platform_identifier": "web_player windows 10;chrome 141.0.0.0;desktop",
			"capabilities": map[string]any{"change_volume": true, "enable_play_token": true, "supports_file_media_type": true, "play_token_lost_behavior": "pause", "disable_connect": false, "audio_podcasts": true, "video_playback": false, "manifest_formats": []string{"file_ids_mp4", "file_ids_mp4_dual"}}},
		"outro_endcontent_snooping": false, "connection_id": connection, "client_version": "harmony:4.62.0", "volume": 52428,
	}
	raw, err := c.request(ctx, http.MethodPost, "/track-playback/v1/devices", "", body)
	if err != nil {
		return err
	}
	var result struct {
		Seq int64 `json:"initial_seq_num"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	c.seq = result.Seq
	return nil
}

func (c *Connect) Transfer(target string) error {
	_, err := c.request(c.ctx, http.MethodPost, "/connect-state/v1/connect/transfer/from/"+c.ID+"-observer/to/"+url.PathEscape(target), "", map[string]any{"transfer_options": map[string]string{"restore_paused": "restore"}})
	return err
}

func (c *Connect) Command(target, endpoint string, extra map[string]any) error {
	command := map[string]any{"endpoint": endpoint}
	for key, value := range extra {
		command[key] = value
	}
	_, err := c.request(c.ctx, http.MethodPost, "/connect-state/v1/player/command/from/"+c.ID+"-observer/to/"+url.PathEscape(target), "", map[string]any{"command": command})
	var turned *ConnectError
	if errors.As(err, &turned) {
		turned.Endpoint = endpoint
	}
	return err
}

// ConnectError is Spotify turning a Connect request down: the command, if it
// was one, and what Spotify said of why.
type ConnectError struct {
	Endpoint, Path string
	Status         int
	Type, Message  string // the error_type and message of its answer
}

func (e *ConnectError) Error() string {
	s := fmt.Sprintf("Connect: %s HTTP %d", e.Path, e.Status)
	if e.Endpoint != "" {
		s = fmt.Sprintf("Connect: %s %s HTTP %d", e.Endpoint, e.Path, e.Status)
	}
	if why := strings.TrimSpace(e.Type + " " + e.Message); why != "" {
		s += ": " + why
	}
	return s
}

func (c *Connect) Report(ref any, position, duration int64, paused bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	speed := 1
	if paused {
		speed = 0
	}
	body := map[string]any{"seq_num": c.seq, "state_ref": ref, "sub_state": map[string]any{"position": position, "duration": duration, "playback_speed": speed, "media_type": "AUDIO"}, "debug_source": "vinyl"}
	raw, err := c.request(c.ctx, http.MethodPut, "/track-playback/v1/devices/"+c.ID+"/state", "", body)
	if err == nil && len(raw) > 0 {
		c.notify("report", raw, nil)
	}
	return err
}

func (c *Connect) Reject(machine ConnectMachine, ref *ConnectRef, current any, position, duration int64, paused bool) error {
	if ref == nil || ref.Index < 0 || ref.Index >= len(machine.States) {
		return errors.New("Connect: 无效冲突状态")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	speed := 1
	if paused {
		speed = 0
	}
	body := map[string]any{"seq_nums": []int64{c.seq}, "state_ref": current, "sub_state": map[string]any{"position": position, "duration": duration, "playback_speed": speed, "media_type": "AUDIO"}, "rejected_state_refs": []any{map[string]any{"state_machine_id": machine.ID, "state_id": machine.States[ref.Index].ID, "paused": ref.Paused}}, "debug_source": "state_conflict"}
	raw, err := c.request(c.ctx, http.MethodPost, "/track-playback/v1/devices/"+c.ID+"/state_conflict", "", body)
	if err != nil {
		return err
	}
	var response struct {
		Commands []json.RawMessage `json:"commands"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &response) == nil {
		for _, command := range response.Commands {
			c.notify("command", command, nil)
		}
	}
	return nil
}

func (c *Connect) Volume(target string, volume float64) error {
	_, err := c.request(c.ctx, http.MethodPut, "/connect-state/v1/connect/volume/from/"+c.ID+"-observer/to/"+url.PathEscape(target), "", map[string]any{"volume": int(min(max(volume, 0), 1) * 65535)})
	return err
}
