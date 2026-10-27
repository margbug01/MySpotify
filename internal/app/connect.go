package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

type connectState struct {
	user                         string
	client                       *spotify.Connect
	cluster                      spotify.ConnectCluster
	machine                      spotify.ConnectMachine
	ref                          *spotify.ConnectRef
	observer, receiver, applying bool
	err                          string
	open                         bool
	known                        map[string]spotify.Track // what Decorate named, by URI
	decorating                   bool
	timer                        *time.Timer
	jobs                         chan func() error
	done                         chan struct{}
}

func (a *app) stopConnect() {
	state := a.connect
	a.connect = nil
	if state == nil {
		return
	}
	if state.timer != nil {
		state.timer.Stop()
	}
	close(state.done)
	go state.client.Close()
	a.pl.Control = nil
	if a.pl.IsRemote() {
		a.pl.StopConnect()
	}
}

func (a *app) startConnect() {
	if a.connect != nil && a.profile != nil && a.connect.user == a.profile.UserID {
		return
	}
	a.stopConnect()
	if a.profile == nil || shotHook != nil || a.quitting {
		return
	}
	hostname, _ := os.Hostname()
	sum := sha256.Sum256([]byte("Vinyl/" + a.profile.UserID + "/" + hostname))
	state := &connectState{user: a.profile.UserID, jobs: make(chan func() error, 32), done: make(chan struct{})}
	state.client = spotify.NewConnect(hex.EncodeToString(sum[:20]), "Vinyl · "+hostname, func(event spotify.ConnectEvent) {
		a.post(func() {
			if a.connect == state {
				a.connectEvent(event)
			}
		})
	})
	a.connect = state
	a.pl.Control = a.connectControl
	go func() {
		for {
			select {
			case <-state.done:
				return
			case job := <-state.jobs:
				select {
				case <-state.done:
					return
				default:
				}
				if err := job(); err != nil {
					a.post(func() {
						if a.connect == state {
							state.err = err.Error()
							a.toast = state.err
						}
					})
				}
			}
		}
	}()
	state.client.Start()
	a.connectTick(state)
}

func (a *app) connectJob(job func() error) {
	if a.connect == nil {
		return
	}
	select {
	case a.connect.jobs <- job:
	default:
		a.toast = "Connect 操作正在排队，请稍后再试"
	}
}

func (a *app) connectTick(state *connectState) {
	state.timer = time.AfterFunc(5*time.Second, func() {
		a.post(func() {
			if a.connect != state || a.quitting {
				return
			}
			a.reportConnect()
			a.connectTick(state)
		})
	})
}

func (a *app) reportConnect() {
	state := a.connect
	if state == nil || state.applying || !state.receiver || state.ref == nil || a.pl.IsRemote() || a.pl.Loading() {
		return
	}
	index := state.ref.Index
	if index < 0 || index >= len(state.machine.States) {
		return
	}
	paused := !a.pl.Playing()
	state.ref.Paused = paused
	ref := map[string]any{"state_machine_id": state.machine.ID, "state_id": state.machine.States[index].ID, "paused": paused}
	pos, duration := a.pl.Position().Milliseconds(), a.pl.Duration().Milliseconds()
	a.connectJob(func() error { return state.client.Report(ref, pos, duration, paused) })
}

