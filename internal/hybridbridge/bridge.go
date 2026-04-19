package hybridbridge

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Sentinel errors returned by Bridge public API.
var (
	ErrSessionNotFound = errors.New("hybridbridge: session not found")
	ErrStreamNotFound  = errors.New("hybridbridge: stream not found")
	ErrStreamNotActive = errors.New("hybridbridge: stream not in active state")
	ErrBridgeNotRunning = errors.New("hybridbridge: bridge is not running")
)

// StreamState represents the lifecycle state of a hybrid stream.
type StreamState uint8

const (
	// StreamOpening: open request sent, waiting for open-ack.
	StreamOpening StreamState = iota
	// StreamActive: stream is fully open and ready to carry data.
	StreamActive
	// StreamDraining: close requested; waiting for all in-flight data to be ACKed.
	StreamDraining
	// StreamClosed: graceful close complete.
	StreamClosed
	// StreamReset: forced reset; all pending data discarded.
	StreamReset
)

// streamEntry holds per-stream runtime state.
type streamEntry struct {
	hybridID    HybridStreamID
	state       StreamState
	seq         DownSeq        // last sequence number allocated for sending
	lastAcked   DownSeq        // highest cumulatively ACKed downstream sequence
	pending     map[DownSeq][]byte
	sendTimes   map[DownSeq]time.Time
	retransmits map[DownSeq]int
	rto         time.Duration
	createdAt   time.Time
	lastActive  time.Time
	mu          sync.Mutex
}

// sessionEntry holds per-session runtime state.
type sessionEntry struct {
	sessionID  HybridSessionID
	streams    map[HybridStreamID]*streamEntry
	createdAt  time.Time
	lastActive time.Time

	// Pending ACK/NACK frames to flush to MasterDNS on the next ackFlush tick.
	pendingAck  *DownstreamAckFrame
	pendingNack *DownstreamNackFrame
	ackMu       sync.Mutex

	mu sync.RWMutex
}

func (se *sessionEntry) getOrCreateStream(id HybridStreamID, initialRTO time.Duration) *streamEntry {
	se.mu.Lock()
	defer se.mu.Unlock()
	if st, ok := se.streams[id]; ok {
		return st
	}
	st := &streamEntry{
		hybridID:    id,
		state:       StreamOpening,
		pending:     make(map[DownSeq][]byte),
		sendTimes:   make(map[DownSeq]time.Time),
		retransmits: make(map[DownSeq]int),
		rto:         initialRTO,
		createdAt:   time.Now(),
		lastActive:  time.Now(),
	}
	se.streams[id] = st
	return st
}

func (se *sessionEntry) getStream(id HybridStreamID) *streamEntry {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.streams[id]
}

func (se *sessionEntry) removeStream(id HybridStreamID) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.streams, id)
}

// ============================================================
// Interfaces
// ============================================================

// ControlPlane is the interface the Bridge uses to communicate with the
// MasterDNS upstream control channel.
type ControlPlane interface {
	// SendControlFrame sends a control frame upstream via MasterDNS.
	SendControlFrame(sessionID HybridSessionID, frame ControlFrame) error
}

// DataPlane is the interface the Bridge uses to emit data downstream via
// spoof-tunnel.
type DataPlane interface {
	// SendDownstream emits a downstream data or retransmit frame.
	SendDownstream(hdr DownstreamFrameHeader, payload []byte) error
}

// ============================================================
// BridgeConfig
// ============================================================

// BridgeConfig holds tuneable parameters for the Bridge.
type BridgeConfig struct {
	// AckFlushInterval is how often pending ACK/NACK frames are sent upstream.
	AckFlushInterval time.Duration
	// RetransmitInterval is how often the retransmit timer fires.
	RetransmitInterval time.Duration
	// MetricsInterval is how often the metrics snapshot loop ticks.
	MetricsInterval time.Duration
	// InitialRTO seeds the per-stream retransmit timeout.
	InitialRTO time.Duration
	// MaxRetransmits is the per-packet retry limit before the packet is dropped.
	MaxRetransmits int
	// MaxSessions is the upper bound on concurrent sessions.
	MaxSessions int
	// MaxStreamsPerSession is the upper bound on per-session active streams.
	MaxStreamsPerSession int
}

