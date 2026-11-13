package spotify

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SpeakerRegistry manages mDNS discovery, TTL expiration, and subscriber notifications for Google Cast speakers.
type SpeakerRegistry struct {
	mu          sync.RWMutex
	speakers    map[string]*CastSpeaker
	instances   map[string]string
	started     bool
	stopCh      chan struct{}
	doneCh      chan struct{}
	activeConn  net.PacketConn
	wg          sync.WaitGroup
	nextSubID   int
	subscribers map[int]func()

	now                func() time.Time
	queryInterval      time.Duration
	cleanInterval      time.Duration
	readTimeout        time.Duration
	readBackoffMin     time.Duration
	readBackoffMax     time.Duration
	speakerTTL         time.Duration
	listenPacket       func(lc *net.ListenConfig, ctx context.Context, network, address string) (net.PacketConn, error)
	listenMulticastUDP func(network string, ifi *net.Interface, gaddr *net.UDPAddr) (*net.UDPConn, error)
	listenUDP          func(network string, laddr *net.UDPAddr) (*net.UDPConn, error)
}

// SpeakerRegistryOption configures a SpeakerRegistry instance.
type SpeakerRegistryOption func(*SpeakerRegistry)

// WithNoopDiscovery disables real mDNS socket binding so tests never open UDP 5353 sockets.
func WithNoopDiscovery() SpeakerRegistryOption {
	errDisabled := errors.New("cast discovery disabled")
	return func(r *SpeakerRegistry) {
		r.listenPacket = func(*net.ListenConfig, context.Context, string, string) (net.PacketConn, error) {
			return nil, errDisabled
		}
		r.listenMulticastUDP = func(string, *net.Interface, *net.UDPAddr) (*net.UDPConn, error) {
			return nil, errDisabled
		}
		r.listenUDP = func(string, *net.UDPAddr) (*net.UDPConn, error) {
			return nil, errDisabled
		}
	}
}

// WithListenPacket overrides the primary PacketConn listener used by mDNS discovery.
func WithListenPacket(fn func(lc *net.ListenConfig, ctx context.Context, network, address string) (net.PacketConn, error)) SpeakerRegistryOption {
	return func(r *SpeakerRegistry) {
		if fn != nil {
			r.listenPacket = fn
		}
	}
}

func withRegistryNow(fn func() time.Time) SpeakerRegistryOption {
	return func(r *SpeakerRegistry) {
		if fn != nil {
			r.now = fn
		}
	}
}

func withRegistryIntervals(queryInterval, cleanInterval, readTimeout time.Duration) SpeakerRegistryOption {
	return func(r *SpeakerRegistry) {
		if queryInterval > 0 {
			r.queryInterval = queryInterval
		}
		if cleanInterval > 0 {
			r.cleanInterval = cleanInterval
		}
		if readTimeout > 0 {
			r.readTimeout = readTimeout
		}
	}
}

func withRegistryReadBackoff(min, max time.Duration) SpeakerRegistryOption {
	return func(r *SpeakerRegistry) {
		r.readBackoffMin = min
		r.readBackoffMax = max
	}
}

func withRegistryListeners(
	listenPacket func(lc *net.ListenConfig, ctx context.Context, network, address string) (net.PacketConn, error),
	listenMulticastUDP func(network string, ifi *net.Interface, gaddr *net.UDPAddr) (*net.UDPConn, error),
	listenUDP func(network string, laddr *net.UDPAddr) (*net.UDPConn, error),
) SpeakerRegistryOption {
	return func(r *SpeakerRegistry) {
		if listenPacket != nil {
			r.listenPacket = listenPacket
		}
		if listenMulticastUDP != nil {
			r.listenMulticastUDP = listenMulticastUDP
		}
		if listenUDP != nil {
			r.listenUDP = listenUDP
		}
	}
}

// NewSpeakerRegistry constructs a SpeakerRegistry with default mDNS intervals and listeners.
func NewSpeakerRegistry(opts ...SpeakerRegistryOption) *SpeakerRegistry {
	r := &SpeakerRegistry{
		speakers:       make(map[string]*CastSpeaker),
		instances:      make(map[string]string),
		subscribers:    make(map[int]func()),
		now:            time.Now,
		queryInterval:  15 * time.Second,
		cleanInterval:  15 * time.Second,
		readTimeout:    45 * time.Second,
		readBackoffMin: 50 * time.Millisecond,
		readBackoffMax: 5 * time.Second,
		speakerTTL:     castSpeakerTTL,
		listenPacket: func(lc *net.ListenConfig, ctx context.Context, network, address string) (net.PacketConn, error) {
			return lc.ListenPacket(ctx, network, address)
		},
		listenMulticastUDP: net.ListenMulticastUDP,
		listenUDP:          net.ListenUDP,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// Subscribe registers a callback invoked when discovered speakers change and returns an idempotent unsubscribe function.
func (r *SpeakerRegistry) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	r.mu.Lock()
	r.nextSubID++
	id := r.nextSubID
	if r.subscribers == nil {
		r.subscribers = make(map[int]func())
	}
	r.subscribers[id] = fn
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.subscribers, id)
			r.mu.Unlock()
		})
	}
}

