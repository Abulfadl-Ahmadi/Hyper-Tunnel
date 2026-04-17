package tunnel

import (
	"sync/atomic"
	"testing"
	"time"
)

// ===========================================================================
// RecvBuffer tests
// ===========================================================================

func TestRecvBufferInOrderDelivery(t *testing.T) {
	deliverCh := make(chan []byte, 16)
	rb := NewRecvBuffer(deliverCh, 50*time.Millisecond)

	payload := []byte("hello")
	if !rb.Receive(1, payload) {
		t.Fatal("Receive should return true for new packet")
	}
	if rb.LastDelivered() != 1 {
		t.Fatalf("expected lastDelivered=1, got=%d", rb.LastDelivered())
	}

	select {
	case data := <-deliverCh:
		if string(data) != "hello" {
			t.Fatalf("unexpected delivered data: %q", data)
		}
	default:
		t.Fatal("expected data in delivery channel")
	}
}

func TestRecvBufferDuplicateSuppression(t *testing.T) {
	deliverCh := make(chan []byte, 16)
	rb := NewRecvBuffer(deliverCh, 50*time.Millisecond)

	rb.Receive(1, []byte("a"))
	// Second receive of same seq should be a duplicate
	if rb.Receive(1, []byte("a")) {
		t.Fatal("Receive should return false for duplicate")
	}
	if rb.TotalDuplicates != 1 {
		t.Fatalf("expected 1 duplicate, got=%d", rb.TotalDuplicates)
	}

	// Deliver seq 1, then sending it again should also be a dup
	if rb.Receive(1, []byte("a")) {
		t.Fatal("Receive should return false for already-delivered packet")
	}
}

func TestRecvBufferOutOfOrderReassembly(t *testing.T) {
	deliverCh := make(chan []byte, 16)
	rb := NewRecvBuffer(deliverCh, 50*time.Millisecond)

	// Deliver seq 3 first (out of order)
	rb.Receive(3, []byte("three"))
	if rb.LastDelivered() != 0 {
		t.Fatalf("expected lastDelivered=0 after out-of-order, got=%d", rb.LastDelivered())
	}
	if rb.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got=%d", rb.PendingCount())
	}

	// Deliver seq 2 (still out of order; gap before 1)
	rb.Receive(2, []byte("two"))
	if rb.LastDelivered() != 0 {
		t.Fatalf("expected lastDelivered=0 after seq 2 without seq 1, got=%d", rb.LastDelivered())
	}
	if rb.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got=%d", rb.PendingCount())
	}

	// Now deliver seq 1 – this should flush 1, 2, 3 in order
	rb.Receive(1, []byte("one"))
	if rb.LastDelivered() != 3 {
		t.Fatalf("expected lastDelivered=3 after filling gap, got=%d", rb.LastDelivered())
	}
	if rb.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after flush, got=%d", rb.PendingCount())
	}

	// Verify delivery order
	expected := []string{"one", "two", "three"}
	for i, want := range expected {
		select {
		case data := <-deliverCh:
			if string(data) != want {
				t.Fatalf("delivery[%d]: got=%q want=%q", i, string(data), want)
			}
		default:
			t.Fatalf("delivery[%d]: expected data, channel empty", i)
		}
	}
}

func TestRecvBufferMemoryBound(t *testing.T) {
	deliverCh := make(chan []byte, 16)
	rb := NewRecvBuffer(deliverCh, 50*time.Millisecond)
	rb.maxReorderSlots = 4

	// Fill reorder slots with out-of-order packets (2..5)
	for i := uint32(2); i <= 5; i++ {
		rb.Receive(i, []byte("x"))
	}
	if rb.PendingCount() != 4 {
		t.Fatalf("expected 4 pending, got=%d", rb.PendingCount())
	}

	// 6th out-of-order packet should be dropped
	rb.Receive(6, []byte("x"))
	if rb.PendingCount() != 4 {
		t.Fatalf("expected still 4 pending after overflow, got=%d", rb.PendingCount())
	}
	if rb.TotalDropped != 1 {
		t.Fatalf("expected 1 dropped, got=%d", rb.TotalDropped)
	}
}