func (a *app) connectEvent(event spotify.ConnectEvent) {
	state := a.connect
	if event.Err != nil {
		state.err = event.Err.Error()
		log.Printf("connect: %s: %v", event.Kind, event.Err)
	}
	switch event.Kind {
	case "observer_ready":
		state.observer = true
		state.err = ""
	case "receiver_ready":
		state.receiver = true
		state.err = ""
	case "observer_offline":
		state.observer = false
	case "receiver_offline":
		state.receiver = false
		if state.ref != nil {
			state.ref = nil
			a.pl.StopConnect()
		}
	case "cluster":
		cluster, err := spotify.ParseConnectCluster(event.Data)
		if err != nil {
			return
		}
		if cluster.Timestamp > 0 && cluster.Timestamp < state.cluster.Timestamp {
			return
		}
		state.cluster = cluster
		a.pl.ConnectOptions(cluster.Player.Options)
		if cluster.Active != "" && cluster.Active != state.client.ID {
			state.ref = nil
			track := cluster.Track()
			if track.ID != "" {
				position := int64(cluster.Player.Position)
				if cluster.Player.Playing && !cluster.Player.Paused && cluster.Timestamp > cluster.Player.Timestamp && cluster.Player.Timestamp > 0 {
					position += int64(cluster.Timestamp - cluster.Player.Timestamp)
				}
				state.applying = true
				a.pl.MirrorConnect(track, time.Duration(max(0, position))*time.Millisecond, cluster.Player.Playing && !cluster.Player.Paused)
				state.applying = false
				a.showConnectQueue(cluster.Queue())
			} else {
				a.pl.StopConnect()
			}
		} else if cluster.Active == state.client.ID && state.ref != nil {
			a.connectQueue(state.ref.Index)
		}
	case "report":
		if state.ref == nil {
			return
		}
		var response struct {
			Machine spotify.ConnectMachine `json:"state_machine"`
			Updated *spotify.ConnectRef    `json:"updated_state_ref"`
		}
		if json.Unmarshal(event.Data, &response) != nil || response.Machine.ID == "" {
			return
		}
		currentID := state.machine.States[state.ref.Index].ID
		for index, next := range response.Machine.States {
			if response.Updated != nil && response.Updated.Index == index || response.Updated == nil && next.ID == currentID {
				track, err := response.Machine.Track(index)
				if err == nil && a.pl.Current() != nil && track.URI() == a.pl.Current().URI() {
					state.machine = response.Machine
					state.ref.Index = index
					a.connectQueue(index)
				}
				break
			}
		}
	case "command":
		var command spotify.ConnectCommand
		if json.Unmarshal(event.Data, &command) != nil {
			return
		}
		switch command.Type {
		case "replace_state":
			if command.Ref == nil {
				state.ref = nil
				if !a.pl.IsRemote() {
					a.pl.StopConnect()
				}
				a.connectJob(func() error { return state.client.Report(nil, 0, 0, true) })
				return
			}
			if len(command.Previous) > 0 && string(command.Previous) != "null" && state.ref != nil {
				var previous struct {
					Machine string `json:"state_machine_id"`
					State   string `json:"state_id"`
					Paused  bool   `json:"paused"`
				}
				if json.Unmarshal(command.Previous, &previous) == nil && (previous.Machine != state.machine.ID || previous.State != state.machine.States[state.ref.Index].ID || previous.Paused != state.ref.Paused) {
					state.err = "Connect 状态冲突，正在重新同步"
					current := map[string]any{"state_machine_id": state.machine.ID, "state_id": state.machine.States[state.ref.Index].ID, "paused": state.ref.Paused}
					position, duration, paused := a.pl.Position().Milliseconds(), a.pl.Duration().Milliseconds(), !a.pl.Playing()
					a.connectJob(func() error {
						return state.client.Reject(command.Machine, command.Ref, current, position, duration, paused)
					})
					return
				}
			}
			state.machine = command.Machine
			a.applyConnect(command.Ref, command.Seek)
		case "set_volume":
			if command.Volume != nil {
				state.applying = true
				a.volume = float64(min(max(*command.Volume, 0), 65535)) / 65535
				a.pl.SetVolume(a.volume)
				state.applying = false
			}
		case "log_out":
			state.ref = nil
			a.pl.StopConnect()
		case "ping":
			a.reportConnect()
		}
	}
}

func (a *app) applyConnect(ref *spotify.ConnectRef, seek *int64) {
	state := a.connect
	track, err := state.machine.Track(ref.Index)
	if err != nil {
		state.err = err.Error()
		a.toast = state.err
		state.ref = nil
		a.pl.StopConnect()
		return
	}
	copyRef := *ref
	state.ref = &copyRef
	state.cluster.Active = state.client.ID
	position := time.Duration(-1)
	if seek != nil {
		position = time.Duration(max(0, *seek)) * time.Millisecond
	}
	state.applying = true
	a.pl.ConnectOptions(state.machine.Attributes.Options)
	a.pl.ApplyConnect(track, position, ref.Paused)
	a.connectQueue(ref.Index)
	state.applying = false
	a.reportConnect()
}

