# Phase Notes

## Phase 0 - Program Setup and Baseline

**Completion Date:** 2026-04-15

### Summary

Phase 0 establishes the foundational infrastructure for the Hybrid Tunnel integration project. This phase sets up:
- Standardized documentation structure
- Root-level build, test, and lint automation
- Baseline report template for standalone projects
- Decision log and phase tracking

### Tasks Completed

- [x] Create branch strategy and naming convention (`phase-X/*`, `hotfix/*`)
- [x] Add top-level docs:
  - [x] `docs/architecture/current-state.md`
  - [x] `docs/architecture/target-state.md`
  - [x] `docs/testing/test-matrix.md`
  - [x] `DECISIONS.md`
- [x] Add Make targets/scripts for repeatable checks:
  - [x] build both projects
  - [x] run unit tests
  - [x] run lint/static checks
- [ ] Record baseline performance on both standalone projects:
  - [ ] throughput up/down
  - [ ] p50/p95 latency
  - [ ] loss behavior
  - [ ] memory and goroutine counts
- [x] Baseline report exists in `docs/testing/baseline.md`
- [x] Reproducible local CI commands documented

### Key Decisions

1. **P0-D01: Branch Strategy** — Adopted `phase-X/*` naming with protected main branch.
2. **P0-D02: Build/Test Commands** — Standardized on Go toolchain; documented in root Makefile.
3. **P0-D03: Documentation Structure** — Established `docs/{architecture,protocol,testing,ops,deploy}` hierarchy.
4. **P0-D04: Baseline Scope** — Loopback-only measurements for throughput/latency; controlled-loss testing for reliability.

See `DECISIONS.md` for full details.

### Verification Status

- **Implemented:**
  - Root `Makefile` with build/test/lint/vet targets
  - Phase 0 documentation set (`docs/architecture/*`, `docs/testing/*`, `DECISIONS.md`)
  - Baseline report structure in `docs/testing/baseline.md`

- **Pending verification in this workspace session:**
  - Running `make build`
  - Running `make test`
  - Running `make lint`
  - Populating baseline report numeric measurements

### Deviations from Roadmap

No structural deviation. Performance numbers are still pending execution and measurement capture.

### Notes for Phase 1

Phase 1 will begin by defining the hybrid bridge contract interfaces (`internal/hybridbridge`). This requires:
- Canonical ID types (HybridSessionID, HybridStreamID, DownSeq, KeyEpoch)
- Control-plane frame contracts
- Downstream frame header format
- Protocol versioning strategy

All implementations in Phase 1+ must maintain binary compatibility backward unless explicitly versioned and migrated.

---

## Phase 1 - Unified Architecture and Contracts

**Completion Date:** 2026-04-16

### Summary

Phase 1 establishes strict contracts before runtime wiring. It introduces canonical IDs,
control-plane frame contracts, downstream frame header contracts, and version/feature
compatibility behavior in both code stubs and docs.

### Deliverables

- `internal/hybridbridge/types.go`
  - Canonical IDs: `HybridSessionID`, `HybridStreamID`, `DownSeq`, `KeyEpoch`
  - Versioning: `ProtocolVersion`, `FeatureFlags`
  - Control frame contracts and frame stubs:
    - stream open/ack/close/reset
    - downstream ACK/NACK feedback
    - stats and heartbeat
    - key-rotation signal
  - Downstream frame header contract

- `docs/protocol/hybrid-control.md`
  - Frame type catalog
  - Canonical ID mapping
  - Version/feature compatibility strategy

- `docs/protocol/hybrid-downstream.md`
  - Header field contract and byte layout
  - Serialization rules and compatibility behavior

### Notes

- This phase intentionally defines contracts only (no packet parse/build integration yet).
- Runtime behavior and transport integration begin in Phase 2.

---

## Phase 2+ (Pending)

Notes for subsequent phases will be added as work completes.

---

## Phase 2 - MasterDNS Control-Plane Extension (In Progress)

**Last Update:** 2026-04-16

### Completed in this update

- Added hybrid control packet enum values and packet-name mappings:
  - `MasterDNS/internal/enums/dns.go`
  - `MasterDNS/internal/enums/dns_names.go`

- Extended `vpnproto` packet behavior for hybrid packet types:
  - Updated `buildPacketFlags()` in `MasterDNS/internal/vpnproto/parser.go`
    to classify hybrid packet types under valid and stream+sequence sets.
  - Extended packed-control eligibility for hybrid ACK/NACK controls in
    `MasterDNS/internal/vpnproto/packing.go`.

- Integrated hybrid control routing in client/server paths:
  - Client registration in `MasterDNS/internal/client/handlers/stream_handlers.go`
  - Server post-session dispatch acceptance in
    `MasterDNS/internal/udpserver/server_postsession.go`
  - Stream-creation and missing-stream handling adjustments for
    `PACKET_HYBRID_STREAM_OPEN`.

