# Hybrid Tunnel Integration Roadmap

## Full Project Path

- `/mnt/e/projects/ip-spoof_2_masterDNS`

## Main Goal

Build a production-grade asymmetric hybrid transport by combining:

- **MasterDNS** for upstream/control (`client -> Iran relay -> abroad server`)
- **Spoof-Tunnel** for high-speed downstream (`abroad server -> Iran relay -> client`)

Then run **X-UI / V2Ray / Trojan and related protocols** on top, with stable multi-user operation.

---

## Execution Rules (for all phases)

- Complete phases in order; do not skip dependencies.
- Every phase must end with:
  - code review checklist complete
  - tests executed (unit/integration where applicable)
  - `PHASE_NOTES.md` update with decisions and deviations
- Keep changes small and mergeable per phase.
- No protocol changes without versioning and migration notes.
- For any ambiguity, define it in `DECISIONS.md` before coding.

---

## Phase 0 - Program Setup and Baseline

### Objective

Create a controlled implementation environment and baseline metrics before touching protocol logic.

### Tasks

- [x] Create branch strategy and naming convention (`phase-X/*`, `hotfix/*`).
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

### Done Criteria

- [x] Baseline report exists in `docs/testing/baseline.md`
- [x] Reproducible local CI commands documented

---

## Phase 1 - Unified Architecture and Contracts

### Objective

Define strict interfaces and protocol contracts before implementation.

### Tasks

- [ ] Create `internal/hybridbridge` design package (types/interfaces only first).
- [ ] Define canonical IDs:
  - [ ] `HybridSessionID` (32-bit)
  - [ ] `HybridStreamID` (32-bit)
  - [ ] `DownSeq` (64-bit)
  - [ ] `KeyEpoch` (16-bit)
- [ ] Define control-plane frame contracts (carried by MasterDNS):
  - [ ] stream open/ack/close/reset
  - [ ] downstream ACK/NACK feedback
  - [ ] stats and heartbeat
  - [ ] key-rotation signal
- [ ] Define downstream spoof frame header and serialization format.
- [ ] Add versioning strategy:
  - [ ] protocol version byte
  - [ ] feature flags
  - [ ] backward compatibility behavior

### Done Criteria

- [ ] `docs/protocol/hybrid-control.md` complete
- [ ] `docs/protocol/hybrid-downstream.md` complete
- [ ] compile-safe interface stubs added

---

## Phase 2 - MasterDNS Control-Plane Extension

### Objective

Extend MasterDNS to carry hybrid control and downstream feedback safely.

### Tasks

- [ ] Add new packet enums for hybrid control.
- [ ] Extend `vpnproto` parsing/building for hybrid control payloads.
- [ ] Integrate control handlers into client and server dispatch paths.
- [ ] Add control packet packing compatibility in packed-control blocks path.
- [ ] Add session capability negotiation during `SESSION_INIT/SESSION_ACCEPT`:
  - [ ] hybrid supported?
  - [ ] max feedback rate
  - [ ] max stream counts

### Done Criteria

- [ ] Unit tests for new packet parse/build/roundtrip
- [ ] Client/server can exchange hybrid control frames over DNS only

---

## Phase 3 - Spoof-Tunnel Downstream Refactor (Production Reliability)

### Objective

Upgrade spoof downstream reliability to production behavior under reorder/loss.

### Tasks

- [ ] Replace simplistic receive tracking with true reassembly buffer:
  - [ ] out-of-order map/ring
  - [ ] contiguous delivery cursor
  - [ ] duplicate suppression
- [ ] Improve send buffer/retransmit:
  - [ ] dynamic RTO
  - [ ] retry limits per packet and per stream
  - [ ] retransmit prioritization
- [ ] Implement explicit ACK/NACK generation hooks for upstream tunneling.
- [ ] Add memory bounds:
  - [ ] max reorder slots
  - [ ] max in-flight bytes
  - [ ] eviction and fail-safe rules
- [ ] Keep compatibility mode for standalone spoof deployment until full migration.

### Done Criteria

- [ ] Loss/reorder integration tests pass
- [ ] No unbounded memory growth in stress tests

---

## Phase 4 - Bridge Core Implementation (Abroad + Iran Relays)

### Objective

Implement `hybridbridge` runtime and connect both sides.

### Tasks

- [ ] Implement `Bridge` manager with loops:
  - [ ] `controlRxLoop`
  - [ ] `downRxLoop`
  - [ ] `schedulerLoop`
  - [ ] `ackFlushLoop`
  - [ ] `retransmitLoop`
  - [ ] `metricsLoop`
- [ ] Implement stream lifecycle state machine:
  - [ ] opening, active, draining, closed, reset
- [ ] Implement session/stream mapping:
  - [ ] MasterDNS stream ID <-> Hybrid stream ID
  - [ ] spoof flow association
- [ ] Implement ACK/NACK feedback tunnel:
  - [ ] downlink receive stats -> MasterDNS control frames
- [ ] Implement graceful and forced teardown semantics.

### Done Criteria

- [ ] Single-stream end-to-end asymmetric flow works reliably
- [ ] No panic/leak in stop/start loops

---

## Phase 5 - Congestion, Flow Control, and Fairness

### Objective

Support many simultaneous long-lived users without starvation or collapse.

### Tasks