// connectQueue shows what follows the machine's state index as Spotify's
// apps do, from the cluster: the machine holds only the track and the one
// after. Until the cluster has come to the track, the machine's two.
func (a *app) connectQueue(index int) {
	state := a.connect
	track, err := state.machine.Track(index)
	if err != nil {
		return
	}
	queue := state.cluster.Queue()
	for i := range queue {
		if queue[i].URI() == track.URI() {
			track.UID = queue[i].UID
			// The machine names some tracks' artists only by URI; the
			// cluster may name them.
			if track.Unnamed() && !queue[i].Unnamed() {
				track.Artists = queue[i].Artists
			}
			queue[i] = track
			a.showConnectQueue(queue[i:])
			return
		}
	}
	a.pl.ConnectQueue(state.machine.Queue(index))
}

// showConnectQueue shows queue, naming the tracks the cluster gives only
// the URIs of: from what Spotify said before, or once it says.
func (a *app) showConnectQueue(queue []spotify.Track) {
	state := a.connect
	missing := false
	for i, t := range queue {
		if !t.Unnamed() {
			continue
		}
		if known, ok := state.known[t.URI()]; ok {
			known.UID = t.UID
			queue[i] = known
		} else {
			missing = true
		}
	}
	a.pl.ConnectQueue(queue)
	if !missing || state.decorating {
		return
	}
	state.decorating = true
	go func() {
		named, err := spotify.Decorate(queue)
		a.post(func() {
			if a.connect != state {
				return
			}
			state.decorating = false
			if err != nil {
				log.Printf("connect: queue: %v", err)
				return
			}
			if state.known == nil || len(state.known) > 2000 {
				state.known = map[string]spotify.Track{}
			}
			// Those Spotify does not name either are kept as they are, not
			// asked of again.
			for i, t := range queue {
				if t.Unnamed() {
					state.known[t.URI()] = named[i]
				}
			}
			if state.ref != nil || a.pl.IsRemote() {
				a.showConnectQueue(append([]spotify.Track(nil), a.pl.Queue()...))
			}
		})
	}()
}

func (a *app) connectControl(action string, value any) bool {
	state := a.connect
	if state == nil || state.applying {
		return false
	}
	if !state.observer || !state.receiver {
		if a.pl.IsRemote() || state.ref != nil {
			a.toast = "Connect 正在重连，请稍后"
			return true
		}
		return false
	}
	target := state.cluster.Active
	if target == "" {
		target = state.client.ID
	}
	extra := map[string]any{}
	if action == "toggle" && target == state.client.ID && state.ref == nil && a.pl.Current() != nil {
		extra = connectPlayBody(player.QueueRequest{Tracks: append([]spotify.Track(nil), a.pl.Queue()...), Start: 0})
		order, at := a.pl.PlayOrder()
		if at >= 0 && at < len(order) {
			extra["options"].(map[string]any)["skip_to"] = map[string]any{"track_index": order[at]}
		}
		extra["options"].(map[string]any)["seek_to"] = a.pl.Position().Milliseconds()
		extra["options"].(map[string]any)["initially_paused"] = a.pl.Playing()
		a.connectJob(func() error { return state.client.Command(target, "play", extra) })
		return true
	}
	switch action {
	case "play":
		extra = connectPlayBody(value.(player.QueueRequest))
	case "toggle":
		if a.pl.Playing() {
			action = "pause"
		} else {
			action = "resume"
		}
	case "seek":
		if state.ref != nil && state.machine.States[state.ref.Index].NoSeek {
			a.toast = "当前内容不支持拖动进度"
			return true
		}
		action = "seek_to"
		extra["value"] = max(0, value.(time.Duration).Milliseconds())
	case "advance":
		if state.ref == nil {
			return false
		}
		next := state.machine.States[state.ref.Index].Transitions.Advance
		if next == nil {
			a.pl.StopConnect()
			return true
		}
		zero := int64(0)
		a.applyConnect(next, &zero)
		return true
	case "shuffle":
		action = "set_shuffling_context"
		extra["value"] = value.(bool)
	case "repeat":
		mode := value.(player.RepeatMode)
		action = "set_options"
		extra["repeating_context"] = mode == player.RepeatAll
		extra["repeating_track"] = mode == player.RepeatOne
	case "volume":
		volume := value.(float64)
		a.connectJob(func() error { return state.client.Volume(target, volume) })
		return true
	case "jump":
		// A track of the queue Spotify gave is skipped to where it is, as
		// Spotify's apps do, rather than played as a queue of its own.
		queue, i := a.pl.Queue(), value.(int)
		if state.ref == nil && !a.pl.IsRemote() || queue[i].UID == "" {
			return false
		}
		track := map[string]string{"uri": queue[i].URI(), "uid": queue[i].UID}
		request := player.QueueRequest{Tracks: append([]spotify.Track(nil), queue...), Start: i}
		a.connectJob(func() error {
			err := state.client.Command(target, "skip_next", map[string]any{"track": track})
			if err != nil {
				err = state.client.Command(target, "play", connectPlayBody(request))
			}
			return err
		})
		return true
	case "skip_next", "skip_prev":
	default:
		return false
	}
	a.connectJob(func() error { return state.client.Command(target, action, extra) })
	return true
}

