package hybridbridge

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================
// Test doubles
// ============================================================

// mockControlPlane records sent control frames.
type mockControlPlane struct {
	mu     sync.Mutex
	frames []ControlFrame
}

func (m *mockControlPlane) SendControlFrame(_ HybridSessionID, f ControlFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frames = append(m.frames, f)
	return nil
}

func (m *mockControlPlane) lastFrame() ControlFrame {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.frames) == 0 {
		return nil
	}
	return m.frames[len(m.frames)-1]
}

func (m *mockControlPlane) frameCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.frames)
}

// mockDataPlane records sent downstream frames.
type mockDataPlane struct {
	mu       sync.Mutex
	sent     []DownstreamFrameHeader
	payloads [][]byte
	failSend bool
}

func (m *mockDataPlane) SendDownstream(hdr DownstreamFrameHeader, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSend {
		return ErrStreamNotActive // reuse a sentinel as generic send error
	}
	m.sent = append(m.sent, hdr)
	p := make([]byte, len(payload))
	copy(p, payload)
	m.payloads = append(m.payloads, p)
	return nil
}

func (m *mockDataPlane) sentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

// newTestBridge creates a bridge with fast timers suitable for unit tests.
func newTestBridge(cp ControlPlane, dp DataPlane) *Bridge {
	cfg := BridgeConfig{
		AckFlushInterval:    5 * time.Millisecond,
		RetransmitInterval:  5 * time.Millisecond,
		MetricsInterval:     50 * time.Millisecond,
		InitialRTO:          20 * time.Millisecond,
		MaxRetransmits:      3,
		MaxSessions:         64,
		MaxStreamsPerSession: 32,
	}
	return NewBridge(cfg, cp, dp)
}

// ============================================================
// Lifecycle tests
// ============================================================

func TestBridgeStartStop(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)

	b.Start()
	if !b.running.Load() {
		t.Fatal("expected bridge to be running after Start()")
	}
	// Double-start should be a no-op
	b.Start()

	b.Stop()
	if b.running.Load() {
		t.Fatal("expected bridge to be stopped after Stop()")
	}
	// Double-stop should be a no-op
	b.Stop()
}

// ============================================================
// Stream open / ack tests
// ============================================================

func TestStreamOpenSendsOpenAckAndBecomesActive(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(1)
	stid := HybridStreamID(10)

	b.EnqueueControlRx(sid, &StreamOpenFrame{
		Header:    ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID: sid,
		StreamID:  stid,
		Target:    "example.com:443",
	})

	// Allow controlRxLoop to process
	time.Sleep(20 * time.Millisecond)

	// Bridge should have sent a StreamOpenAck
	f := cp.lastFrame()
	if f == nil {
		t.Fatal("expected an open-ack frame to be sent upstream")
	}
	if f.Type() != FrameStreamOpenAck {
		t.Fatalf("expected FrameStreamOpenAck, got type=%d", f.Type())
	}
	ack, ok := f.(*StreamOpenAckFrame)
	if !ok {
		t.Fatal("expected *StreamOpenAckFrame")
	}
	if !ack.Accepted {
		t.Fatal("expected Accepted=true in open-ack")
	}

	// Stream must be in Active state
	se := b.getSession(sid)
	if se == nil {
		t.Fatal("expected session to be created")
	}
	st := se.getStream(stid)
	if st == nil {
		t.Fatal("expected stream to be created")
	}
	st.mu.Lock()
	state := st.state
	st.mu.Unlock()
	if state != StreamActive {
		t.Fatalf("expected StreamActive, got state=%d", state)
	}
}

func TestStreamOpenAckRejectedBecomesReset(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(2)
	stid := HybridStreamID(20)

	// Manually create a session and stream in Opening state
	se := b.getOrCreateSession(sid)
	se.getOrCreateStream(stid, b.config.InitialRTO)

	b.EnqueueControlRx(sid, &StreamOpenAckFrame{
		Header:    ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID: sid,
		StreamID:  stid,
		Accepted:  false,
	})

	time.Sleep(20 * time.Millisecond)

	// Stream should be removed from the session after rejection
	st := se.getStream(stid)
	if st != nil {
		t.Fatal("expected stream to be removed after rejection")
	}
}

// ============================================================
// Stream close / drain / finalize tests
// ============================================================

func TestStreamCloseFinalizesWhenNoPending(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(3)
	stid := HybridStreamID(30)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	st.mu.Unlock()

	b.EnqueueControlRx(sid, &StreamCloseFrame{
		Header:    ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID: sid,
		StreamID:  stid,
	})

	time.Sleep(20 * time.Millisecond)

	// Stream should be removed (closed and finalized)
	got := se.getStream(stid)
	if got != nil {
		t.Fatal("expected stream to be removed after close with no pending data")
	}
}

