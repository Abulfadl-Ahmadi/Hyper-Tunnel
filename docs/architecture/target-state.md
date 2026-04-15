# Target State Architecture

## Overview

This document describes the intended final architecture after completing all phases (Phase 0–11).

---

## High-Level Design

### Asymmetric Hybrid Transport

```
┌─────────────────────┐
│   End User (Iran)   │
└──────────┬──────────┘
           │
           ▼ (SOCKS5)
┌──────────────────────────────┐
│   Iran Relay Node            │
│  (Hybrid Bridge Manager)     │
│  ┌────────────────────┐      │
│  │ MasterDNS Client   │ ─┐   │
│  └────────────────────┘  │   │
│  ┌────────────────────┐  │   │
│  │ Spoof-Tunnel Recv  │  │   │
│  └────────────────────┘  │   │
└──────────┬──────────────────┘
           │ (DNS upstream)
           │ (Raw packets downstream)
           ▼
┌──────────────────────────────┐
│  Foreign Server Node         │
│  (Hybrid Bridge Manager)     │
│  ┌────────────────────┐      │
│  │ MasterDNS Server   │ ─┐   │
│  └────────────────────┘  │   │
│  ┌────────────────────┐  │   │
│  │ Spoof-Tunnel Send  │  │   │
│  └────────────────────┘  │   │
└──────────┬──────────────────┘
           │
           ▼ (TCP)
┌──────────────────────┐
│   Target Server      │
│   (V2Ray / X-UI)     │
└──────────────────────┘
```

### Data Path

1. **Upstream (Control + Requests):**
   - Client app → Iran relay SOCKS5
   - Iran relay → MasterDNS client (DNS queries)
   - Foreign server MasterDNS server → Target server
   - ACKs return via same path

2. **Downstream (Bulk Responses):**
   - Target server → Foreign relay Spoof-Tunnel send
   - Spoofed raw packets → Iran relay Spoof-Tunnel recv
   - Reassembled data → Client app
   - ACKs tunnel back to foreign relay via MasterDNS

---

## Core Components

### 1. Hybrid Bridge Manager

New package: `internal/hybridbridge/` (added in Phase 4)

Responsibilities:
- Session lifecycle management (open, active, draining, closed, reset)
- Stream ID mapping (MasterDNS ↔ Spoof-Tunnel)
- Scheduler loop: fair distribution across streams
- Retransmit loop: exponential backoff with per-stream priority
- ACK flush loop: batch upstream feedback
- Metrics loop: correlation of upstream/downstream stats

### 2. Enhanced MasterDNS

Phase 2 additions:
- New packet types for hybrid control frames
- Capability negotiation during SESSION_INIT
- Carrying downstream ACK/NACK payloads in DNS responses
- Session-level metadata (max streams, feedback rate, epochs)

### 3. Enhanced Spoof-Tunnel

Phase 3+ additions:
- True reassembly buffer (ring + contiguous cursor)
- Explicit ACK/NACK generation hooks
- Per-stream retransmit state machine
- Memory bounds enforcement
- Flow control credits (added Phase 5)
- Adaptive FEC (Phase 6)
- Replay window + epoch tracking (Phase 7)

### 4. Observability Layer

Phase 8 additions:
- Structured logging with session/stream/seq correlation IDs
- Prometheus metrics endpoint
- Health checks and readiness probes
- Runbooks for incident response

---

## Protocol Contracts

### Canonical Types (Phase 1)

```
HybridSessionID    : uint32  (64K sessions max)
HybridStreamID     : uint32  (4B streams per session)
DownSeq            : uint64  (downstream sequence)
KeyEpoch           : uint16  (256 epochs per session)
```

### Control-Plane Frames (Phase 1–2)

Carried over MasterDNS; defines:
- STREAM_OPEN / STREAM_OPEN_ACK
- STREAM_CLOSE / STREAM_RESET
- ACK / NACK (feedback for downstream)
- HEARTBEAT
- KEY_ROTATE signal
- STATS (latency, loss, reorder)

### Downstream Frame Header (Phase 1–3)

Spoof packet wrapper:
- [KeyEpoch:2] [HybridSessionID:4] [HybridStreamID:4] [DownSeq:8] [Flags:1] [Payload:…]
- Total header: 19 bytes (optimizable in Phase 7)

---

## Flow Control & Fairness (Phase 5)

### Per-Stream Credits

Each stream has:
- Send window: max bytes in flight
- Receive window: max reorder buffer

### Fair Scheduling

- Deficit Round-Robin (DRR) across active streams
- Prevent head-of-line blocking
- Backpressure: if MasterDNS ACK queue saturates, throttle Spoof sender

---

## Loss Adaptation & FEC (Phase 6)

### Adaptive FEC Policy

- Monitor EWMA loss rate on downstream
- Adjust parity shards based on loss (0–40%)
- Fallback to no FEC if link is clean
- Bounded by configured overhead limit

---

## Security Hardening (Phase 7)

### Replay Protection

- Per-stream replay window (64-bit bitmap)
- Sequence number checks
- Epoch-based key rotation with overlap window

### Crypto Bindings

- AEAD AAD includes: SessionID, StreamID, Seq, Epoch
- Prevents cross-stream or cross-epoch packet injection

---

## Observability & Operations (Phase 8)

### Metrics Exported

- Active sessions/streams
- RTT, loss rate, retransmit counts
- Queue depths (upstream ACK, downstream retransmit)
- Memory (RSS, goroutine count)
- FEC efficiency (recovery success %)

### Runbooks

- Incident response playbooks
- Packet loss spike diagnosis
- Congestion collapse handling
- Session hang detection

---

## Deployment Modes

### Mode 1: Full Hybrid (default)

Both MasterDNS and Spoof-Tunnel active; asymmetric multiplexed transport.

### Mode 2: MasterDNS-Only (fallback)

If Spoof-Tunnel unavailable; all traffic (down + up) via DNS.

### Mode 3: Spoof-Only (standalone)

Spoof-Tunnel alone; upstream must be local or via another tunnel.

---

## Integration Milestones

| Phase | Deliverable | Acceptance |
|-------|-------------|-----------|
| 0 | Baseline infra | Build/test/lint working |
| 1 | Protocol contracts | Interfaces compile |
| 2 | MasterDNS extensions | Control frames roundtrip |
| 3 | Spoof reassembly | Loss/reorder tests pass |
| 4 | Bridge core | Single stream end-to-end |
| 5 | Flow control | Multi-stream fairness |
| 6 | FEC adaptation | Goodput vs. non-FEC proven |
| 7 | Security | Replay/fuzz tests pass |
| 8 | Observability | Metrics + runbooks complete |
| 9 | V2Ray/X-UI | Protocol validation |
| 10 | Performance | SLOs met under load |
| 11 | Release | Version tagged + rollback tested |

---

## Success Criteria (Project Complete)

- ✅ Asymmetric tunnel operational end-to-end in production mode
- ✅ Stable multi-user performance under sustained real traffic
- ✅ No critical memory/goroutine leaks in soak tests
- ✅ Replay and malformed packet protections validated
- ✅ Full observability and runbooks in place
- ✅ X-UI/V2Ray/Trojan traffic stable over hybrid path
- ✅ Release, rollback, and migration plans documented
