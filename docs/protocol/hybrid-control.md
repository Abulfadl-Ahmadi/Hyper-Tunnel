# Hybrid Tunnel Control-Plane Protocol

## Overview

This document defines the control-plane frame contracts and canonical ID types for the hybrid tunnel bridge, as implemented in `internal/hybridbridge`.

---

## Canonical ID Types

| Name             | Type    | Description                       |
|------------------|---------|-----------------------------------|
| HybridSessionID  | uint32  | Unique session identifier         |
| HybridStreamID   | uint32  | Unique stream identifier          |
| DownSeq          | uint64  | Downstream sequence number        |
| KeyEpoch         | uint16  | Key epoch for crypto rotation     |

---

## Control-Plane Frame Types

All control-plane frames are carried over MasterDNS. Each frame has a type byte and a frame-specific payload.

| Type Name           | Value | Purpose                                 |
|---------------------|-------|-----------------------------------------|
| FrameStreamOpen     | 0     | Open a new logical stream               |
| FrameStreamOpenAck  | 1     | Acknowledge stream open                 |
| FrameStreamClose    | 2     | Close a logical stream                  |
| FrameStreamReset    | 3     | Reset a logical stream                  |
| FrameDownstreamAck  | 4     | Downstream ACK feedback                 |
| FrameDownstreamNack | 5     | Downstream NACK feedback                |
| FrameStats          | 6     | Stats/metrics/heartbeat                 |
| FrameHeartbeat      | 7     | Keepalive/heartbeat                     |
| FrameKeyRotation    | 8     | Signal key epoch rotation               |

---

## Frame Contracts (Stub)

### StreamOpenFrame
```
struct StreamOpenFrame {
    SessionID: HybridSessionID
    StreamID: HybridStreamID
    // ...additional fields TBD...
}
```

### StreamOpenAckFrame
```
struct StreamOpenAckFrame {
    SessionID: HybridSessionID
    StreamID: HybridStreamID
    // ...additional fields TBD...
}
```

### StreamCloseFrame
```
struct StreamCloseFrame {
    SessionID: HybridSessionID
    StreamID: HybridStreamID
    // ...additional fields TBD...
}
```

### DownstreamAckFrame / DownstreamNackFrame
```
struct DownstreamAckFrame {
    SessionID: HybridSessionID
    StreamID: HybridStreamID
    Seq: DownSeq
    // ...additional fields TBD...
}
```

### StatsFrame
```
struct StatsFrame {
    SessionID: HybridSessionID
    // ...metrics fields TBD...
}
```

### HeartbeatFrame
```
struct HeartbeatFrame {
    SessionID: HybridSessionID
    // ...fields TBD...
}
```

### KeyRotationFrame
```
struct KeyRotationFrame {
    SessionID: HybridSessionID
    NewEpoch: KeyEpoch
    // ...fields TBD...
}
```

---

## Versioning Strategy

- All frames include `ProtocolVersion` (`uint8`), currently `1`
- All frames include `FeatureFlags` (`uint16`) for capability signaling
- Unknown frame types must be ignored for forward compatibility
- Receivers must reject unsupported `ProtocolVersion`

Current feature flags:
- `FeatureDownstreamAckNack`
- `FeatureStatsFrame`
- `FeatureKeyRotation`

---

## Next Steps

- Finalize field layouts and serialization for each frame type
- Implement compile-safe interface stubs in Go
- Add downstream spoof frame header contract in `hybrid-downstream.md`