func (r *SpeakerRegistry) notifyChange() {
	r.mu.RLock()
	fns := make([]func(), 0, len(r.subscribers))
	for _, fn := range r.subscribers {
		fns = append(fns, fn)
	}
	r.mu.RUnlock()
	for _, fn := range fns {
		fn()
	}
}

// Speakers returns a sorted snapshot (by Name, then ID) of non-expired Cast speakers.
func (r *SpeakerRegistry) Speakers() []*CastSpeaker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.now()
	list := make([]*CastSpeaker, 0, len(r.speakers))
	for _, s := range r.speakers {
		if now.Before(s.ExpiresAt) {
			clone := *s
			list = append(list, &clone)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}
		return list[i].ID < list[j].ID
	})
	return list
}

// Upsert inserts or updates a Cast speaker in the registry and notifies subscribers.
func (r *SpeakerRegistry) Upsert(spk *CastSpeaker) {
	if spk == nil || spk.ID == "" {
		return
	}
	clone := *spk
	if clone.ExpiresAt.IsZero() {
		clone.ExpiresAt = r.now().Add(r.speakerTTL)
	}
	r.mu.Lock()
	if r.speakers == nil {
		r.speakers = make(map[string]*CastSpeaker)
	}
	r.speakers[clone.ID] = &clone
	r.mu.Unlock()
	r.notifyChange()
}

// Seed replaces the cached speakers with the provided list (useful in tests without opening sockets).
func (r *SpeakerRegistry) Seed(speakers ...*CastSpeaker) {
	r.mu.Lock()
	r.speakers = make(map[string]*CastSpeaker, len(speakers))
	r.instances = make(map[string]string)
	now := r.now()
	for _, s := range speakers {
		if s == nil || s.ID == "" {
			continue
		}
		clone := *s
		if clone.ExpiresAt.IsZero() {
			clone.ExpiresAt = now.Add(r.speakerTTL)
		}
		r.speakers[clone.ID] = &clone
	}
	r.mu.Unlock()
}

// SetSpotifyDeviceID records the resolved Spotify Connect device ID on a cached speaker if present.
func (r *SpeakerRegistry) SetSpotifyDeviceID(speakerID, spotifyDeviceID string) {
	if speakerID == "" || spotifyDeviceID == "" {
		return
	}
	r.mu.Lock()
	if existing, ok := r.speakers[speakerID]; ok {
		existing.SpotifyDeviceID = spotifyDeviceID
	}
	r.mu.Unlock()
}

// Start begins background mDNS multicast listening and periodic scanning. It is idempotent while running.
func (r *SpeakerRegistry) Start() {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	r.stopCh = stopCh
	r.doneCh = doneCh
	r.wg.Add(1)
	r.mu.Unlock()

	go r.discoveryLoop(stopCh, doneCh)
}

// Stop closes the active mDNS listener, stops discovery/cleanup goroutines, and waits for them to exit.
func (r *SpeakerRegistry) Stop() {
	r.mu.Lock()
	stopCh := r.stopCh
	doneCh := r.doneCh
	conn := r.activeConn
	if stopCh != nil {
		r.started = false
		r.stopCh = nil
		r.doneCh = nil
		r.activeConn = nil
		close(stopCh)
	}
	r.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if doneCh != nil {
		<-doneCh
	}
	r.wg.Wait()
}