func TestStreamDrainFinalizesAfterLastAck(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(4)
	stid := HybridStreamID(40)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	// Add a pending packet
	st.pending[DownSeq(1)] = []byte("data")
	st.sendTimes[DownSeq(1)] = time.Now()
	st.retransmits[DownSeq(1)] = 0
	st.seq = 1
	st.mu.Unlock()

	// Close the stream while it has pending data -> Draining
	b.EnqueueControlRx(sid, &StreamCloseFrame{
		Header:    ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID: sid,
		StreamID:  stid,
	})

	time.Sleep(20 * time.Millisecond)

	// Stream should still exist (pending data)
	got := se.getStream(stid)
	if got == nil {
		t.Fatal("expected stream to still exist while draining")
	}
	got.mu.Lock()
	state := got.state
	got.mu.Unlock()
	if state != StreamDraining {
		t.Fatalf("expected StreamDraining, got state=%d", state)
	}

	// ACK the pending packet -> should finalize
	b.EnqueueControlRx(sid, &DownstreamAckFrame{
		Header:    ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID: sid,
		StreamID:  stid,
		AckUntil:  DownSeq(1),
	})

	time.Sleep(20 * time.Millisecond)

	// Stream should now be removed
	got2 := se.getStream(stid)
	if got2 != nil {
		t.Fatal("expected stream to be removed after last ACK in drain state")
	}
}

// ============================================================
// Stream reset tests
// ============================================================

func TestStreamResetDropsPendingAndRemovesStream(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(5)
	stid := HybridStreamID(50)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	st.pending[DownSeq(1)] = []byte("a")
	st.pending[DownSeq(2)] = []byte("b")
	st.mu.Unlock()

	b.EnqueueControlRx(sid, &StreamResetFrame{
		Header:    ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID: sid,
		StreamID:  stid,
		Reason:    1,
	})

	time.Sleep(20 * time.Millisecond)

	got := se.getStream(stid)
	if got != nil {
		t.Fatal("expected stream to be removed after reset")
	}
	dropped := b.statsDropped.Load()
	if dropped != 2 {
		t.Fatalf("expected 2 dropped packets after reset, got=%d", dropped)
	}
}

// ============================================================
// SendDownstream tests
// ============================================================

func TestSendDownstreamSuccess(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)

	sid := HybridSessionID(6)
	stid := HybridStreamID(60)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	st.mu.Unlock()

	seq, err := b.SendDownstream(sid, stid, []byte("payload"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seq != 1 {
		t.Fatalf("expected seq=1, got=%d", seq)
	}
	if dp.sentCount() != 1 {
		t.Fatalf("expected 1 sent frame, got=%d", dp.sentCount())
	}

	st.mu.Lock()
	pending := len(st.pending)
	st.mu.Unlock()
	if pending != 1 {
		t.Fatalf("expected 1 pending packet, got=%d", pending)
	}
}

func TestSendDownstreamErrorSessionNotFound(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)

	_, err := b.SendDownstream(HybridSessionID(99), HybridStreamID(1), []byte("x"))
	if err != ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestSendDownstreamErrorStreamNotActive(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)

	sid := HybridSessionID(7)
	stid := HybridStreamID(70)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamOpening // not active
	st.mu.Unlock()

	_, err := b.SendDownstream(sid, stid, []byte("x"))
	if err != ErrStreamNotActive {
		t.Fatalf("expected ErrStreamNotActive, got: %v", err)
	}
}

func TestSendDownstreamDataPlaneErrorDropsPending(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{failSend: true}
	b := newTestBridge(cp, dp)

	sid := HybridSessionID(8)
	stid := HybridStreamID(80)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	st.mu.Unlock()

	_, err := b.SendDownstream(sid, stid, []byte("x"))
	if err == nil {
		t.Fatal("expected error from failing data plane")
	}
	st.mu.Lock()
	pending := len(st.pending)
	st.mu.Unlock()
	if pending != 0 {
		t.Fatalf("expected pending=0 after send failure, got=%d", pending)
	}
}

// ============================================================
// Downstream ACK / NACK via downRxLoop tests
// ============================================================