// DefaultBridgeConfig returns a BridgeConfig with safe production defaults.
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		AckFlushInterval:    50 * time.Millisecond,
		RetransmitInterval:  20 * time.Millisecond,
		MetricsInterval:     1 * time.Second,
		InitialRTO:          200 * time.Millisecond,
		MaxRetransmits:      10,
		MaxSessions:         1024,
		MaxStreamsPerSession: 256,
	}
}

// ============================================================
// BridgeStats
// ============================================================

// BridgeStats is a snapshot of bridge-level counters.
type BridgeStats struct {
	ActiveSessions uint64
	ActiveStreams   uint64
	Retransmits    uint64
	DroppedPackets uint64
}

// ============================================================
// Event types
// ============================================================

// ControlRxEvent is an inbound control frame from MasterDNS.
type ControlRxEvent struct {
	SessionID HybridSessionID
	Frame     ControlFrame
}

// DownRxEvent is downstream feedback received from spoof-tunnel (ACK/NACK).
type DownRxEvent struct {
	SessionID  HybridSessionID
	StreamID   HybridStreamID
	AckUntil   DownSeq
	AckBitmap  uint64  // selective ACK bitmap for the 64 sequences after AckUntil
	MissingSeq []DownSeq
}

// ============================================================
// Bridge
// ============================================================

