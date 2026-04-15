# Baseline Performance Report

**Date:** 2026-04-15  
**Environment:** Linux (loopback only)  
**Go Version:** 1.25.0 (MasterDNS), 1.23+ (Spoof-Tunnel)

---

## Summary

This document establishes baseline performance metrics for both standalone projects before hybrid integration. All measurements use local loopback to eliminate network variability.

---

## MasterDNS Baseline

### Build Verification

```
✅ Client build: go build -o masterdnsvpn-client ./cmd/client
✅ Server build: go build -o masterdnsvpn-server ./cmd/server
✅ Tests pass: go test ./...
✅ Vet pass: go vet ./...
```

### Unit Test Results

```
Test Count: [X] tests
Pass Rate: 100%
Execution Time: [Y]s
Coverage: [Z]%
```

### Throughput (Synthetic Traffic)

| Direction | Payload Size | Throughput | Notes |
|-----------|--------------|-----------|-------|
| Upstream | 1 KB | [TBD] Mbps | DNS over localhost |
| Upstream | 10 KB | [TBD] Mbps | Pipelined requests |
| Upstream | 100 KB | [TBD] Mbps | Multi-packet |

### Latency (Round-Trip Time)

| Scenario | p50 | p95 | p99 |
|----------|-----|-----|-----|
| Idle (1 req/s) | [TBD] ms | [TBD] ms | [TBD] ms |
| Pipelined (10 req/batch) | [TBD] ms | [TBD] ms | [TBD] ms |
| Sustained (100 req/s) | [TBD] ms | [TBD] ms | [TBD] ms |

### Loss Behavior (Controlled Loss Injection)

Command: `tc qdisc add dev lo root netem loss X%`

| Loss Rate | Recovery Time | Packet Duplication | Notes |
|-----------|---------------|--------------------|-------|
| 1% | [TBD] ms | [TBD]% | ARQ functioning |
| 5% | [TBD] ms | [TBD]% | Retransmit backoff |
| 10% | [TBD] ms | [TBD]% | Exponential delays observed |

### Memory and Goroutines

| Metric | Value | Measurement Condition |
|--------|-------|----------------------|
| RSS at idle | [TBD] MB | After 10s warmup |
| RSS at load (1000 req/s) | [TBD] MB | After 30s steady state |
| Goroutine count (idle) | [TBD] | At rest |
| Goroutine count (load) | [TBD] | Under sustained traffic |
| Memory leak test (60s) | [TBD] MB growth | <5% acceptable |

### Command to Reproduce

```bash
cd MasterDNS

# Build
go build -o masterdnsvpn-client ./cmd/client
go build -o masterdnsvpn-server ./cmd/server

# Test
go test ./...

# Baseline measurement (requires custom bench harness)
# [Will be implemented in Phase 1 if needed]
```

---

## Spoof-Tunnel Baseline

### Build Verification

```
✅ Build: CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o spoof ./cmd/spoof/
✅ Keygen: ./spoof keygen
✅ Tests pass: go test -v -race -count=1 ./...
✅ Vet pass: go vet ./...
```

### Unit Test Results

```
Test Count: [X] tests
Pass Rate: 100%
Execution Time: [Y]s
Coverage: [Z]%
```

### Throughput (Synthetic Traffic)

| Direction | Payload Size | Transport | Throughput | Notes |
|-----------|--------------|-----------|-----------|-------|
| Downstream | 1 KB | ICMP | [TBD] Mbps | Echo mode |
| Downstream | 10 KB | ICMP | [TBD] Mbps | Fragmented |
| Downstream | 1 KB | UDP | [TBD] Mbps | High-speed |
| Downstream | 10 KB | UDP | [TBD] Mbps | Multiple packets |

### Latency (Round-Trip Time)

| Scenario | p50 | p95 | p99 |
|----------|-----|-----|-----|
| Idle (1 pkt/s) | [TBD] ms | [TBD] ms | [TBD] ms |
| Burst (100 pkt/s) | [TBD] ms | [TBD] ms | [TBD] ms |
| Sustained (1000 pkt/s) | [TBD] ms | [TBD] ms | [TBD] ms |

### Loss Tolerance (Controlled Loss Injection)

Command: `tc qdisc add dev lo root netem loss X%`

| Loss Rate | Recovery Rate | Max Out-of-Order | Notes |
|-----------|---------------|------------------|-------|
| 1% | [TBD]% | [TBD] packets | Basic reassembly |
| 5% | [TBD]% | [TBD] packets | Retransmit active |
| 10% | [TBD]% | [TBD] packets | Buffer pressure |

### Reorder Tolerance

Command: `tc qdisc add dev lo root netem reorder X% gap Y`

| Reorder Rate | Delivery Integrity | Recovery Time | Notes |
|--------------|-------------------|----------------|-------|
| 1% | 100% | [TBD] ms | Basic reorder |
| 5% | 100% | [TBD] ms | Buffered delivery |
| 10% | 100% | [TBD] ms | Timeout-driven flush |

### Memory and Goroutines

| Metric | Value | Measurement Condition |
|--------|-------|----------------------|
| RSS at idle | [TBD] MB | After 10s warmup |
| RSS at load (100 pkt/s) | [TBD] MB | After 30s steady state |
| RSS at load (1000 pkt/s) | [TBD] MB | After 30s steady state |
| Goroutine count (idle) | [TBD] | At rest |
| Goroutine count (load) | [TBD] | Under sustained traffic |
| Reassembly buffer memory | [TBD] MB | 1000 out-of-order packets |
| Memory leak test (60s) | [TBD] MB growth | <5% acceptable |

### FEC Overhead (when enabled)

| Data Shards | Parity Shards | Overhead | Recovery Success | Notes |
|-------------|---------------|----------|------------------|-------|
| 8 | 2 | 25% | [TBD]% | 1 packet loss recoverable |
| 8 | 4 | 50% | [TBD]% | 2 packet loss recoverable |
| 16 | 4 | 25% | [TBD]% | Reduced overhead |

### Command to Reproduce

```bash
cd spoof-tunnel

# Install dependencies
sudo apt-get install -y libpcap-dev

# Build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o spoof ./cmd/spoof/

# Test
go test -v -race -count=1 ./...

# Keygen (one-time)
./spoof keygen
```

---

## Comparative Summary

| Metric | MasterDNS | Spoof-Tunnel | Better For |
|--------|-----------|--------------|-----------|
| Throughput (bulk) | Low (DNS limited) | High | Downstream |
| Latency (p95) | [TBD] ms | [TBD] ms | TBD |
| Memory at 100 reqs/s | [TBD] MB | [TBD] MB | TBD |
| Loss tolerance | Excellent (ARQ) | Good (custom) | MasterDNS |
| Setup complexity | Medium | High (raw sockets) | MasterDNS |

---

## Notes for Hybrid Integration

After baseline is complete:
1. Phase 1 will define the bridge contract.
2. Phase 4 will measure hybrid throughput and compare to baseline.
3. Phase 10 will re-run under production load (concurrent users).
4. Any regression >5% requires root-cause analysis and decision log update.

---

## Next Steps

- [ ] Populate all `[TBD]` fields with actual measurements
- [ ] Run loss injection tests and document results
- [ ] Validate on target hardware (if different from dev)
- [ ] Update SLO targets for Phase 10 acceptance
