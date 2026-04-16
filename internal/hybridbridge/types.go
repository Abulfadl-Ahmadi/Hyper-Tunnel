// Package hybridbridge defines the core types and interfaces for the hybrid tunnel bridge layer.
package hybridbridge

// Canonical ID types

type HybridSessionID uint32 // 32-bit session identifier

type HybridStreamID uint32  // 32-bit stream identifier

type DownSeq uint64         // 64-bit downstream sequence number

type KeyEpoch uint16        // 16-bit key epoch

type ProtocolVersion uint8

const (
	ProtocolVersion1 ProtocolVersion = 1
)

type FeatureFlags uint16

const (
	FeatureDownstreamAckNack FeatureFlags = 1 << iota
	FeatureStatsFrame
	FeatureKeyRotation
)

// Control-plane frame types (contracts)

type ControlFrameType uint8

const (
	FrameStreamOpen ControlFrameType = iota
	FrameStreamOpenAck
	FrameStreamClose
	FrameStreamReset
	FrameDownstreamAck
	FrameDownstreamNack
	FrameStats
	FrameHeartbeat
	FrameKeyRotation
)

// ControlFrame is the interface for all control-plane frames carried by MasterDNS
// (actual serialization defined in protocol doc)
type ControlFrame interface {
	Version() ProtocolVersion
	Type() ControlFrameType
}

// ControlFrameHeader is the shared contract for all control-plane frames.
type ControlFrameHeader struct {
	ProtocolVersion ProtocolVersion
	Features        FeatureFlags
	SessionID       HybridSessionID
}

// StreamOpenFrame requests opening a stream.
type StreamOpenFrame struct {
	Header   ControlFrameHeader
	SessionID HybridSessionID
	StreamID  HybridStreamID
	Target    string
}

func (f *StreamOpenFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *StreamOpenFrame) Type() ControlFrameType { return FrameStreamOpen }

// StreamOpenAckFrame acknowledges stream open.
type StreamOpenAckFrame struct {
	Header    ControlFrameHeader
	SessionID HybridSessionID
	StreamID  HybridStreamID
	Accepted  bool
}

func (f *StreamOpenAckFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *StreamOpenAckFrame) Type() ControlFrameType   { return FrameStreamOpenAck }

// StreamCloseFrame requests graceful close.
type StreamCloseFrame struct {
	Header    ControlFrameHeader
	SessionID HybridSessionID
	StreamID  HybridStreamID
}

func (f *StreamCloseFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *StreamCloseFrame) Type() ControlFrameType   { return FrameStreamClose }

// StreamResetFrame requests forced reset.
type StreamResetFrame struct {
	Header    ControlFrameHeader
	SessionID HybridSessionID
	StreamID  HybridStreamID
	Reason    uint8
}

func (f *StreamResetFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *StreamResetFrame) Type() ControlFrameType   { return FrameStreamReset }

// DownstreamAckFrame sends downstream receive feedback.
type DownstreamAckFrame struct {
	Header    ControlFrameHeader
	SessionID HybridSessionID
	StreamID  HybridStreamID
	AckUntil  DownSeq
}

func (f *DownstreamAckFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *DownstreamAckFrame) Type() ControlFrameType   { return FrameDownstreamAck }

// DownstreamNackFrame reports missing ranges for retransmit scheduling.
type DownstreamNackFrame struct {
	Header     ControlFrameHeader
	SessionID  HybridSessionID
	StreamID   HybridStreamID
	MissingSeq []DownSeq
}

func (f *DownstreamNackFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *DownstreamNackFrame) Type() ControlFrameType   { return FrameDownstreamNack }

// StatsFrame exports per-session and per-stream telemetry.
type StatsFrame struct {
	Header           ControlFrameHeader
	SessionID        HybridSessionID
	RTTMicrosecondsP50 uint32
	RTTMicrosecondsP95 uint32
	LossPermille     uint16
}

func (f *StatsFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *StatsFrame) Type() ControlFrameType   { return FrameStats }

// HeartbeatFrame keeps session liveness.
type HeartbeatFrame struct {
	Header       ControlFrameHeader
	SessionID    HybridSessionID
	UnixMillis   int64
}

func (f *HeartbeatFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *HeartbeatFrame) Type() ControlFrameType   { return FrameHeartbeat }

// KeyRotationFrame coordinates key epoch rollover.
type KeyRotationFrame struct {
	Header        ControlFrameHeader
	SessionID     HybridSessionID
	CurrentEpoch  KeyEpoch
	NextEpoch     KeyEpoch
	ActivateAfter DownSeq
}

func (f *KeyRotationFrame) Version() ProtocolVersion { return f.Header.ProtocolVersion }
func (f *KeyRotationFrame) Type() ControlFrameType   { return FrameKeyRotation }

// Downstream spoof frame header (to be finalized in protocol doc)
type DownstreamFrameHeader struct {
	ProtocolVersion ProtocolVersion
	Features        FeatureFlags
	KeyEpoch        KeyEpoch
	SessionID       HybridSessionID
	StreamID        HybridStreamID
	Seq             DownSeq
	Flags           uint8
}