// Bridge is the runtime manager that connects the MasterDNS control-plane
// (upstream) with the spoof-tunnel downstream data-plane.
//
// Lifecycle:
//
//	bridge := NewBridge(cfg, cp, dp)
//	bridge.Start()
//	// ... runtime use ...
//	bridge.Stop()
type Bridge struct {
	config BridgeConfig
	cp     ControlPlane
	dp     DataPlane

	sessions   map[HybridSessionID]*sessionEntry
	sessionsMu sync.RWMutex

	// Inbound event channels
	controlRxCh chan ControlRxEvent
	downRxCh    chan DownRxEvent

	// Atomic stats counters
	statsRetransmits atomic.Uint64
	statsDropped     atomic.Uint64

	// Lifecycle
	running atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewBridge creates a new Bridge instance. Call Start() to begin operation.
func NewBridge(cfg BridgeConfig, cp ControlPlane, dp DataPlane) *Bridge {
	return &Bridge{
		config:      cfg,
		cp:          cp,
		dp:          dp,
		sessions:    make(map[HybridSessionID]*sessionEntry),
		controlRxCh: make(chan ControlRxEvent, 256),
		downRxCh:    make(chan DownRxEvent, 256),
		stopCh:      make(chan struct{}),
	}
}

// Start begins all six bridge goroutine loops. Calling Start on an already
// running Bridge is a no-op.
func (b *Bridge) Start() {
	if !b.running.CompareAndSwap(false, true) {
		return
	}
	b.wg.Add(6)
	go b.controlRxLoop()
	go b.downRxLoop()
	go b.schedulerLoop()
	go b.ackFlushLoop()
	go b.retransmitLoop()
	go b.metricsLoop()
}

// Stop signals all loops to exit and waits for them to finish. Calling Stop
// on a Bridge that has not been started is a no-op.
func (b *Bridge) Stop() {
	if !b.running.CompareAndSwap(true, false) {
		return
	}
	close(b.stopCh)
	b.wg.Wait()
}

// EnqueueControlRx delivers an inbound control frame from MasterDNS to the
// bridge. Frames are processed asynchronously by controlRxLoop.
// If the internal channel is full the frame is dropped (back-pressure signal).
func (b *Bridge) EnqueueControlRx(sessionID HybridSessionID, frame ControlFrame) {
	select {
	case b.controlRxCh <- ControlRxEvent{SessionID: sessionID, Frame: frame}:
	default:
	}
}

// EnqueueDownRx delivers a downstream feedback event from spoof-tunnel to the
// bridge. Events are processed asynchronously by downRxLoop.
// If the internal channel is full the event is dropped.
func (b *Bridge) EnqueueDownRx(ev DownRxEvent) {
	select {
	case b.downRxCh <- ev:
	default:
	}
}

// SendDownstream enqueues a downstream data frame for an active stream.
// The payload is tracked for potential retransmission.
// Returns the allocated downstream sequence number on success.
func (b *Bridge) SendDownstream(sessionID HybridSessionID, streamID HybridStreamID, payload []byte) (DownSeq, error) {
	se := b.getSession(sessionID)
	if se == nil {
		return 0, ErrSessionNotFound
	}
	st := se.getStream(streamID)
	if st == nil {
		return 0, ErrStreamNotFound
	}

	st.mu.Lock()
	if st.state != StreamActive {
		st.mu.Unlock()
		return 0, ErrStreamNotActive
	}
	st.seq++
	seq := st.seq

	dataCopy := make([]byte, len(payload))
	copy(dataCopy, payload)
	st.pending[seq] = dataCopy
	st.sendTimes[seq] = time.Now()
	st.retransmits[seq] = 0
	st.lastActive = time.Now()
	st.mu.Unlock()

	hdr := DownstreamFrameHeader{
		ProtocolVersion: ProtocolVersion1,
		SessionID:       sessionID,
		StreamID:        streamID,
		Seq:             seq,
		Flags:           0x00,
	}
	if err := b.dp.SendDownstream(hdr, payload); err != nil {
		// Remove from pending so it is not retransmitted
		st.mu.Lock()
		delete(st.pending, seq)
		delete(st.sendTimes, seq)
		delete(st.retransmits, seq)
		st.mu.Unlock()
		return 0, err
	}
	return seq, nil
}

// Stats returns a point-in-time snapshot of bridge metrics.
func (b *Bridge) Stats() BridgeStats {
	b.sessionsMu.RLock()
	activeSessions := uint64(len(b.sessions))
	var activeStreams uint64
	for _, se := range b.sessions {
		se.mu.RLock()
		activeStreams += uint64(len(se.streams))
		se.mu.RUnlock()
	}
	b.sessionsMu.RUnlock()
	return BridgeStats{
		ActiveSessions: activeSessions,
		ActiveStreams:   activeStreams,
		Retransmits:    b.statsRetransmits.Load(),
		DroppedPackets: b.statsDropped.Load(),
	}
}

// ============================================================
// Session helpers
// ============================================================

func (b *Bridge) getOrCreateSession(id HybridSessionID) *sessionEntry {
	b.sessionsMu.Lock()
	defer b.sessionsMu.Unlock()
	if s, ok := b.sessions[id]; ok {
		return s
	}
	s := &sessionEntry{
		sessionID:  id,
		streams:    make(map[HybridStreamID]*streamEntry),
		createdAt:  time.Now(),
		lastActive: time.Now(),
	}
	b.sessions[id] = s
	return s
}

func (b *Bridge) getSession(id HybridSessionID) *sessionEntry {
	b.sessionsMu.RLock()
	defer b.sessionsMu.RUnlock()
	return b.sessions[id]
}

func (b *Bridge) removeSession(id HybridSessionID) {
	b.sessionsMu.Lock()
	defer b.sessionsMu.Unlock()
	delete(b.sessions, id)
}

// maybeFinalizeDrain transitions a draining stream to closed once all
// in-flight data has been acknowledged. The stream is removed from the session.
func (b *Bridge) maybeFinalizeDrain(se *sessionEntry, st *streamEntry) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.state == StreamDraining && len(st.pending) == 0 {
		st.state = StreamClosed
		se.removeStream(st.hybridID)
	}
}

// ============================================================
// controlRxLoop – processes inbound control frames from MasterDNS
// ============================================================

func (b *Bridge) controlRxLoop() {
	defer b.wg.Done()
	for {
		select {
		case <-b.stopCh:
			return
		case ev := <-b.controlRxCh:
			b.handleControlFrame(ev.SessionID, ev.Frame)
		}
	}
}