func (r *SpeakerRegistry) listenMDNSPacketConn() (net.PacketConn, error) {
	lc := &net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			if err := c.Control(func(fd uintptr) {
				opErr = setMDNSSocketOptions(fd)
			}); err != nil {
				return err
			}
			return opErr
		},
	}

	needMulticastJoin := true
	packetConn, err := r.listenPacket(lc, context.Background(), "udp4", "0.0.0.0:5353")
	if err != nil {
		addr, resolveErr := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
		if resolveErr != nil {
			err = resolveErr
		} else {
			packetConn, err = r.listenMulticastUDP("udp4", nil, addr)
			if err == nil {
				needMulticastJoin = false
			}
		}
		if err != nil {
			packetConn, err = r.listenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
			if err != nil {
				return nil, err
			}
		}
	}

	if needMulticastJoin {
		if udpConn, ok := packetConn.(*net.UDPConn); ok {
			if rawConn, scErr := udpConn.SyscallConn(); scErr == nil {
				var joinErr error
				if ctrlErr := rawConn.Control(func(fd uintptr) {
					joinErr = joinMDNSMulticast(fd)
				}); ctrlErr != nil {
					log.Printf("spotify: cast mDNS socket control: %v", ctrlErr)
				} else if joinErr != nil {
					log.Printf("spotify: cast mDNS multicast join: %v", joinErr)
				}
			} else if !errors.Is(scErr, syscall.EINVAL) {
				log.Printf("spotify: cast mDNS syscall conn: %v", scErr)
			}
		}
	}
	return packetConn, nil
}

func (r *SpeakerRegistry) discoveryLoop(stopCh, doneCh chan struct{}) {
	defer func() {
		r.mu.Lock()
		if r.stopCh == stopCh {
			r.started = false
			r.stopCh = nil
			r.doneCh = nil
		}
		r.activeConn = nil
		r.mu.Unlock()
		close(doneCh)
		r.wg.Done()
	}()

	packetConn, err := r.listenMDNSPacketConn()
	if err != nil {
		return
	}

	r.mu.Lock()
	stopped := r.stopCh != stopCh
	if !stopped {
		r.activeConn = packetConn
	}
	r.mu.Unlock()
	if stopped {
		_ = packetConn.Close()
		return
	}
	defer func() {
		_ = packetConn.Close()
	}()

	r.runCastDiscovery(packetConn, r.queryInterval, r.cleanInterval, r.readTimeout, stopCh)
}

func (r *SpeakerRegistry) runCastDiscovery(packetConn net.PacketConn, queryInterval, cleanInterval, readTimeout time.Duration, stop <-chan struct{}) {
	if queryInterval <= 0 {
		queryInterval = 15 * time.Second
	}
	if cleanInterval <= 0 {
		cleanInterval = 15 * time.Second
	}
	if readTimeout <= 0 {
		readTimeout = 45 * time.Second
	}

	destAddr := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}
	query := buildPTRQuery(castMDNSService)
	done := make(chan struct{})
	var loopWG sync.WaitGroup
	loopWG.Add(1)
	defer loopWG.Wait()
	defer close(done)

	go func() {
		defer loopWG.Done()
		queryTicker := time.NewTicker(queryInterval)
		cleanTicker := time.NewTicker(cleanInterval)
		defer queryTicker.Stop()
		defer cleanTicker.Stop()
		for {
			select {
			case <-done:
				return
			case <-stop:
				return
			case <-queryTicker.C:
				if _, err := packetConn.WriteTo(query, destAddr); err != nil && !errors.Is(err, net.ErrClosed) {
					log.Printf("spotify: cast mDNS query write: %v", err)
				}
			case <-cleanTicker.C:
				r.cleanExpired()
			}
		}
	}()

	if _, err := packetConn.WriteTo(query, destAddr); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Printf("spotify: cast mDNS initial query write: %v", err)
	}

	buf := make([]byte, 4096)
	var backoff time.Duration
	loggedDeadlineErr := false
	for {
		select {
		case <-stop:
			return
		default:
		}
		if err := packetConn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			if !loggedDeadlineErr && !errors.Is(err, net.ErrClosed) {
				log.Printf("spotify: cast mDNS set read deadline: %v", err)
				loggedDeadlineErr = true
			}
		} else {
			loggedDeadlineErr = false
		}
		n, src, err := packetConn.ReadFrom(buf)
		if err != nil {
			select {
			case <-stop:
				return
			default:
			}
			r.cleanExpired()
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				backoff = 0
				continue
			}
			if backoff == 0 {
				backoff = r.readBackoffMin
			} else {
				backoff *= 2
			}
			if backoff > r.readBackoffMax {
				backoff = r.readBackoffMax
			}
			if backoff > 0 {
				timer := time.NewTimer(backoff)
				select {
				case <-stop:
					timer.Stop()
					return
				case <-done:
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			continue
		}

		backoff = 0
		r.parseMDNSResponse(buf[:n], src)
	}
}

func (r *SpeakerRegistry) cleanExpired() {
	r.mu.Lock()
	now := r.now()
	changed := false
	for id, s := range r.speakers {
		if !now.Before(s.ExpiresAt) {
			if r.deleteSpeakerLocked(id) {
				changed = true
			}
		}
	}
	r.mu.Unlock()
	if changed {
		r.notifyChange()
	}
}

