# Current State Architecture

## Overview

This document describes the current standalone state of both projects before integration.

---

## MasterDNS (Upstream Transport)

### Purpose
DNS-based upstream tunnel carrying TCP traffic through DNS queries and responses with custom ARQ retransmission logic.

### Architecture Highlights

- **Protocol**: Custom DNS tunneling with ARQ (Automatic Repeat reQuest)
- **Encryption**: AES / ChaCha20 / XOR
- **Transport Overhead**: 5–7 bytes per packet (~88% lower than DNSTT)
- **Session Model**: Single master session with optional multi-resolver support
- **Multiplexing**: SOCKS5 streams via SOCKS protocol
- **MTU Handling**: Adaptive MTU discovery and synchronization
- **Reliability**: Packed control blocks, request packing, optional compression

### Key Modules

- `internal/client/` — Client-side session, tunnel runtime, stream handling
- `internal/server/` — Server-side DNS listener, dispatch, response building
- `internal/dnsparser/` — DNS parsing and serialization
- `internal/vpnproto/` — Custom VPN protocol codec
- `internal/compression/` — Compression layer (LZ4, optional)
- `internal/security/` — Encryption and key management
- `internal/config/` — Configuration parsing (TOML)
- `cmd/client/`, `cmd/server/` — Entry points

### Data Flow

```
Client App
    ↓
SOCKS5 Listener (localhost:18000)
    ↓
Session Manager (multiplexing)
    ↓
Tunnel Encoder (custom protocol)
    ↓
Encryption (ChaCha20/AES)
    ↓
DNS Queries (via configured resolvers)
    ↓
Server DNS Listener (port 53)
    ↓
DNS Parser → Decoder → Session Dispatcher
    ↓
Target Server
```

### Current Capabilities

- ✅ Reliable upstream over DNS
- ✅ SOCKS5 proxy
- ✅ Multi-resolver balancing
- ✅ Adaptive MTU discovery
- ✅ ARQ retransmission
- ✅ Session persistence

### Limitations (for hybrid use)

- Asymmetric: response traffic must also traverse DNS
- Cannot directly carry downstream packets
- No explicit flow control or fairness across streams
- No integrated feedback loop for downstream ACKs

---

## Spoof-Tunnel (Downstream Transport)

### Purpose
Layer 3/Layer 4 high-speed downstream packet delivery using mutual IP spoofing and custom reliability layer.

### Architecture Highlights

- **Protocol**: ICMP Echo or UDP with mutual IP spoofing
- **Encryption**: ChaCha20-Poly1305 AEAD
- **Transport Overhead**: Minimal (IP/ICMP/UDP headers only)
- **Session Model**: Single master session per endpoint pair
- **Multiplexing**: Stream IDs within session
- **Reliability Layer**: Custom TCP-like ACK/NACK, retransmission, out-of-order buffering
- **Raw Sockets**: Kernel bypass via raw sockets + BPF filtering

### Key Modules

- `internal/transport/` — ICMP/UDP send/recv, spoofing logic
- `internal/protocol/` — Session init, frame format, serialization
- `internal/socks/` — SOCKS5 proxy acceptance and forwarding
- `internal/tunnel/` — Stream multiplexing, lifecycle
- `internal/crypto/` — ChaCha20-Poly1305 AEAD
- `internal/fec/` — Reed-Solomon FEC (optional)
- `cmd/spoof/` — Entry point

### Data Flow

```
SOCKS5 Listener (localhost:1080)
    ↓
Stream Multiplexer (StreamID)
    ↓
Reliability Layer (seq, ack, retransmit)
    ↓
Encryption (ChaCha20-Poly1305)
    ↓
Raw Socket Send (spoofed IP)
    ↓
Network [Loss / Reorder / Delay]
    ↓
Raw Socket Recv (BPF filter)
    ↓
Decryption & Reassembly
    ↓
Target App
```

### Current Capabilities

- ✅ High-speed downstream via IP spoofing
- ✅ ICMP and UDP transports
- ✅ Stream multiplexing (StreamID)
- ✅ Custom retransmission with exponential backoff
- ✅ Out-of-order packet buffering
- ✅ Optional FEC (Reed-Solomon)
- ✅ SOCKS5 proxy

### Limitations (for hybrid use)

- Downstream-only (no upstream capability)
- Simple reassembly (no contiguous delivery guarantee in all cases)
- No explicit session-level flow control
- No per-stream credits or fairness scheduling
- Cannot carry control signals back to MasterDNS

---

## Integration Gap

### What Works Independently

| Feature | MasterDNS | Spoof-Tunnel |
|---------|-----------|--------------|
| Upstream delivery | ✅ | ❌ |
| Downstream delivery | ❌ | ✅ |
| Encryption | ✅ | ✅ |
| Multiplexing | ✅ | ✅ |
| Reliability | ✅ (ARQ) | ✅ (custom) |
| Flow control | 🟡 (basic) | 🟡 (basic) |

### What's Missing

1. **Bidirectional session mapping** — MasterDNS stream ID ↔ Spoof StreamID
2. **Feedback tunnel** — Downstream ACKs must route back via MasterDNS
3. **Unified flow control** — No per-session or per-stream fairness across both paths
4. **Congestion coordination** — Spoof sender unaware of MasterDNS backpressure
5. **Shared key rotation** — No coordinated key epoch management
6. **Unified metrics** — No correlation of upstream latency with downstream loss

---

## Performance Baseline

See `docs/testing/baseline.md` for detailed standalone measurements.

### High-Level Summary (Local Loopback)

- **MasterDNS** throughput: ~Mbps range (see baseline)
- **Spoof-Tunnel** throughput: ~Mbps range (see baseline)
- **MasterDNS** p95 latency: ~ms (see baseline)
- **Spoof-Tunnel** p95 latency: ~ms (see baseline)

---

## Next Phase: Target State

See `docs/architecture/target-state.md` for post-integration design.