- [ ] Add per-stream and per-session flow control credits.
- [ ] Add fair scheduling (DRR/WFQ) across streams.
- [ ] Add adaptive pacing for downstream spoof send path.
- [ ] Add backpressure from MasterDNS control queue saturation to spoof sender.
- [ ] Add anti-HoL behavior across streams.

### Done Criteria

- [ ] Multi-stream fairness tests pass
- [ ] Throughput remains stable under mixed workloads

---

## Phase 6 - FEC and Loss Adaptation

### Objective

Improve downstream resilience in high-loss conditions without wasting bandwidth.

### Tasks

- [ ] Add adaptive FEC policy:
  - [ ] based on EWMA loss/reorder
  - [ ] bounded parity ratio
- [ ] Add policy fallback (disable FEC if clean link).
- [ ] Add observability for FEC efficiency:
  - [ ] recovery success rate
  - [ ] overhead ratio

### Done Criteria

- [ ] Controlled-loss tests show improved goodput vs non-FEC
- [ ] FEC overhead bounded by configured limits

---

## Phase 7 - Security Hardening

### Objective

Prevent replay/cross-stream abuse and secure long-lived production sessions.

### Tasks

- [ ] Bind AEAD AAD to session/stream/seq/epoch metadata.
- [ ] Add replay window per stream.
- [ ] Add key epoch rotation:
  - [ ] coordinated via MasterDNS control plane
  - [ ] overlap window for in-flight packets
- [ ] Harden parser limits to prevent memory abuse.
- [ ] Add strict validation for malformed frames and unknown flags.

### Done Criteria

- [ ] Fuzz tests for parser and frame decoder pass
- [ ] Replay tests demonstrate rejection behavior

---

## Phase 8 - Observability and Operations

### Objective

Make production operation debuggable and measurable.

### Tasks

- [ ] Add structured logs with correlation IDs:
  - [ ] session ID
  - [ ] stream ID
  - [ ] seq window stats
- [ ] Export metrics endpoint (Prometheus or equivalent):
  - [ ] active sessions/streams
  - [ ] RTT/loss/retransmit
  - [ ] queue depths
  - [ ] memory and goroutine counters
- [ ] Add health checks and readiness probes.
- [ ] Add runbooks:
  - [ ] incident response
  - [ ] packet loss spikes
  - [ ] congestion collapse handling

### Done Criteria

- [ ] `docs/ops/` runbooks complete
- [ ] Metrics dashboard template committed

---

## Phase 9 - Integration with X-UI / V2Ray / Trojan

### Objective

Validate real protocol behavior over hybrid transport with sustained load.

### Tasks

- [ ] Define deployment topology for Iran relay and abroad relay.
- [ ] Integrate X-UI/V2Ray/Trojan endpoint chaining through hybrid bridge.
- [ ] Validate protocol profiles:
  - [ ] Trojan/TLS
  - [ ] VLESS/VMess
  - [ ] Shadowsocks
  - [ ] gRPC/H2
- [ ] Add config templates for multi-user provisioning.
- [ ] Verify long-lived connections and reconnect behavior.

### Done Criteria

- [ ] Stable multi-user protocol tests documented
- [ ] `docs/deploy/xui-integration.md` complete

---

## Phase 10 - Performance Qualification

### Objective

Prove production readiness with load and soak testing.

### Tasks

- [ ] Create load scenarios:
  - [ ] 10, 50, 100, 300 concurrent users
  - [ ] mixed short/long flows
  - [ ] mixed protocol types
- [ ] Run soak tests (8h, 24h, 72h).
- [ ] Track SLOs:
  - [ ] p95 latency ceiling
  - [ ] retransmit rate
  - [ ] session drop rate
  - [ ] memory growth trend
- [ ] Tune and repeat until target SLO met.

### Done Criteria

- [ ] `docs/testing/perf-qualification.md` complete
- [ ] SLO pass/fail table committed

---

## Phase 11 - Release and Rollout

### Objective

Ship safely with rollback and staged rollout.

### Tasks

- [ ] Add release notes and migration notes.
- [ ] Add compatibility toggles:
  - [ ] `mode=masterdns-only`
  - [ ] `mode=hybrid`
  - [ ] `mode=spoof-only` (if needed)
- [ ] Stage rollout:
  - [ ] canary
  - [ ] small cohort
  - [ ] full rollout
- [ ] Prepare rollback playbook and config toggles.

### Done Criteria

- [ ] tagged release candidate
- [ ] rollback tested in staging

---

## Recommended Task Granularity (for implementation requests)

When requesting implementation, use this format:

- Phase + ticket ID (example: `P4-T03`)
- exact files allowed to change
- expected tests to run
- acceptance criteria

Suggested ticket naming:

- `P1-T01`, `P1-T02`, ...
- `P2-T01`, `P2-T02`, ...

---

## Global Acceptance Criteria (Project Complete)

- [ ] Asymmetric tunnel operational end-to-end in production mode
- [ ] Stable multi-user performance under sustained real traffic
- [ ] No critical memory/goroutine leaks in soak tests
- [ ] Replay and malformed packet protections validated
- [ ] Full observability and runbooks in place
- [ ] X-UI/V2Ray/Trojan traffic stable over hybrid path
- [ ] Release, rollback, and migration plans documented