func (r *SpeakerRegistry) deleteSpeakerLocked(id string) bool {
	if _, ok := r.speakers[id]; !ok {
		return false
	}
	delete(r.speakers, id)
	for inst, spkID := range r.instances {
		if spkID == id {
			delete(r.instances, inst)
		}
	}
	return true
}

func (r *SpeakerRegistry) resolveSpeakerIDLocked(rec mdnsServiceRecord, ipStr string) string {
	if rec.id != "" {
		return rec.id
	}
	if rec.instance != "" && rec.instance != "_default" {
		if id, ok := r.instances[strings.ToLower(rec.instance)]; ok {
			return id
		}
	}
	if rec.goodbye && ipStr != "" && rec.port > 0 && strings.HasSuffix(strings.ToLower(rec.instance), "."+castMDNSService) {
		for id, s := range r.speakers {
			if s.IP == ipStr && s.Port == rec.port {
				return id
			}
		}
	}
	return ""
}

func (r *SpeakerRegistry) parseMDNSResponse(data []byte, src net.Addr) {
	if len(data) < 12 {
		return
	}

	ipStr := ""
	if udpAddr, ok := src.(*net.UDPAddr); ok && udpAddr.IP != nil {
		ipStr = udpAddr.IP.String()
	}

	records, dnsIP := extractMDNSRecords(data)
	if ipStr == "" && dnsIP != "" {
		ipStr = dnsIP
	}
	if len(records) == 0 {
		return
	}

	changed := false
	r.mu.Lock()
	now := r.now()
	for _, rec := range records {
		if rec.goodbye {
			if targetID := r.resolveSpeakerIDLocked(rec, ipStr); targetID != "" {
				if r.deleteSpeakerLocked(targetID) {
					changed = true
				}
			}
			continue
		}

		targetID := r.resolveSpeakerIDLocked(rec, ipStr)
		if targetID == "" {
			continue
		}
		existing, found := r.speakers[targetID]
		if !found {
			if rec.id == "" || rec.name == "" {
				continue
			}
			model := rec.model
			if model == "" {
				model = "Google Cast Speaker"
			}
			port := rec.port
			if port <= 0 {
				port = 8009
			}
			r.speakers[rec.id] = &CastSpeaker{
				ID:        rec.id,
				Name:      rec.name,
				Model:     model,
				IP:        ipStr,
				Port:      port,
				ExpiresAt: now.Add(r.speakerTTL),
			}
			if rec.instance != "" && rec.instance != "_default" {
				r.instances[strings.ToLower(rec.instance)] = rec.id
			}
			changed = true
			continue
		}

		name := existing.Name
		if rec.name != "" {
			name = rec.name
		}
		model := existing.Model
		if rec.model != "" {
			model = rec.model
		}
		ip := existing.IP
		if ipStr != "" {
			ip = ipStr
		}
		port := existing.Port
		if rec.port > 0 {
			port = rec.port
		} else if port <= 0 {
			port = 8009
		}

		if existing.Name != name || existing.IP != ip || existing.Port != port || existing.Model != model {
			changed = true
		}
		existing.Name = name
		existing.Model = model
		existing.IP = ip
		existing.Port = port
		existing.ExpiresAt = now.Add(r.speakerTTL)
		if rec.instance != "" && rec.instance != "_default" {
			r.instances[strings.ToLower(rec.instance)] = targetID
		}
	}
	r.mu.Unlock()
	if changed {
		r.notifyChange()
	}
}

func buildPTRQuery(service string) []byte {
	buf := new(bytes.Buffer)
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	for _, part := range strings.Split(service, ".") {
		buf.WriteByte(byte(len(part)))
		buf.WriteString(part)
	}
	buf.WriteByte(0x00)
	binary.Write(buf, binary.BigEndian, uint16(12)) // PTR = 12
	binary.Write(buf, binary.BigEndian, uint16(1))  // IN = 1
	return buf.Bytes()
}

type mdnsServiceRecord struct {
	instance string
	id       string
	name     string
	model    string
	port     int
	ttl      uint32
	hasTTL   bool
	goodbye  bool
}

func (r *mdnsServiceRecord) recordTTL(ttl uint32) {
	if ttl == 0 {
		r.ttl = 0
		r.hasTTL = true
		r.goodbye = true
		return
	}
	if r.goodbye {
		return
	}
	if !r.hasTTL || ttl < r.ttl {
		r.ttl = ttl
		r.hasTTL = true
	}
}

