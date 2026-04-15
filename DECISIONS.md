# Technical Decisions

## Overview

This document records key architectural, protocol, and implementation decisions made during the Hybrid Tunnel integration project. Decisions are timestamped and linked to the Phase in which they were made.

---

## Phase 0 - Program Setup and Baseline

### P0-D01: Branch Strategy and Naming Convention

**Date:** 2026-04-15  
**Status:** Adopted  
**Scope:** Git workflow

**Decision:**
- Feature/implementation branches: `phase-X/description` (e.g., `phase-1/hybrid-ids`)
- Hotfix branches: `hotfix/issue-description` (e.g., `hotfix/session-leak`)
- Main branch: `main` (protected, requires reviewed PR)
- Each phase has a dedicated branch that collects all Phase N work before merge to main.

**Rationale:**
- Clear phase attribution simplifies rollback and bisection.
- Phase branches enforce staged delivery per roadmap.

**Notes:**
- CI validates tests before merge.

---

### P0-D02: Build and Test Command Standards

**Date:** 2026-04-15  
**Status:** Adopted  
**Scope:** CI/CD, local development

**Decision:**

For **MasterDNS**:
- Build client: `cd MasterDNS && go build -o masterdnsvpn-client ./cmd/client`
- Build server: `cd MasterDNS && go build -o masterdnsvpn-server ./cmd/server`
- Test: `cd MasterDNS && go test ./...`
- Vet: `cd MasterDNS && go vet ./...`

For **Spoof-Tunnel**:
- Build: `cd spoof-tunnel && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o spoof ./cmd/spoof/`
- Test: `cd spoof-tunnel && go test -v -race -count=1 ./...`
- Vet: `cd spoof-tunnel && go vet ./...`

**Rationale:**
- Mirrors existing CI workflows (GitHub Actions).
- Ensures reproducibility across dev and CI.
- Standard Go toolchain commands; no custom scripts required yet.

**Notes:**
- Spoof-Tunnel requires `libpcap-dev` on Linux for raw socket testing.
- Root Makefile wraps these for convenience.

---

### P0-D03: Documentation Structure

**Date:** 2026-04-15  
**Status:** Adopted  
**Scope:** Docs organization

**Decision:**

Toplevel directories:
- `docs/architecture/` — System design and current/target state
- `docs/protocol/` — Protocol specs and frame formats (added in Phase 1+)
- `docs/testing/` — Test matrices, baselines, qualification reports
- `docs/ops/` — Runbooks, observability, debugging (added in Phase 8+)
- `docs/deploy/` — Deployment guides, config templates (added in Phase 9+)

Root-level files:
- `DECISIONS.md` — This file; all major decisions
- `PHASE_NOTES.md` — Phase-by-phase retrospective
- `README.md` — Updated to reflect current state

**Rationale:**
- Scalable, organized structure as project grows.
- Clear separation of concerns.
- Easy navigation for new contributors.

**Notes:**
- Each phase may add new doc dirs as needed.

---

### P0-D04: Baseline Measurement Scope

**Date:** 2026-04-15  
**Status:** Adopted  
**Scope:** Performance testing

**Decision:**

Baseline captures **standalone** project metrics:
- **MasterDNS (upstream)**:
  - Throughput (up/down): measured with synthetic traffic
  - Latency: p50, p95, p99 (RTT)
  - Packet loss behavior: measured under controlled loss
  - Memory: peak RSS, goroutine count at rest and under load
  
- **Spoof-Tunnel (downstream)**:
  - Throughput: up/down under ideal and lossy conditions
  - Latency: p50, p95, p99 (packet RTT)
  - Loss tolerance: recovery rate under reorder/loss
  - Memory: peak RSS, goroutine count
  - FEC overhead: with/without FEC enabled

Conditions:
- Local loopback (no network) for throughput/latency baseline.
- Controlled loss injection (tc qdisc) for loss behavior.
- 30s steady-state measurements after warmup.

**Rationale:**
- Establishes pre-integration baseline to measure Phase 1–10 regression.
- Loopback eliminates network variability.
- Allows later comparison of hybrid vs. standalone performance.

**Notes:**
- Baseline runs documented in `docs/testing/baseline.md`.
- Tools: `go test`, custom benches, `tc` (traffic control), `pprof`.

---

## Phase 1+ (Placeholder)

Decisions for Phase 1 and beyond will be added as implementation progresses.

---

## Decision Template

When adding a new decision, use this format:

```markdown
### P{N}-D{NN}: {Title}

**Date:** YYYY-MM-DD  
**Status:** Proposed | Adopted | Superseded | Rejected  
**Scope:** {area}

**Decision:**
{statement of what was decided}

**Rationale:**
{why this decision was made}

**Alternatives Considered:**
{other options and why they were rejected}

**Notes:**
{implementation details, caveats, follow-up actions}
```