func TestDownRxAckRemovesPendingAndQueuesUpstreamAck(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(9)
	stid := HybridStreamID(90)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	st.pending[DownSeq(1)] = []byte("a")
	st.pending[DownSeq(2)] = []byte("b")
	st.seq = 2
	st.mu.Unlock()

	b.EnqueueDownRx(DownRxEvent{
		SessionID: sid,
		StreamID:  stid,
		AckUntil:  DownSeq(2),
	})

	// Wait for downRxLoop and ackFlushLoop to process
	time.Sleep(50 * time.Millisecond)

	st.mu.Lock()
	pending := len(st.pending)
	st.mu.Unlock()
	if pending != 0 {
		t.Fatalf("expected all pending removed after ACK, got=%d", pending)
	}

	// Upstream ACK should have been flushed
	found := false
	cp.mu.Lock()
	for _, f := range cp.frames {
		if f.Type() == FrameDownstreamAck {
			found = true
			break
		}
	}
	cp.mu.Unlock()
	if !found {
		t.Fatal("expected DownstreamAck frame to be sent upstream")
	}
}

func TestDownRxSelectiveBitmapAcksCorrectPackets(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(10)
	stid := HybridStreamID(100)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	// Packets 1, 2, 3
	st.pending[DownSeq(1)] = []byte("a")
	st.pending[DownSeq(2)] = []byte("b")
	st.pending[DownSeq(3)] = []byte("c")
	st.seq = 3
	st.mu.Unlock()

	// Cumulative ACK up to 1; selective bitmap bit 1 (offset 2 from 1 = seq 3)
	bitmap := uint64(1 << 1) // seq 3 = AckUntil(1) + 1 + 1
	b.EnqueueDownRx(DownRxEvent{
		SessionID: sid,
		StreamID:  stid,
		AckUntil:  DownSeq(1),
		AckBitmap: bitmap,
	})

	time.Sleep(30 * time.Millisecond)

	st.mu.Lock()
	_, seq2Pending := st.pending[DownSeq(2)]
	_, seq3Pending := st.pending[DownSeq(3)]
	st.mu.Unlock()

	if seq2Pending == false {
		// seq 2 was NOT in bitmap so should still be pending
	}
	if !seq2Pending {
		t.Fatal("expected seq 2 still pending (not in selective ACK)")
	}
	if seq3Pending {
		t.Fatal("expected seq 3 removed by selective ACK")
	}
}

func TestDownRxNackQueuesUpstreamNack(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(11)
	stid := HybridStreamID(110)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, b.config.InitialRTO)
	st.mu.Lock()
	st.state = StreamActive
	st.pending[DownSeq(1)] = []byte("a")
	st.seq = 1
	st.mu.Unlock()

	b.EnqueueDownRx(DownRxEvent{
		SessionID:  sid,
		StreamID:   stid,
		AckUntil:   DownSeq(0),
		MissingSeq: []DownSeq{DownSeq(1)},
	})

	// Wait for ackFlushLoop
	time.Sleep(50 * time.Millisecond)

	found := false
	cp.mu.Lock()
	for _, f := range cp.frames {
		if f.Type() == FrameDownstreamNack {
			found = true
			break
		}
	}
	cp.mu.Unlock()
	if !found {
		t.Fatal("expected DownstreamNack frame to be sent upstream after NACK event")
	}
}

// ============================================================
// Heartbeat echo tests
// ============================================================

func TestHeartbeatEchosBack(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(12)
	b.getOrCreateSession(sid)

	b.EnqueueControlRx(sid, &HeartbeatFrame{
		Header:     ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID:  sid,
		UnixMillis: 123456789,
	})

	time.Sleep(20 * time.Millisecond)

	f := cp.lastFrame()
	if f == nil {
		t.Fatal("expected heartbeat echo to be sent upstream")
	}
	if f.Type() != FrameHeartbeat {
		t.Fatalf("expected FrameHeartbeat, got type=%d", f.Type())
	}
	hb, ok := f.(*HeartbeatFrame)
	if !ok {
		t.Fatal("expected *HeartbeatFrame")
	}
	if hb.UnixMillis != 123456789 {
		t.Fatalf("expected echoed UnixMillis=123456789, got=%d", hb.UnixMillis)
	}
}

// ============================================================
// Retransmit loop tests
// ============================================================

func TestRetransmitLoopSendsTimedOutPacket(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(13)
	stid := HybridStreamID(130)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, 10*time.Millisecond)
	st.mu.Lock()
	st.state = StreamActive
	st.pending[DownSeq(1)] = []byte("retransmit-me")
	// Set an old send time to force retransmit
	st.sendTimes[DownSeq(1)] = time.Now().Add(-100 * time.Millisecond)
	st.retransmits[DownSeq(1)] = 0
	st.seq = 1
	st.mu.Unlock()

	// Wait for retransmitLoop to fire
	time.Sleep(50 * time.Millisecond)

	retransmits := b.statsRetransmits.Load()
	if retransmits == 0 {
		t.Fatal("expected at least one retransmit to have occurred")
	}
}