- Added hybrid ACK/close semantics and packet priorities:
  - `MasterDNS/internal/enums/packet_ack.go`
  - `MasterDNS/internal/enums/packet_priority.go`

- Added/extended tests for hybrid behavior:
  - `MasterDNS/internal/vpnproto/parser_test.go`
  - `MasterDNS/internal/vpnproto/packing_test.go`
  - `MasterDNS/internal/enums/packet_ack_test.go`
  - `MasterDNS/internal/enums/packet_priority_test.go`

### Remaining for Phase 2

- None. Phase 2 is complete.

### Session capability negotiation implemented

- `SESSION_INIT` now supports an optional hybrid capability extension block:
  - hybrid supported flag
  - max feedback rate
  - max stream count
- Server parses capability offers and negotiates bounded values during
  `handleSessionInitRequest`.
- `SESSION_ACCEPT` now returns negotiated capability values as an optional
  extension block (while preserving legacy payload compatibility).
- Client consumes negotiated values from `SESSION_ACCEPT` and stores them in
  runtime session state.
- **Bug fix:** hybrid capability negotiation now correctly handles unconfigured
  server limits (`MaxAllowedClientActiveStreams=0`, `ClientMaxPacketsPerBatch=0`)
  by treating zero as "no server-side cap" rather than a hard zero limit.

### Spoof protocol extension (alignment prep)

- Added backward-compatible hybrid capability codecs to
  `spoof-tunnel/internal/protocol/packet.go`.
- Added optional INIT payload extension helpers for capability metadata:
  - `NewInitPacketWithHybridCapabilities`
  - `ParseInitWithHybridCapabilities`
- Added protocol unit tests in
  `spoof-tunnel/internal/protocol/packet_test.go` for:
  - capability codec roundtrip
  - INIT payload with capabilities
  - legacy INIT payload without capabilities

### Spoof runtime handshake integration

- Client INIT send paths in `spoof-tunnel/internal/tunnel/client.go` now use
  `NewInitPacketWithHybridCapabilities` to advertise capabilities.
- Server INIT handling in `spoof-tunnel/internal/tunnel/server.go` now uses
  `ParseInitWithHybridCapabilities` with graceful rejection on malformed
  capability extension payloads.
- Server session state now records client-offered hybrid capabilities for
  future runtime policy integration.

### Validation Status

- All tests pass (`make test` and `make lint`).

---

## Phase 3 - Spoof-Tunnel Downstream Refactor (Production Reliability)

**Completion Date:** 2026-04-16

### Summary

Phase 3 replaces the simplistic `RecvBuffer` and `SendBuffer` in spoof-tunnel with
production-grade implementations that provide true reassembly, dynamic retransmit
timeouts, retry limits, and upstream tunneling integration hooks.

### Deliverables

- **`spoof-tunnel/internal/tunnel/reliability.go`** — Complete rewrite:

  - **RecvBuffer** (upgraded, backward-compatible API):
    - True out-of-order reassembly: buffered packet map + contiguous delivery cursor
    - Gap fill triggers flush of all consecutive buffered packets in order
    - Duplicate suppression: entries removed from `received` map once delivered
    - Bounded memory: `maxReorderSlots` (default 256) prevents unbounded growth
    - ACK/NACK hook fields (`AckHook`, `NackHook`) for upstream tunneling
      integration — nil by default for standalone operation
    - `PendingCount()`, `Stats()` for observability

  - **SendBuffer** (upgraded, backward-compatible API):
    - Dynamic RTO via RFC 6298 SRTT/RTTVAR EWMA estimator
    - Karn's algorithm: RTT sampled only from first-transmit packets
    - Sub-granule RTT filter: samples below 1ms (clock noise) are ignored
    - Exponential backoff: `effectiveRTO = rto * 2^retransmits`
    - Per-packet retry limit (`maxRetries`, default 10): exhausted packets dropped
    - `Stats()` for observability (sent, retransmits, dropped, pending)

- **`spoof-tunnel/internal/tunnel/reliability_test.go`** — 18 new tests:
  - In-order delivery
  - Out-of-order reassembly and gap flush
  - Duplicate suppression (in-flight and already-delivered)
  - Memory bounds enforcement
  - ACK generation with selective bitmap
  - NACK generation and hooks
  - SendBuffer window, ACK, selective-ACK
  - Retransmit with exponential backoff
  - Retry limit enforcement
  - Dynamic RTO convergence (SRTT)
  - Karn's algorithm
  - Stats counters

### Compatibility

- All existing call sites (`server.go`, `client.go`) use the unchanged `NewRecvBuffer` /
  `NewSendBuffer` / `RecvBuffer` / `SendBuffer` API without modification.
- `AckHook` and `NackHook` are nil by default; standalone spoof operation is unaffected.

### Validation Status

- All tests pass (`make test` and `make lint`).

---

## Phase 4+ (Pending)

Notes for subsequent phases will be added as work completes.