func TestRecvBufferGenerateAck(t *testing.T) {
	// 1ns ackInterval: ShouldSendAck always returns true, allowing direct
	// GenerateAck calls without waiting in tests.
	rb := NewRecvBuffer(nil, 1*time.Nanosecond)

	rb.Receive(1, []byte("a"))
	rb.Receive(3, []byte("c")) // out of order

	ackSeq, bitmap := rb.GenerateAck()
	if ackSeq != 1 {
		t.Fatalf("expected ackSeq=1, got=%d", ackSeq)
	}
	// seq 2 is missing, seq 3 is present → bit 1 (i=2, offset from ackSeq) should be set
	if bitmap&(1<<1) == 0 {
		t.Fatalf("expected bit 1 set for seq 3 (offset 2 from ackSeq 1), bitmap=%064b", bitmap)
	}
}

func TestRecvBufferGenerateNack(t *testing.T) {
	rb := NewRecvBuffer(nil, 50*time.Millisecond)

	rb.Receive(1, []byte("a"))
	rb.Receive(3, []byte("c")) // seq 2 missing
	rb.Receive(5, []byte("e")) // seqs 2,4 missing

	missing := rb.GenerateNack(0)
	if len(missing) == 0 {
		t.Fatal("expected missing sequences in NACK")
	}
	// seq 2 and 4 should be missing
	missingMap := make(map[uint32]bool)
	for _, m := range missing {
		missingMap[m] = true
	}
	if !missingMap[2] {
		t.Fatalf("expected seq 2 in missing, got=%v", missing)
	}
	if !missingMap[4] {
		t.Fatalf("expected seq 4 in missing, got=%v", missing)
	}
}

func TestRecvBufferNackHook(t *testing.T) {
	rb := NewRecvBuffer(nil, 50*time.Millisecond)

	var called atomic.Bool
	var calledWith []uint32
	rb.NackHook = func(missingSeqs []uint32) {
		called.Store(true)
		calledWith = append(calledWith, missingSeqs...)
	}

	rb.Receive(1, []byte("a"))
	rb.Receive(3, []byte("c")) // seq 2 missing

	rb.GenerateNack(0)
	if !called.Load() {
		t.Fatal("NackHook was not called")
	}
	if len(calledWith) == 0 || calledWith[0] != 2 {
		t.Fatalf("unexpected NackHook args: %v", calledWith)
	}
}

func TestRecvBufferAckHook(t *testing.T) {
	// 1ns ackInterval: ShouldSendAck always returns true, allowing direct
	// GenerateAck calls without waiting in tests.
	rb := NewRecvBuffer(nil, 1*time.Nanosecond)

	var called atomic.Bool
	var lastAck uint32
	var lastBitmap uint64
	rb.AckHook = func(ackSeq uint32, bitmap uint64) {
		called.Store(true)
		lastAck = ackSeq
		lastBitmap = bitmap
	}

	rb.Receive(1, []byte("a"))
	rb.Receive(3, []byte("c")) // out of order

	rb.GenerateAck()
	if !called.Load() {
		t.Fatal("AckHook was not called")
	}
	if lastAck != 1 {
		t.Fatalf("unexpected ackSeq in hook: got=%d want=1", lastAck)
	}
	if lastBitmap&(1<<1) == 0 {
		t.Fatalf("expected bit 1 set for seq 3, bitmap=%064b", lastBitmap)
	}
}

func TestRecvBufferStats(t *testing.T) {
	rb := NewRecvBuffer(nil, 50*time.Millisecond)
	rb.maxReorderSlots = 2

	rb.Receive(1, []byte("a"))
	rb.Receive(1, []byte("a")) // dup
	rb.Receive(3, []byte("c"))
	rb.Receive(4, []byte("d"))
	rb.Receive(5, []byte("e")) // dropped (maxReorderSlots=2 already filled by 3,4)

	recv, dups, dropped, pending := rb.Stats()
	if recv != 3 {
		t.Fatalf("expected 3 received, got=%d", recv)
	}
	if dups != 1 {
		t.Fatalf("expected 1 duplicate, got=%d", dups)
	}
	if dropped != 1 {
		t.Fatalf("expected 1 dropped, got=%d", dropped)
	}
	if pending != 2 {
		t.Fatalf("expected 2 pending, got=%d", pending)
	}
}