func (b *Bridge) handleControlFrame(sessionID HybridSessionID, frame ControlFrame) {
	switch f := frame.(type) {
	case *StreamOpenFrame:
		b.handleStreamOpen(f)
	case *StreamOpenAckFrame:
		b.handleStreamOpenAck(f)
	case *StreamCloseFrame:
		b.handleStreamClose(f)
	case *StreamResetFrame:
		b.handleStreamReset(f)
	case *DownstreamAckFrame:
		b.handleDownstreamAck(f)
	case *DownstreamNackFrame:
		b.handleDownstreamNack(f)
	case *HeartbeatFrame:
		b.handleHeartbeat(f)
	case *KeyRotationFrame:
		b.handleKeyRotation(f)
	}
}

func (b *Bridge) handleStreamOpen(f *StreamOpenFrame) {
	se := b.getOrCreateSession(f.SessionID)
	st := se.getOrCreateStream(f.StreamID, b.config.InitialRTO)

	st.mu.Lock()
	st.state = StreamOpening
	st.lastActive = time.Now()
	st.mu.Unlock()

	// Send open-ack and immediately mark active
	ack := &StreamOpenAckFrame{
		Header: ControlFrameHeader{
			ProtocolVersion: ProtocolVersion1,
			SessionID:       f.SessionID,
		},
		SessionID: f.SessionID,
		StreamID:  f.StreamID,
		Accepted:  true,
	}
	_ = b.cp.SendControlFrame(f.SessionID, ack)

	st.mu.Lock()
	st.state = StreamActive
	st.mu.Unlock()
}

func (b *Bridge) handleStreamOpenAck(f *StreamOpenAckFrame) {
	se := b.getSession(f.SessionID)
	if se == nil {
		return
	}
	st := se.getStream(f.StreamID)
	if st == nil {
		return
	}
	st.mu.Lock()
	if f.Accepted {
		if st.state == StreamOpening {
			st.state = StreamActive
		}
	} else {
		st.state = StreamReset
		st.pending = make(map[DownSeq][]byte)
		st.sendTimes = make(map[DownSeq]time.Time)
		st.retransmits = make(map[DownSeq]int)
	}
	st.lastActive = time.Now()
	st.mu.Unlock()

	if !f.Accepted {
		se.removeStream(f.StreamID)
	}
}

func (b *Bridge) handleStreamClose(f *StreamCloseFrame) {
	se := b.getSession(f.SessionID)
	if se == nil {
		return
	}
	st := se.getStream(f.StreamID)
	if st == nil {
		return
	}
	st.mu.Lock()
	if st.state == StreamActive {
		st.state = StreamDraining
	}
	st.lastActive = time.Now()
	hasPending := len(st.pending) > 0
	st.mu.Unlock()

	if !hasPending {
		// Nothing in flight: finalize immediately
		b.maybeFinalizeDrain(se, st)
	}
}

func (b *Bridge) handleStreamReset(f *StreamResetFrame) {
	se := b.getSession(f.SessionID)
	if se == nil {
		return
	}
	st := se.getStream(f.StreamID)
	if st == nil {
		return
	}
	st.mu.Lock()
	st.state = StreamReset
	b.statsDropped.Add(uint64(len(st.pending)))
	st.pending = make(map[DownSeq][]byte)
	st.sendTimes = make(map[DownSeq]time.Time)
	st.retransmits = make(map[DownSeq]int)
	st.lastActive = time.Now()
	st.mu.Unlock()

	se.removeStream(f.StreamID)
}

func (b *Bridge) handleDownstreamAck(f *DownstreamAckFrame) {
	se := b.getSession(f.SessionID)
	if se == nil {
		return
	}
	st := se.getStream(f.StreamID)
	if st == nil {
		return
	}
	st.mu.Lock()
	for seq := range st.pending {
		if seq <= f.AckUntil {
			delete(st.pending, seq)
			delete(st.sendTimes, seq)
			delete(st.retransmits, seq)
		}
	}
	if f.AckUntil > st.lastAcked {
		st.lastAcked = f.AckUntil
	}
	st.lastActive = time.Now()
	st.mu.Unlock()

	b.maybeFinalizeDrain(se, st)
}

