# Test Matrix

## Overview

This document defines the test scope and execution strategy across all phases of the Hybrid Tunnel project.

---

## Test Categories

### Unit Tests

**Scope:** Individual modules and internal APIs  
**Framework:** Go `testing` package  
**Coverage Target:** ≥80% per phase  

Commands:
```bash
cd MasterDNS && go test ./...
cd spoof-tunnel && go test -v -race -count=1 ./...
```

**Running Tests by Phase:**

| Phase | MasterDNS Tests | Spoof-Tunnel Tests | Bridge Tests |
|-------|-----------------|-------------------|--------------|
| 0–1 | DNS parser, codec, crypto | Transport, stream mux | N/A (stubs only) |
| 2 | VPN proto extensions | Reliability layer | Control dispatch |
| 3 | Client/server dispatch | Reassembly buffer, FEC | Session mapping |
| 4 | Full client/server flow | Retransmit + ACK | Bridge loops + state machine |
| 5–11 | Flow control, fairness | Congestion, security | Observability, integration |

---

### Integration Tests

**Scope:** Cross-module interactions, end-to-end flows  
**Environment:** Local loopback or controlled network (tc qdisc)  
**Duration:** < 10s per test (except soak tests)

#### Phase-Gated Integration Tests

**Phase 2:** MasterDNS carries hybrid control frames (no Spoof-Tunnel)
```bash
go test -run TestMasterDNSHybridControlRoundtrip -v
```

**Phase 3:** Spoof-Tunnel handles reorder/loss
```bash
go test -run TestSpoofReassembly_PacketsOutOfOrder -v
go test -run TestSpoofRetransmit_ExceedsMaxRetries -v
```

**Phase 4:** Single-stream asymmetric flow end-to-end
```bash
go test -run TestHybridBridge_SingleStreamFlow -v
```

**Phase 5:** Multi-stream fairness
```bash
go test -run TestHybridBridge_StreamFairness -v
```

**Phase 6:** FEC recovery under loss
```bash
go test -run TestSpoofFEC_RecoveryUnderLoss -v
```

**Phase 7:** Replay rejection + key rotation
```bash
go test -run TestHybridBridge_ReplayRejection -v
go test -run TestHybridBridge_KeyRotation -v
```

**Phase 8:** Metrics export + health checks
```bash
go test -run TestMetricsExport -v
```

---

### Performance & Stress Tests

**Scope:** Throughput, latency, memory stability  
**Environment:** Local loopback + optional packet loss injection  

#### Baseline (Phase 0)

Run before any integration; document in `docs/testing/baseline.md`:
- **MasterDNS standalone:** throughput, p50/p95/p99 latency, memory at rest/under load
- **Spoof-Tunnel standalone:** throughput, latency, loss recovery, memory

#### Regression Tests (Phase 1+)

After each phase, re-run baseline suite; fail if regression > 5%.

#### Load Tests (Phase 10)

Simulate concurrent users:
- 10 concurrent streams, 30s duration
- 50 concurrent streams, 60s duration
- 100 concurrent streams, 2m duration
- 300 concurrent streams, 5m duration

Measure:
- Aggregate throughput
- Per-stream p95 latency
- Packet loss / retransmit rate
- Memory growth

#### Soak Tests (Phase 10)

Long-running stability tests:
- 8-hour soak with sustained mixed load
- 24-hour soak with idle/burst cycles
- 72-hour soak for production validation

Fail criteria:
- Memory growth > 10% per hour
- Goroutine count increase > 100
- Session drop rate > 0.1%

---

### Fuzz Tests

**Scope:** Parser robustness, malformed input handling  
**Framework:** Go `testing/fuzz` (1.18+)  

**Phase 7 Targets:**
- DNS packet parser
- Hybrid control frame decoder
- Downstream frame decoder
- Encryption/decryption with corrupted input

Commands:
```bash
go test -fuzz FuzzDNSParser -fuzztime 30s
go test -fuzz FuzzHybridFrameDecoder -fuzztime 30s
go test -fuzz FuzzDownstreamDecoder -fuzztime 30s
```

---

### Protocol Compliance Tests

**Scope:** Spec adherence, versioning, backward compatibility  

**Phase 1:** Protocol version byte, feature flags
```bash
go test -run TestProtocolVersion -v
go test -run TestFeatureFlags -v
```

**Phase 2:** MasterDNS extensions (new packet types)
```bash
go test -run TestMasterDNSPacketRoundtrip -v
```

**Phase 3:** Spoof frame format (header layout)
```bash
go test -run TestSpoofFrameFormat -v
```

**Phase 7:** Backward compatibility under migration
```bash
go test -run TestMigrationCompat -v
```

---

### Operational Tests

**Scope:** Runbook validation, incident scenarios  

**Phase 8+:**
- Graceful shutdown
- Reconnect after transient network outage
- Recovery from packet loss spike
- Key rotation without session drop
- Metrics collection + export accuracy

---

## Local CI Commands

Run these locally to validate before push:

```bash
# Full baseline
make baseline

# Unit + integration (Phase 0–2)
make test

# Lint and vet
make lint vet

# Full build + test (all projects)
make all
```

See `Makefile` at workspace root for details.

---

## CI/CD Matrix

| Trigger | Jobs | Timeout |
|---------|------|---------|
| Push to `phase-X/*` | Build + unit + integration | 15m |
| Push to `main` | Build + unit + integration + fuzz (30s) | 30m |
| Release tag | Build + all tests + soak (1h) | 2h |

---

## Test Data and Fixtures

Location: `tests/fixtures/`

- DNS packets (valid, malformed)
- Crypto keys (test vectors)
- Loss/reorder patterns (pcap traces)
- Config samples

---

## Continuous Metrics

After Phase 8, tracked metrics:

| Metric | Target | Phase |
|--------|--------|-------|
| Test coverage | ≥80% | All |
| Lint warnings | 0 | All |
| Fuzz crash-free | Yes | 7+ |
| Memory leak free | Yes | 8+ |
| p95 latency (idle) | <100ms | 10+ |
| p95 latency (100 streams) | <500ms | 10+ |
| Loss recovery rate | >99% | 3–10 |
| FEC overhead | <20% | 6–10 |

---

## Acceptance Criteria per Phase

Each phase ends when:
1. All unit tests pass
2. Phase-specific integration tests pass
3. No new lint warnings introduced
4. Regression tests show <5% throughput/latency change
5. PHASE_NOTES.md and DECISIONS.md updated
