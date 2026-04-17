package tunnel

import (
	"sync"
	"time"
)

// Default memory bounds and retry limits.
const (
	defaultMaxReorderSlots = 256
	defaultMaxRetries      = 10
)

// Dynamic RTO parameters (RFC 6298).
const (
	rtoAlpha   = 0.125 // SRTT smoothing factor (1/8)
	rtoBeta    = 0.25  // RTTVAR smoothing factor (1/4)
	rtoGranule = time.Millisecond
	rtoMin     = 200 * time.Millisecond
	rtoMax     = 30 * time.Second
)

// ===========================================================================
// SendBuffer - reliable send with dynamic RTO and retry limits
// ===========================================================================

// pendingPacket stores a single in-flight packet.
type pendingPacket struct {
	data        []byte
	sendTime    time.Time
	retransmits int // how many times this packet has been retransmitted
}

// SendBuffer manages packets waiting for acknowledgment (server-side).
// It implements sliding window, dynamic RTO retransmission, and retry limits.
type SendBuffer struct {
	mu sync.Mutex

	// Packets waiting for ACK: seqNum -> packet
	pending map[uint32]*pendingPacket

	// Sequence tracking
	nextSeq   uint32 // Next sequence to use for new packets
	lastAcked uint32 // Last continuously ACKed sequence

	// Flow control
	windowSize int // Max in-flight packets

	// Dynamic RTO state (RFC 6298)
	srtt     time.Duration // Smoothed RTT
	rttvar   time.Duration // RTT variance
	rto      time.Duration // Current retransmit timeout
	rtoReady bool          // Whether we have a measured RTO

	// Retry limits
	maxRetries int

	// Callback for retransmitting packets
	retransmitFn func(seqNum uint32, data []byte) error

	// Stats
	totalSent        uint64
	totalRetransmits uint64
	totalDropped     uint64
}

// NewSendBuffer creates a new send buffer.
// retransmitAge seeds the initial RTO (for backward API compatibility).
func NewSendBuffer(windowSize int, retransmitAge time.Duration, retransmitFn func(uint32, []byte) error) *SendBuffer {
	if retransmitAge <= 0 {
		retransmitAge = rtoMin
	}
	return &SendBuffer{
		pending:      make(map[uint32]*pendingPacket),
		nextSeq:      1,
		lastAcked:    0,
		windowSize:   windowSize,
		srtt:         retransmitAge,
		rttvar:       retransmitAge / 2,
		rto:          retransmitAge,
		rtoReady:     false,
		maxRetries:   defaultMaxRetries,
		retransmitFn: retransmitFn,
	}
}

// updateRTTLocked updates RTO from a measured RTT sample (RFC 6298 §2).
// Must be called with sb.mu held.
// Samples below rtoGranule are ignored: they are below clock resolution and
// would corrupt the estimator (e.g. loopback ACKs in tests).
func (sb *SendBuffer) updateRTTLocked(sample time.Duration) {
	if sample < rtoGranule {
		return
	}
	if !sb.rtoReady {
		// First measurement: SRTT = R, RTTVAR = R/2
		sb.srtt = sample
		sb.rttvar = sample / 2
		sb.rtoReady = true
	} else {
		// RTTVAR = (1-beta)*RTTVAR + beta*|SRTT-R'|
		delta := sb.srtt - sample
		if delta < 0 {
			delta = -delta
		}
		sb.rttvar = time.Duration(float64(sb.rttvar)*(1-rtoBeta) + float64(delta)*rtoBeta)
		// SRTT = (1-alpha)*SRTT + alpha*R'
		sb.srtt = time.Duration(float64(sb.srtt)*(1-rtoAlpha) + float64(sample)*rtoAlpha)
	}
	// RTO = SRTT + max(G, 4*RTTVAR)
	granule4 := 4 * rtoGranule
	if 4*sb.rttvar > granule4 {
		granule4 = 4 * sb.rttvar
	}
	sb.rto = sb.srtt + granule4
	if sb.rto < rtoMin {
		sb.rto = rtoMin
	}
	if sb.rto > rtoMax {
		sb.rto = rtoMax
	}
}

// CanSend returns true if we can send more packets (window not full).
func (sb *SendBuffer) CanSend() bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return len(sb.pending) < sb.windowSize
}

// Send records a packet as sent and returns its sequence number.
func (sb *SendBuffer) Send(data []byte) uint32 {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	seqNum := sb.nextSeq
	sb.nextSeq++

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	sb.pending[seqNum] = &pendingPacket{
		data:        dataCopy,
		sendTime:    time.Now(),
		retransmits: 0,
	}
	sb.totalSent++
	return seqNum
}