// ===========================================================================
// SendBuffer tests
// ===========================================================================

func TestSendBufferSendAndAck(t *testing.T) {
	var sent []uint32
	sb := NewSendBuffer(8, 200*time.Millisecond, func(seq uint32, _ []byte) error {
		sent = append(sent, seq)
		return nil
	})

	seqA := sb.Send([]byte("a"))
	seqB := sb.Send([]byte("b"))
	if seqA != 1 || seqB != 2 {
		t.Fatalf("unexpected sequences: A=%d B=%d", seqA, seqB)
	}
	if sb.Pending() != 2 {
		t.Fatalf("expected 2 pending, got=%d", sb.Pending())
	}

	acked := sb.ProcessAck(2, 0) // ACK up to seq 2
	if len(acked) != 2 {
		t.Fatalf("expected 2 acked, got=%d", len(acked))
	}
	if sb.Pending() != 0 {
		t.Fatalf("expected 0 pending after ACK, got=%d", sb.Pending())
	}
}

func TestSendBufferSelectiveAck(t *testing.T) {
	sb := NewSendBuffer(8, 200*time.Millisecond, func(_ uint32, _ []byte) error { return nil })

	sb.Send([]byte("a")) // seq 1
	sb.Send([]byte("b")) // seq 2
	sb.Send([]byte("c")) // seq 3

	// ACK cumulative seq 1, selective bitmap has seq 3 (offset 2 from 1 -> bit 1)
	bitmap := uint64(1 << 1) // bit 1 => seq 1+1+1 = seq 3
	acked := sb.ProcessAck(1, bitmap)
	if len(acked) != 2 { // seq 1 and seq 3
		t.Fatalf("expected 2 acked (1 and 3), got=%d: %v", len(acked), acked)
	}
	// seq 2 should still be pending
	if sb.Pending() != 1 {
		t.Fatalf("expected 1 pending (seq 2), got=%d", sb.Pending())
	}
}

func TestSendBufferWindowFull(t *testing.T) {
	sb := NewSendBuffer(2, 200*time.Millisecond, func(_ uint32, _ []byte) error { return nil })

	sb.Send([]byte("a"))
	sb.Send([]byte("b"))

	if sb.CanSend() {
		t.Fatal("CanSend should be false when window is full")
	}
}

func TestSendBufferRetransmitWithBackoff(t *testing.T) {
	var retransmitted []uint32
	sb := NewSendBuffer(8, 10*time.Millisecond, func(seq uint32, _ []byte) error {
		retransmitted = append(retransmitted, seq)
		return nil
	})

	sb.Send([]byte("data"))

	// First candidates: 10ms timeout
	time.Sleep(15 * time.Millisecond)
	candidates := sb.GetRetransmitCandidates()
	if len(candidates) == 0 {
		t.Fatal("expected retransmit candidate after timeout")
	}
	sb.Retransmit(candidates[0])
	if len(retransmitted) != 1 {
		t.Fatalf("expected 1 retransmit, got=%d", len(retransmitted))
	}

	// Second retransmit: backoff doubles the timeout
	// after first retransmit, the backoff = 10ms * 2^1 = 20ms
	// should NOT trigger again immediately
	candidates2 := sb.GetRetransmitCandidates()
	if len(candidates2) != 0 {
		t.Fatalf("expected no candidates immediately after retransmit (backoff), got=%d", len(candidates2))
	}
}