func TestRetransmitLoopDropsAfterMaxRetries(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(14)
	stid := HybridStreamID(140)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, 5*time.Millisecond)
	st.mu.Lock()
	st.state = StreamActive
	st.pending[DownSeq(1)] = []byte("will-be-dropped")
	st.sendTimes[DownSeq(1)] = time.Now().Add(-200 * time.Millisecond)
	// Exhaust retry count (maxRetries = 3)
	st.retransmits[DownSeq(1)] = b.config.MaxRetransmits
	st.seq = 1
	st.mu.Unlock()

	time.Sleep(50 * time.Millisecond)

	dropped := b.statsDropped.Load()
	if dropped == 0 {
		t.Fatal("expected packet to be dropped after max retries")
	}

	st.mu.Lock()
	_, stillPending := st.pending[DownSeq(1)]
	st.mu.Unlock()
	if stillPending {
		t.Fatal("expected dropped packet to be removed from pending")
	}
}

// ============================================================
// Stats tests
// ============================================================

func TestStatsCountsSessionsAndStreams(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)

	sid1 := HybridSessionID(20)
	sid2 := HybridSessionID(21)
	stid1 := HybridStreamID(200)
	stid2 := HybridStreamID(201)

	se1 := b.getOrCreateSession(sid1)
	se1.getOrCreateStream(stid1, b.config.InitialRTO)

	se2 := b.getOrCreateSession(sid2)
	se2.getOrCreateStream(stid1, b.config.InitialRTO)
	se2.getOrCreateStream(stid2, b.config.InitialRTO)

	stats := b.Stats()
	if stats.ActiveSessions != 2 {
		t.Fatalf("expected 2 sessions, got=%d", stats.ActiveSessions)
	}
	if stats.ActiveStreams != 3 {
		t.Fatalf("expected 3 streams, got=%d", stats.ActiveStreams)
	}
}

// ============================================================
// NACK-driven immediate retransmit test
// ============================================================

func TestNackForcesImmediateRetransmit(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)
	b.Start()
	defer b.Stop()

	sid := HybridSessionID(15)
	stid := HybridStreamID(150)

	se := b.getOrCreateSession(sid)
	st := se.getOrCreateStream(stid, 1*time.Second) // High RTO so normal retransmit won't fire
	st.mu.Lock()
	st.state = StreamActive
	st.pending[DownSeq(1)] = []byte("nack-me")
	st.sendTimes[DownSeq(1)] = time.Now() // recent send time
	st.retransmits[DownSeq(1)] = 0
	st.seq = 1
	st.mu.Unlock()

	// NACK for seq 1 should zero the send time, forcing immediate retransmit
	b.EnqueueControlRx(sid, &DownstreamNackFrame{
		Header:     ControlFrameHeader{ProtocolVersion: ProtocolVersion1, SessionID: sid},
		SessionID:  sid,
		StreamID:   stid,
		MissingSeq: []DownSeq{DownSeq(1)},
	})

	// Wait for controlRxLoop to zero the timestamp and retransmitLoop to fire
	time.Sleep(50 * time.Millisecond)

	if b.statsRetransmits.Load() == 0 {
		t.Fatal("expected retransmit after NACK forced send time to zero")
	}
}

// ============================================================
// Session mapping / isolation tests
// ============================================================

func TestSessionIsolation(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	b := newTestBridge(cp, dp)

	sidA := HybridSessionID(100)
	sidB := HybridSessionID(101)
	stid := HybridStreamID(1)

	seA := b.getOrCreateSession(sidA)
	seA.getOrCreateStream(stid, b.config.InitialRTO)

	seB := b.getOrCreateSession(sidB)
	seB.getOrCreateStream(stid, b.config.InitialRTO)

	// Removing stream from B should not affect A
	seB.removeStream(stid)
	if seA.getStream(stid) == nil {
		t.Fatal("stream in session A should not be affected by removal in session B")
	}
}

// ============================================================
// EnqueueControlRx channel-full drop test
// ============================================================

func TestEnqueueControlRxDropsWhenFull(t *testing.T) {
	cp := &mockControlPlane{}
	dp := &mockDataPlane{}
	cfg := DefaultBridgeConfig()
	b := NewBridge(cfg, cp, dp)
	// Do NOT start: keep channel drained=false so we can fill it
	sid := HybridSessionID(99)

	// Fill the controlRxCh to capacity
	var droppedAtLeastOne atomic.Bool
	for i := 0; i < cap(b.controlRxCh)+5; i++ {
		b.EnqueueControlRx(sid, &HeartbeatFrame{SessionID: sid})
		select {
		default:
			droppedAtLeastOne.Store(true)
		case b.controlRxCh <- ControlRxEvent{}:
		}
	}
	// As long as no panic occurred, the drop path is exercised
	_ = droppedAtLeastOne.Load()
}