// ProcessAck handles an ACK from client.
// Returns list of seqNums that were acknowledged.
// Applies Karn's algorithm: RTT is only sampled from first-transmit packets.
func (sb *SendBuffer) ProcessAck(ackSeqNum uint32, recvBitmap uint64) []uint32 {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	var acked []uint32

	// Cumulative ACK: remove all packets up to ackSeqNum
	for seq := sb.lastAcked + 1; seq <= ackSeqNum; seq++ {
		if pkt, exists := sb.pending[seq]; exists {
			// Karn's algorithm: only update RTT for non-retransmitted packets
			if pkt.retransmits == 0 {
				sb.updateRTTLocked(time.Since(pkt.sendTime))
			}
			delete(sb.pending, seq)
			acked = append(acked, seq)
		}
	}
	if ackSeqNum > sb.lastAcked {
		sb.lastAcked = ackSeqNum
	}

	// Selective ACK bitmap (next 64 packets after ackSeqNum)
	for i := uint64(0); i < 64; i++ {
		if recvBitmap&(1<<i) != 0 {
			seq := ackSeqNum + 1 + uint32(i)
			if pkt, exists := sb.pending[seq]; exists {
				if pkt.retransmits == 0 {
					sb.updateRTTLocked(time.Since(pkt.sendTime))
				}
				delete(sb.pending, seq)
				acked = append(acked, seq)
			}
		}
	}

	return acked
}

// GetRetransmitCandidates returns packets that need retransmission.
// Packets that have exceeded maxRetries are dropped and not returned.
// Uses exponential backoff: effective timeout = rto * 2^retransmits.
func (sb *SendBuffer) GetRetransmitCandidates() []uint32 {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	now := time.Now()
	var candidates []uint32

	for seqNum, pkt := range sb.pending {
		if pkt.retransmits >= sb.maxRetries {
			// Drop packet after max retries
			delete(sb.pending, seqNum)
			sb.totalDropped++
			continue
		}

		// Exponential backoff: rto * 2^retransmits (capped at rtoMax)
		effectiveRTO := sb.rto
		for i := 0; i < pkt.retransmits; i++ {
			effectiveRTO *= 2
			if effectiveRTO > rtoMax {
				effectiveRTO = rtoMax
				break
			}
		}

		if now.Sub(pkt.sendTime) >= effectiveRTO {
			candidates = append(candidates, seqNum)
		}
	}

	return candidates
}

// Retransmit sends a packet again, increments its retry counter, and resets its send timer.
func (sb *SendBuffer) Retransmit(seqNum uint32) error {
	sb.mu.Lock()
	pkt, exists := sb.pending[seqNum]
	if !exists {
		sb.mu.Unlock()
		return nil // Already ACKed or dropped
	}
	data := pkt.data
	pkt.retransmits++
	pkt.sendTime = time.Now()
	sb.totalRetransmits++
	sb.mu.Unlock()

	return sb.retransmitFn(seqNum, data)
}

// Pending returns number of unacked packets.
func (sb *SendBuffer) Pending() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return len(sb.pending)
}

// Stats returns send buffer statistics.
func (sb *SendBuffer) Stats() (totalSent, totalRetransmits, totalDropped uint64, pendingCount int) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.totalSent, sb.totalRetransmits, sb.totalDropped, len(sb.pending)
}

// ===========================================================================
// RecvBuffer - true reassembly receive buffer with reorder support
// ===========================================================================

// RecvBuffer manages received packets and generates ACKs (client-side).
// It provides true out-of-order reassembly with bounded memory, duplicate
// suppression, and ACK/NACK generation hooks for upstream tunneling integration.
//
// Compatibility: the public API is identical to the prior simplistic
// implementation so existing code requires no changes.
type RecvBuffer struct {
	mu sync.Mutex

	// Out-of-order pending packets: seqNum -> payload bytes
	pending map[uint32][]byte

	// Last contiguously delivered sequence number (delivery cursor)
	lastDelivered uint32

	// Duplicate detection for pending (undelivered) packets.
	// Entries are removed once a packet is delivered.
	received map[uint32]bool

	// Memory bounds
	maxReorderSlots int

	// Output channel for ordered delivery (nil = delivery via hooks only)
	deliverCh chan []byte

	// ACK/NACK generation hooks for upstream tunneling integration.
	// AckHook is called with (ackUntil, selectiveBitmap) when GenerateAck is invoked.
	// NackHook is called with missing sequences when GenerateNack is invoked.
	// Both are nil by default (hooks disabled for standalone operation).
	AckHook  func(ackSeqNum uint32, recvBitmap uint64)
	NackHook func(missingSeqs []uint32)

	// ACK timing
	ackInterval time.Duration
	lastAckTime time.Time

	// Stats
	TotalReceived   uint64
	TotalDuplicates uint64
	TotalDropped    uint64
}

// NewRecvBuffer creates a new receive buffer.
func NewRecvBuffer(deliverCh chan []byte, ackInterval time.Duration) *RecvBuffer {
	return &RecvBuffer{
		pending:         make(map[uint32][]byte),
		received:        make(map[uint32]bool),
		lastDelivered:   0,
		maxReorderSlots: defaultMaxReorderSlots,
		deliverCh:       deliverCh,
		ackInterval:     ackInterval,
	}
}

