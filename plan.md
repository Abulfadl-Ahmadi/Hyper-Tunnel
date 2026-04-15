• Deep Analysis Result

  - I fully traced both codebases end-to-end: MasterDNS ingress/session/stream ARQ path (MasterDNS/internal/udpserver/server_ingress.go:21, MasterDNS/
    internal/udpserver/server_session.go:24, MasterDNS/internal/udpserver/server_postsession.go:29, MasterDNS/internal/udpserver/server_session.go:297) and
    client runtime/dispatch/ACK path (MasterDNS/internal/client/async_runtime.go:287, MasterDNS/internal/client/dispatcher.go:22, MasterDNS/internal/client/
    async_runtime.go:878, MasterDNS/internal/client/client.go:464).
  - I also traced Spoof-Tunnel transport + session + reliability path (spoof-tunnel/internal/transport/transport.go:8, spoof-tunnel/internal/transport/
    udp.go:37, spoof-tunnel/internal/transport/icmp.go:59, spoof-tunnel/internal/tunnel/server.go:197, spoof-tunnel/internal/tunnel/client.go:369, spoof-
    tunnel/internal/tunnel/reliability.go:46).
  - Your asymmetric idea is technically valid if you separate planes: control/upstream over MasterDNS, bulk downstream over spoof link, with tight session
    coupling and ACK tunneling.
  - Critical observation: Spoof-Tunnel’s current reliability is not production-grade for heavy reordering (its RecvBuffer does not actually buffer out-of-
    order payload delivery robustly) (spoof-tunnel/internal/tunnel/reliability.go:170), so it must be upgraded before hosting many X-UI users.

  Current Protocol Internals (What Matters for Merge)

  - MasterDNS already has strong per-stream ARQ + control ACK machinery and fair scheduling (MasterDNS/internal/arq/arq.go:1474, MasterDNS/internal/arq/
  - MasterDNS server deferred connect model is already strong for async target dialing (MasterDNS/internal/udpserver/server_deferred.go:88, MasterDNS/
    internal/udpserver/server_deferred.go:233); reuse this style for downstream bridge workers.

  Hybrid Architecture (Recommended)

    credits.
  - Use spoof link as data plane (down only): only DOWN_DATA, DOWN_FEC, and minimal DOWN_HEARTBEAT packet classes.
  - Tunnel back all downstream ACK/feedback through MasterDNS upstream as compact control blocks (can leverage packed control behavior conceptually similar
    to PACKET_PACKED_CONTROL_BLOCKS) (MasterDNS/internal/vpnproto/packing.go:13).
  - Separate IDs cleanly:
      - HybridSessionID (32-bit global, maps to MasterDNS session + Spoof key context),
      - HybridStreamID (32-bit, independent from MasterDNS uint16 stream ID, with mapping table),
      - DownSeq (64-bit recommended for long-lived heavy traffic).

  Bridge Module Design (Core of Your Ask)

  - Add a new package in both projects (or new merged repo): internal/hybridbridge.
  - Bridge ingress interface (from MasterDNS control plane):
      - OnUpstreamControl(sessionID, streamID, controlType, payload)
      - OnUpstreamAckFeedback(sessionID, streamID, ackBase, ackBitmap, lossStats)
  - Bridge egress interface (to spoof plane):
      - SendDownstreamChunk(sessionID, streamID, downSeq, flags, payload)
      - SendDownstreamParity(sessionID, fecGroupID, shard...)
  - Per-stream state machine:
      - OPENING -> ACTIVE -> DRAINING -> CLOSED, with forced reset path.
      - Keep bounded reorder buffer + gap map + delivery cursor.
  - ACK strategy:
      - Receiver emits delayed ACK every x ms or y packets.
      - Fast NACK for holes above reorder threshold.
      - ACK payload piggybacked into MasterDNS upstream control.
  - Latency mismatch handling (DNS up, spoof down):
      - Two congestion windows:
          - cwnd_down for spoof send pacing,
          - credit_up for control/ACK return budget.
      - If upstream control is delayed, switch to conservative send mode + stronger FEC ratio.
  - FEC usage:
      - Use adaptive RS only on downstream bursts; tune parity by live loss EWMA (not static config).

  Scalability / Production Requirements (X-UI Load)

  - Replace Spoof-Tunnel RecvBuffer with true ordered delivery ring + out-of-order map + contiguous flush (current implementation is insufficient for high-
    loss reordering) (spoof-tunnel/internal/tunnel/reliability.go:170).
  - Introduce per-session and global memory caps (max in-flight bytes, max reorder slots, max pending retransmits) to prevent OOM under many users.
  - Enforce fair-queuing per stream (DRR/WFQ) to avoid one heavy download starving others.
  - Add backpressure to upstream control channel: if MasterDNS control queue saturates, reduce downstream burst rate and temporarily increase FEC.
  - Add explicit GC timers for dead streams/sessions and closed-stream tombstones (MasterDNS already has patterns you can reuse) (MasterDNS/internal/client/
    client_utils.go:332, MasterDNS/internal/udpserver/session.go:54).

  Exact Code-Level Integration Points

  - MasterDNS server hook points
      - Inbound post-session dispatch for control feedback ingest: MasterDNS/internal/udpserver/server_postsession.go:29.
      - Stream data handling path to branch to hybrid bridge mode: MasterDNS/internal/udpserver/server_postsession.go:893.
      - Deferred connect logic reuse for abroad-side egress workers: MasterDNS/internal/udpserver/server_deferred.go:88.
  - MasterDNS client hook points
      - Outbound scheduler path for injecting bridge control ACK/NACK frames: MasterDNS/internal/client/dispatcher.go:22.
      - Inbound processor to consume hybrid control responses: MasterDNS/internal/client/async_runtime.go:878.
      - Stream packet handler to integrate bridge-driven close/reset semantics: MasterDNS/internal/client/client.go:464.
  - Spoof-Tunnel hook points
      - Server send path for downstream data plane emission: spoof-tunnel/internal/tunnel/server.go:596.
      - Client receive path for downstream ingest + reorder + local delivery: spoof-tunnel/internal/tunnel/client.go:732.
      - Reliability refactor surface: spoof-tunnel/internal/tunnel/reliability.go:46.
      - Packet type extension surface: spoof-tunnel/internal/protocol/packet.go:44.

  Packet/Protocol Changes You Should Make

  - Define new hybrid control packet families in MasterDNS enum space (example):
      - PACKET_HYB_DOWN_ACK, PACKET_HYB_DOWN_NACK, PACKET_HYB_DOWN_STATS, PACKET_HYB_KEY_ROTATE.
  - Extend spoof payload header (inside encrypted payload) with:
      - Version(1) | Flags(1) | HybridSessionID(4) | HybridStreamID(4) | DownSeq(8) | AckEpoch(2) | Len(2).
  - Add key-epoch support for rotation without dropping sessions; sync epoch transitions via MasterDNS control packets.
  - Add integrity context binding: include HybridSessionID, StreamID, and Epoch in AEAD associated data to prevent cross-stream replay.

  Implementation Phases (Practical Roadmap)

  - Phase 1 (Control-plane first): add hybrid control packets in MasterDNS only; no spoof data yet; validate session/stream mapping and lifecycle.
  - Phase 2 (Single-stream downlink): enable spoof downstream for one stream with ACK-over-DNS; verify reorder, retransmit, teardown.
  - Phase 3 (Multi-stream): add fair scheduling and per-stream buffers; run long-lived V2Ray/Trojan tests.
  - Phase 4 (Hardening): adaptive FEC, key rotation, anti-replay windows, observability metrics, resource limits.
  - Phase 5 (Commercial readiness): soak test with dozens/hundreds of concurrent logical users and enforce SLOs (latency, loss recovery, memory per session,
    reconnect behavior).

  If you want, next I can produce a concrete message schema + Go interfaces for hybridbridge (struct definitions, channel topology, and goroutine model) so
  you can start coding immediately.