func (b *Bridge) handleDownstreamNack(f *DownstreamNackFrame) {
	se := b.getSession(f.SessionID)
	if se == nil {
		return
	}
	st := se.getStream(f.StreamID)
	if st == nil {
		return
	}
	st.mu.Lock()
	// Force immediate retransmit by clearing the send timestamp
	for _, seq := range f.MissingSeq {
		if _, ok := st.pending[seq]; ok {
			st.sendTimes[seq] = time.Time{}
		}
	}
	st.lastActive = time.Now()
	st.mu.Unlock()
}

func (b *Bridge) handleHeartbeat(f *HeartbeatFrame) {
	se := b.getSession(f.SessionID)
	if se == nil {
		return
	}
	se.mu.Lock()
	se.lastActive = time.Now()
	se.mu.Unlock()

	// Echo heartbeat back
	resp := &HeartbeatFrame{
		Header: ControlFrameHeader{
			ProtocolVersion: ProtocolVersion1,
			SessionID:       f.SessionID,
		},
		SessionID:  f.SessionID,
		UnixMillis: f.UnixMillis,
	}
	_ = b.cp.SendControlFrame(f.SessionID, resp)
}

func (b *Bridge) handleKeyRotation(f *KeyRotationFrame) {
	// Key rotation is acknowledged here. Actual cryptographic epoch transition
	// is handled outside the bridge at the crypto layer.
	_ = f
}

// ============================================================
// downRxLoop – processes downstream feedback from spoof-tunnel
// ============================================================

func (b *Bridge) downRxLoop() {
	defer b.wg.Done()
	for {
		select {
		case <-b.stopCh:
			return
		case ev := <-b.downRxCh:
			b.handleDownRx(ev)
		}
	}
}

func (b *Bridge) handleDownRx(ev DownRxEvent) {
	se := b.getSession(ev.SessionID)
	if se == nil {
		return
	}
	st := se.getStream(ev.StreamID)
	if st == nil {
		return
	}

	st.mu.Lock()
	// Cumulative ACK
	for seq := range st.pending {
		if seq <= ev.AckUntil {
			delete(st.pending, seq)
			delete(st.sendTimes, seq)
			delete(st.retransmits, seq)
		}
	}
	if ev.AckUntil > st.lastAcked {
		st.lastAcked = ev.AckUntil
	}
	// Selective ACK bitmap (64 bits beyond AckUntil)
	for i := uint64(0); i < 64; i++ {
		if ev.AckBitmap&(1<<i) != 0 {
			seq := DownSeq(uint64(ev.AckUntil) + 1 + i)
			delete(st.pending, seq)
			delete(st.sendTimes, seq)
			delete(st.retransmits, seq)
		}
	}
	st.lastActive = time.Now()
	st.mu.Unlock()

	// Queue upstream ACK/NACK for the next ackFlush tick
	se.ackMu.Lock()
	se.pendingAck = &DownstreamAckFrame{
		Header: ControlFrameHeader{
			ProtocolVersion: ProtocolVersion1,
			SessionID:       ev.SessionID,
		},
		SessionID: ev.SessionID,
		StreamID:  ev.StreamID,
		AckUntil:  ev.AckUntil,
	}
	if len(ev.MissingSeq) > 0 {
		se.pendingNack = &DownstreamNackFrame{
			Header: ControlFrameHeader{
				ProtocolVersion: ProtocolVersion1,
				SessionID:       ev.SessionID,
			},
			SessionID:  ev.SessionID,
			StreamID:   ev.StreamID,
			MissingSeq: ev.MissingSeq,
		}
	}
	se.ackMu.Unlock()

	b.maybeFinalizeDrain(se, st)
}

// ============================================================
// schedulerLoop – downstream send pacing
// ============================================================

// schedulerLoop is the downstream send scheduler. In this initial
// implementation it ticks at RetransmitInterval as a heartbeat for future
// congestion-window-driven pacing logic.
func (b *Bridge) schedulerLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.config.RetransmitInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			// Future: congestion-window-driven send pacing
		}
	}
}