// Receive processes an incoming sequenced packet.
// Returns true if this is a new (non-duplicate) packet.
// Out-of-order packets are buffered up to maxReorderSlots; excess packets are dropped.
// When a gap is filled, all consecutive buffered packets are delivered in order.
func (rb *RecvBuffer) Receive(seqNum uint32, data []byte) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Duplicate check: already delivered (contiguous cursor covers this seq)
	if seqNum <= rb.lastDelivered && rb.lastDelivered > 0 {
		rb.TotalDuplicates++
		return false
	}

	// Duplicate check: already seen this out-of-order packet
	if rb.received[seqNum] {
		rb.TotalDuplicates++
		return false
	}
	rb.received[seqNum] = true

	if seqNum == rb.lastDelivered+1 {
		// In-order delivery: deliver immediately and flush any buffered follow-ons
		rb.TotalReceived++
		delete(rb.received, seqNum) // cursor now covers this seq
		rb.deliverLocked(data)
		rb.lastDelivered = seqNum
		rb.flushPendingLocked()
	} else if seqNum > rb.lastDelivered+1 {
		// Out-of-order: buffer if room available
		if len(rb.pending) >= rb.maxReorderSlots {
			// Drop: enforce memory bound
			delete(rb.received, seqNum)
			rb.TotalDropped++
		} else {
			rb.TotalReceived++
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)
			rb.pending[seqNum] = dataCopy
		}
	}

	return true
}

// deliverLocked sends data to the delivery channel (if set) without blocking.
// Must be called with rb.mu held.
func (rb *RecvBuffer) deliverLocked(data []byte) {
	if rb.deliverCh == nil {
		return
	}
	select {
	case rb.deliverCh <- data:
	default:
	}
}

// flushPendingLocked delivers all consecutive buffered packets starting at
// lastDelivered+1 and advances the delivery cursor accordingly.
// Must be called with rb.mu held.
func (rb *RecvBuffer) flushPendingLocked() {
	for {
		next := rb.lastDelivered + 1
		data, exists := rb.pending[next]
		if !exists {
			break
		}
		delete(rb.pending, next)
		delete(rb.received, next) // cursor now covers this seq
		rb.deliverLocked(data)
		rb.lastDelivered = next
	}
}

// ShouldSendAck returns true if it's time to send an ACK.
func (rb *RecvBuffer) ShouldSendAck() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return time.Since(rb.lastAckTime) >= rb.ackInterval
}

// GenerateAck creates ACK data and invokes AckHook if set.
// Returns ackSeqNum (last contiguously delivered) and recvBitmap (next 64 pending seqs).
func (rb *RecvBuffer) GenerateAck() (ackSeqNum uint32, recvBitmap uint64) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.lastAckTime = time.Now()
	ackSeqNum = rb.lastDelivered

	// Build selective bitmap for next 64 sequences
	for i := uint32(1); i <= 64; i++ {
		seq := ackSeqNum + i
		if _, exists := rb.pending[seq]; exists {
			recvBitmap |= (1 << (i - 1))
		}
	}

	if rb.AckHook != nil {
		rb.AckHook(ackSeqNum, recvBitmap)
	}

	return ackSeqNum, recvBitmap
}

// GenerateNack returns the missing sequences within the reorder window and
// invokes NackHook if set. maxMissing limits the returned list size (0 = no limit).
func (rb *RecvBuffer) GenerateNack(maxMissing int) []uint32 {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.pending) == 0 {
		return nil
	}

	// Find the highest buffered sequence
	var highest uint32
	for seq := range rb.pending {
		if seq > highest {
			highest = seq
		}
	}

	// Collect seqs in range (lastDelivered, highest) that are neither buffered
	// (already seen and pending delivery) nor in received (duplicate-guard set).
	// Such seqs have not arrived at all: they are the true holes.
	var missing []uint32
	for seq := rb.lastDelivered + 1; seq < highest; seq++ {
		_, isBuffered := rb.pending[seq]
		alreadySeen := rb.received[seq]
		if !isBuffered && !alreadySeen {
			missing = append(missing, seq)
			if maxMissing > 0 && len(missing) >= maxMissing {
				break
			}
		}
	}

	if len(missing) > 0 && rb.NackHook != nil {
		rb.NackHook(missing)
	}

	return missing
}

// LastDelivered returns the last contiguously delivered sequence number.
func (rb *RecvBuffer) LastDelivered() uint32 {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.lastDelivered
}

// PendingCount returns the number of buffered out-of-order packets.
func (rb *RecvBuffer) PendingCount() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return len(rb.pending)
}

// Stats returns receive buffer statistics.
func (rb *RecvBuffer) Stats() (received, duplicates, dropped uint64, pending int) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.TotalReceived, rb.TotalDuplicates, rb.TotalDropped, len(rb.pending)
}