func connectPlayBody(request player.QueueRequest) map[string]any {
	tracks := make([]map[string]string, 0, len(request.Tracks))
	for _, track := range request.Tracks {
		tracks = append(tracks, map[string]string{"uri": track.URI()})
	}
	return map[string]any{"context": map[string]any{"uri": "", "url": "", "metadata": map[string]any{}, "pages": []any{map[string]any{"tracks": tracks}}}, "play_origin": map[string]any{}, "options": map[string]any{"license": "on-demand", "skip_to": map[string]any{"track_index": request.Start}}}
}

func (a *app) connectButton(c *ui.Context) {
	state := a.connect
	if state == nil {
		return
	}
	label := "设备"
	if state.cluster.Active != "" && state.cluster.Active != state.client.ID {
		if device, ok := state.cluster.Devices[state.cluster.Active]; ok {
			label = device.Name
		}
	}
	button := ui.ButtonBase(c.Key("connect")).Label("播放设备").MaxWidth(140).Padding(6, 8).Radius(6)
	if button.Hovered() {
		button.Background(a.pal.hover)
	}
	button.Children(func() { ui.Text(c, label).FontSize(12).SingleLine().TextColor(a.pal.muted) })
	if button.Clicked() {
		state.open = !state.open
	}
	ui.PopoverBase(c.Key("connect-menu"), button, &state.open, func(panel ui.Element) {
		pop := popoverColors(a.pal.dark)
		panel.AttachTo(button, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 8, 0).Width(270).Padding(12).Radius(10).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2))
		panel.Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, "选择播放设备").FontSize(15).FontWeight(600)
				if state.err != "" {
					ui.Text(c, state.err).FontSize(12).TextColor(a.pal.muted)
				}
				if !state.receiver || !state.observer {
					ui.Text(c, "正在连接 Spotify…").FontSize(12)
					return
				}
				ids := []string{state.client.ID}
				for id, device := range state.cluster.Devices {
					if id != state.client.ID && !device.Capabilities.Hidden && (device.Capabilities.CanPlay == nil || *device.Capabilities.CanPlay) {
						ids = append(ids, id)
					}
				}
				sort.Strings(ids[1:])
				for _, id := range ids {
					name := state.cluster.Devices[id].Name
					if id == state.client.ID {
						name = "此电脑 · Vinyl"
					}
					if name == "" {
						name = "Spotify 设备"
					}
					if id == state.cluster.Active {
						name = "✓ " + name
					}
					row := ui.ButtonBase(c.Key(id)).Label(name).Padding(8, 6).Radius(6)
					if row.Hovered() {
						row.Background(a.pal.hover)
					}
					row.Children(func() { ui.Text(c, name).FontSize(13).SingleLine() })
					if row.Clicked() {
						target := id
						a.connectJob(func() error { return state.client.Transfer(target) })
						state.open = false
					}
				}
				if len(ids) == 1 {
					ui.Text(c, "在同账号的手机或网页打开 Spotify").FontSize(12).TextColor(a.pal.muted)
				}
			})
		})
	})
}