// ============================================================
// ackFlushLoop – flushes pending ACK/NACK frames upstream
// ============================================================

func (b *Bridge) ackFlushLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.config.AckFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.flushPendingAcks()
		}
	}
}

func (b *Bridge) flushPendingAcks() {
	b.sessionsMu.RLock()
	sessions := make([]*sessionEntry, 0, len(b.sessions))
	for _, se := range b.sessions {
		sessions = append(sessions, se)
	}
	b.sessionsMu.RUnlock()

	for _, se := range sessions {
		se.ackMu.Lock()
		ack := se.pendingAck
		nack := se.pendingNack
		se.pendingAck = nil
		se.pendingNack = nil
		se.ackMu.Unlock()

		if ack != nil {
			_ = b.cp.SendControlFrame(ack.SessionID, ack)
		}
		if nack != nil {
			_ = b.cp.SendControlFrame(nack.SessionID, nack)
		}
	}
}

// ============================================================
// retransmitLoop – retransmits timed-out downstream packets
// ============================================================

func (b *Bridge) retransmitLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.config.RetransmitInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.processRetransmits()
		}
	}
}

func (b *Bridge) processRetransmits() {
	now := time.Now()

	b.sessionsMu.RLock()
	sessions := make([]*sessionEntry, 0, len(b.sessions))
	for _, se := range b.sessions {
		sessions = append(sessions, se)
	}
	b.sessionsMu.RUnlock()

	for _, se := range sessions {
		se.mu.RLock()
		streams := make([]*streamEntry, 0, len(se.streams))
		for _, st := range se.streams {
			streams = append(streams, st)
		}
		se.mu.RUnlock()

		for _, st := range streams {
			b.retransmitStream(se.sessionID, st, now)
		}
	}
}

func (b *Bridge) retransmitStream(sessionID HybridSessionID, st *streamEntry, now time.Time) {
	st.mu.Lock()
	if st.state != StreamActive && st.state != StreamDraining {
		st.mu.Unlock()
		return
	}

	type candidate struct {
		seq  DownSeq
		data []byte
	}
	var toSend []candidate
	var toDrop []DownSeq

	for seq, data := range st.pending {
		retries := st.retransmits[seq]
		if retries >= b.config.MaxRetransmits {
			toDrop = append(toDrop, seq)
			continue
		}
		// Exponential backoff: rto * 2^retries, capped at rtoMax
		effectiveRTO := st.rto
		for i := 0; i < retries; i++ {
			effectiveRTO *= 2
			if effectiveRTO > 30*time.Second {
				effectiveRTO = 30 * time.Second
				break
			}
		}
		if now.Sub(st.sendTimes[seq]) >= effectiveRTO {
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)
			toSend = append(toSend, candidate{seq: seq, data: dataCopy})
		}
	}

	for _, seq := range toDrop {
		delete(st.pending, seq)
		delete(st.sendTimes, seq)
		delete(st.retransmits, seq)
	}
	droppedCount := uint64(len(toDrop))

	// Update send times and retry counters before releasing the lock
	for _, c := range toSend {
		st.retransmits[c.seq]++
		st.sendTimes[c.seq] = now
	}
	st.mu.Unlock()

	if droppedCount > 0 {
		b.statsDropped.Add(droppedCount)
	}

	for _, c := range toSend {
		hdr := DownstreamFrameHeader{
			ProtocolVersion: ProtocolVersion1,
			SessionID:       sessionID,
			StreamID:        st.hybridID,
			Seq:             c.seq,
			Flags:           0x01, // retransmit flag
		}
		if err := b.dp.SendDownstream(hdr, c.data); err == nil {
			b.statsRetransmits.Add(1)
		}
	}
}

// ============================================================
// metricsLoop – periodic metrics snapshot
// ============================================================

// metricsLoop ticks at MetricsInterval. The snapshot is available via Stats().
// Future integrations can hook a Prometheus exporter or structured logger here.
func (b *Bridge) metricsLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.config.MetricsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			_ = b.Stats() // reserved for future metric emission hook
		}
	}
}
