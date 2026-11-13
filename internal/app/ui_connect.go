package app

import (
	"sort"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/spotify"
)

type remoteConnectEntry struct {
	key       string
	sortName  string
	label     string
	clusterID string
	speaker   *spotify.CastSpeaker
}

func buildRemoteConnectEntries(cluster spotify.ConnectCluster, speakers []*spotify.CastSpeaker, selfID string) []remoteConnectEntry {
	matchedClusterIDs := make(map[string]bool)
	var entries []remoteConnectEntry

	for _, spk := range speakers {
		matchedID := matchCastClusterDevice(cluster.Devices, spk.SpotifyDeviceID, selfID, spk.Name, true)
		if matchedID != "" {
			matchedClusterIDs[matchedID] = true
		}
		label := spk.Name + " · " + spk.Model
		if matchedID != "" && matchedID == cluster.Active {
			label = "✓ " + label
		}
		entries = append(entries, remoteConnectEntry{
			key:       "cast-" + spk.ID,
			sortName:  spk.Name,
			label:     label,
			clusterID: matchedID,
			speaker:   spk,
		})
	}

	for id, device := range cluster.Devices {
		if matchedClusterIDs[id] || !isPlayableClusterDevice(id, selfID, device) {
			continue
		}
		name := device.Name
		if name == "" {
			name = l10n.T("Spotify 设备")
		}
		label := name
		if id == cluster.Active {
			label = "✓ " + label
		}
		entries = append(entries, remoteConnectEntry{
			key:       id,
			sortName:  name,
			label:     label,
			clusterID: id,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].sortName != entries[j].sortName {
			return entries[i].sortName < entries[j].sortName
		}
		return entries[i].key < entries[j].key
	})

	return entries
}

func renderConnectRow(c *ui.Context, a *app, key, label string, onClick func()) {
	row := ui.ButtonBase(c.Key(key)).Label(label).Padding(8, 6).Radius(6)
	if row.Hovered() {
		row.Background(a.pal.hover)
	}
	row.Children(func() { ui.Text(c, label).FontSize(13).SingleLine() })
	if row.Clicked() && onClick != nil {
		onClick()
	}
}

func renderLocalConnectRow(c *ui.Context, a *app, state *connectState, cluster spotify.ConnectCluster) {
	localName := l10n.T("此电脑 · ") + appName
	if cluster.Active == state.client.ID {
		localName = "✓ " + localName
	}
	renderConnectRow(c, a, state.client.ID, localName, func() {
		target := state.client.ID
		a.connectJob(func() error { return a.transferToDevice(state, target) })
		state.open = false
	})
}

func renderRemoteConnectRow(c *ui.Context, a *app, state *connectState, entry remoteConnectEntry) {
	renderConnectRow(c, a, entry.key, entry.label, func() {
		activateRemoteConnectEntry(a, state, entry)
	})
}

func activateRemoteConnectEntry(a *app, state *connectState, entry remoteConnectEntry) {
	if entry.speaker != nil {
		speaker := entry.speaker
		clusterID := entry.clusterID
		a.connectJob(func() error {
			if clusterID != "" {
				if err := a.transferToDevice(state, clusterID); err == nil {
					return nil
				}
			}
			return a.wakeAndTransferCast(state, speaker)
		})
	} else {
		target := entry.clusterID
		a.connectJob(func() error { return a.transferToDevice(state, target) })
	}
	state.open = false
}

// connectButton is the monitor speaker standing for the device the music
// plays on, which opens the picker of the devices in a panel below it; the
// popover, not the bar, is where the device playing names itself. The
// speaker wears the accent while the music plays somewhere other than here.
func (a *app) connectButton(c *ui.Context) {
	state := a.connect
	if state == nil {
		return
	}
	state.mu.RLock()
	cluster := state.cluster
	state.mu.RUnlock()
	color := a.pal.muted
	if state.isRemoteActive() {
		color = a.pal.accent
	}
	button := iconButton(c.Key("connect"), iconDevice, 16, color, l10n.T("播放设备"))
	if button.Clicked() {
		state.open = !state.open
		if state.open {
			a.speakersRegistry().Start()
		}
	}
	ui.PopoverBase(c.Key("connect-menu"), button, &state.open, func(panel ui.Element) {
		pop := popoverColors(a.pal.dark)
		panel.AttachTo(button, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 8, 0).Width(270).Padding(12).Radius(10).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2))
		panel.Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, l10n.T("选择播放设备")).FontSize(15).FontWeight(600)
				if state.err != "" {
					ui.Text(c, state.err).FontSize(12).TextColor(a.pal.muted)
				}
				if !state.receiver || !state.observer {
					ui.Text(c, l10n.T("正在连接 Spotify…")).FontSize(12)
					return
				}

				renderLocalConnectRow(c, a, state, cluster)
				entries := buildRemoteConnectEntries(cluster, a.speakersRegistry().Speakers(), state.client.ID)
				for _, entry := range entries {
					renderRemoteConnectRow(c, a, state, entry)
				}

				if len(entries) == 0 {
					ui.Text(c, l10n.T("在同账号的手机或网页打开 Spotify")).FontSize(12).TextColor(a.pal.muted)
				}
			})
		})
	})
}