func TestSendBufferRetryLimit(t *testing.T) {
	sb := NewSendBuffer(8, 1*time.Millisecond, func(_ uint32, _ []byte) error { return nil })
	sb.maxRetries = 2

	sb.Send([]byte("data"))

	// Retransmit up to maxRetries+1 times total to trigger drop
	for i := 0; i <= sb.maxRetries+1; i++ {
		time.Sleep(2 * time.Millisecond)
		candidates := sb.GetRetransmitCandidates()
		for _, seq := range candidates {
			sb.Retransmit(seq)
		}
	}

	_, _, dropped, _ := sb.Stats()
	if dropped == 0 {
		t.Fatal("expected packet to be dropped after exceeding maxRetries")
	}
	if sb.Pending() != 0 {
		t.Fatalf("expected 0 pending after drop, got=%d", sb.Pending())
	}
}

func TestSendBufferDynamicRTO(t *testing.T) {
	// Seed a high initial RTO (500ms) and feed much faster RTT samples.
	// The SRTT estimator should converge well below the seeded value.
	sb := NewSendBuffer(8, 500*time.Millisecond, func(_ uint32, _ []byte) error { return nil })

	samples := []time.Duration{
		50 * time.Millisecond,
		60 * time.Millisecond,
		55 * time.Millisecond,
	}
	for _, s := range samples {
		sb.mu.Lock()
		sb.updateRTTLocked(s)
		sb.mu.Unlock()
	}

	sb.mu.Lock()
	srtt := sb.srtt
	ready := sb.rtoReady
	sb.mu.Unlock()

	if !ready {
		t.Fatal("expected rtoReady=true after RTT samples")
	}
	// SRTT should have converged below the seeded 500ms value
	if srtt >= 500*time.Millisecond {
		t.Fatalf("expected SRTT to converge below seeded value (500ms), got=%v", srtt)
	}
	// SRTT should be in the rough vicinity of the samples (well under 200ms)
	if srtt > 200*time.Millisecond {
		t.Fatalf("expected SRTT near sample values (<200ms), got=%v", srtt)
	}
}

func TestSendBufferKarnsAlgorithm(t *testing.T) {
	// Karn: RTT should only be sampled from first-transmit packets.
	var retransmitted []uint32
	sb := NewSendBuffer(8, 10*time.Millisecond, func(seq uint32, _ []byte) error {
		retransmitted = append(retransmitted, seq)
		return nil
	})

	sb.Send([]byte("data")) // seq 1

	// Force retransmit
	time.Sleep(12 * time.Millisecond)
	candidates := sb.GetRetransmitCandidates()
	if len(candidates) > 0 {
		sb.Retransmit(candidates[0])
	}

	// Record initial RTO state
	sb.mu.Lock()
	rtoBefore := sb.rto
	sb.mu.Unlock()

	// ACK seq 1 - because it was retransmitted, RTT should NOT be sampled
	sb.ProcessAck(1, 0)

	sb.mu.Lock()
	rtoAfter := sb.rto
	sb.mu.Unlock()

	// RTO should be unchanged (Karn's: no update for retransmitted packets)
	if rtoAfter != rtoBefore {
		t.Fatalf("Karn's algorithm violated: RTO changed after ACK of retransmitted packet: before=%v after=%v", rtoBefore, rtoAfter)
	}
}

func TestSendBufferStats(t *testing.T) {
	sb := NewSendBuffer(8, 1*time.Millisecond, func(_ uint32, _ []byte) error { return nil })
	sb.maxRetries = 1

	sb.Send([]byte("a"))
	sb.Send([]byte("b"))
	sb.ProcessAck(1, 0) // ACK seq 1

	// Force retransmit and exceed retry limit for seq 2
	for i := 0; i <= sb.maxRetries+1; i++ {
		time.Sleep(2 * time.Millisecond)
		candidates := sb.GetRetransmitCandidates()
		for _, seq := range candidates {
			sb.Retransmit(seq)
		}
	}

	sent, retransmits, dropped, _ := sb.Stats()
	if sent != 2 {
		t.Fatalf("expected 2 sent, got=%d", sent)
	}
	if retransmits == 0 {
		t.Fatalf("expected >0 retransmits, got=%d", retransmits)
	}
	if dropped == 0 {
		t.Fatalf("expected >0 dropped after retry limit, got=%d", dropped)
	}
}
