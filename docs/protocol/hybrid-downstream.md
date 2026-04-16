# Hybrid Tunnel Downstream Frame Protocol

## Overview

This document defines the downstream spoof frame header and serialization format for the hybrid tunnel, as referenced in `internal/hybridbridge`.

---

## Downstream Frame Header

| Field      | Type              | Description                       |
|------------|-------------------|-----------------------------------|
| Version    | uint8             | Protocol version                  |
| Features   | uint16            | Feature flags                     |
| KeyEpoch   | uint16            | Key epoch for crypto rotation     |
| SessionID  | HybridSessionID   | 32-bit session ID                 |
| StreamID   | HybridStreamID    | 32-bit stream ID                  |
| Seq        | DownSeq           | 64-bit downstream sequence        |
| Flags      | uint8             | Frame flags (TBD)                 |

Total header size: 1 + 2 + 2 + 4 + 4 + 8 + 1 = 22 bytes

---

## Serialization Format

```
struct DownstreamFrameHeader {
    Version: uint8
    Features: uint16
    KeyEpoch: uint16
    SessionID: uint32
    StreamID: uint32
    Seq: uint64
    Flags: uint8
}
// followed by encrypted payload
```

- All fields are encoded in network byte order (big-endian)
- Header is followed by encrypted payload (ChaCha20-Poly1305 AEAD)

---

## Flags (TBD)

- Bit 0: End of Stream
- Bit 1: Retransmit
- Bit 2: FEC parity
- Bit 3–7: Reserved

---

## Versioning

- Header may be extended in future versions
- Unknown flags must be ignored for forward compatibility
- Unsupported versions must be rejected

---

## Next Steps

- Finalize flag definitions
- Implement header encode/decode in Go
- Add test vectors for serialization