func readDNSName(pkt []byte, off int) (string, int, bool) {
	var parts []string
	jumped := false
	nextOff := off
	for i := 0; i < 64 && off < len(pkt); i++ {
		l := int(pkt[off])
		if l == 0 {
			if !jumped {
				nextOff = off + 1
			}
			return strings.Join(parts, "."), nextOff, true
		}
		if l&0xC0 == 0xC0 {
			if off+1 >= len(pkt) {
				return "", len(pkt), false
			}
			ptr := ((l & 0x3F) << 8) | int(pkt[off+1])
			if !jumped {
				nextOff = off + 2
				jumped = true
			}
			off = ptr
			continue
		}
		off++
		if off+l > len(pkt) {
			return "", len(pkt), false
		}
		parts = append(parts, string(pkt[off:off+l]))
		off += l
	}
	return "", len(pkt), false
}

func extractMDNSRecords(pkt []byte) ([]mdnsServiceRecord, string) {
	if len(pkt) < 12 {
		return nil, ""
	}
	qdCount := int(binary.BigEndian.Uint16(pkt[4:6]))
	anCount := int(binary.BigEndian.Uint16(pkt[6:8]))
	nsCount := int(binary.BigEndian.Uint16(pkt[8:10]))
	arCount := int(binary.BigEndian.Uint16(pkt[10:12]))
	totalRR := anCount + nsCount + arCount
	if totalRR <= 0 || totalRR > 64 {
		return nil, ""
	}

	off := 12
	for i := 0; i < qdCount && off < len(pkt); i++ {
		var ok bool
		_, off, ok = readDNSName(pkt, off)
		if !ok || off+4 > len(pkt) {
			return nil, ""
		}
		off += 4
	}

	byOwner := make(map[string]*mdnsServiceRecord)
	var order []string
	getRec := func(owner string) *mdnsServiceRecord {
		if owner == "" {
			owner = "_default"
		}
		key := strings.ToLower(owner)
		rec, ok := byOwner[key]
		if !ok {
			rec = &mdnsServiceRecord{instance: owner}
			byOwner[key] = rec
			order = append(order, key)
		}
		return rec
	}

	var ipv4 string
	for i := 0; i < totalRR && off < len(pkt); i++ {
		owner, nextOff, ok := readDNSName(pkt, off)
		if !ok || nextOff+10 > len(pkt) {
			break
		}
		off = nextOff
		rrType := binary.BigEndian.Uint16(pkt[off : off+2])
		rrTTL := binary.BigEndian.Uint32(pkt[off+4 : off+8])
		rdLen := int(binary.BigEndian.Uint16(pkt[off+8 : off+10]))
		rdataStart := off + 10
		if rdataStart+rdLen > len(pkt) {
			break
		}
		rdata := pkt[rdataStart : rdataStart+rdLen]
		off = rdataStart + rdLen

		switch rrType {
		case 1: // A
			if rdLen == 4 && ipv4 == "" {
				ipv4 = net.IP(rdata).String()
			}
		case 12: // PTR
			if strings.EqualFold(owner, castMDNSService) {
				if target, _, ok := readDNSName(pkt, rdataStart); ok && target != "" {
					rec := getRec(target)
					rec.recordTTL(rrTTL)
				}
			}
		case 33: // SRV
			if rdLen >= 6 {
				rec := getRec(owner)
				rec.recordTTL(rrTTL)
				port := int(binary.BigEndian.Uint16(rdata[4:6]))
				if port > 0 {
					rec.port = port
				}
			}
		case 16: // TXT
			rec := getRec(owner)
			rec.recordTTL(rrTTL)
			for j := 0; j < len(rdata); {
				tl := int(rdata[j])
				j++
				if j+tl > len(rdata) {
					break
				}
				kv := string(rdata[j : j+tl])
				j += tl
				if strings.HasPrefix(kv, "fn=") {
					rec.name = strings.TrimSpace(kv[3:])
				} else if strings.HasPrefix(kv, "md=") {
					rec.model = strings.TrimSpace(kv[3:])
				} else if strings.HasPrefix(kv, "id=") {
					val := strings.TrimSpace(kv[3:])
					if cut := strings.IndexAny(val, "#'"); cut != -1 {
						val = val[:cut]
					}
					rec.id = val
				}
			}
		}
	}

	var out []mdnsServiceRecord
	for _, key := range order {
		rec := byOwner[key]
		if !rec.goodbye && rec.id == "" && rec.port == 0 {
			continue
		}
		out = append(out, *rec)
	}
	return out, ipv4
}
