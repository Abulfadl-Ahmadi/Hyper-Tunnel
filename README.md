# Hybrid Asymmetric Tunnel  
**MasterDNS (Upstream) + Spoof Downstream Transport**

A high‑performance asymmetric transport layer built by combining DNS‑based upstream tunneling with high‑speed downstream packet delivery.

---

## 🚀 Overview

Hybrid Asymmetric Tunnel is a custom transport layer that merges two independent systems:

- **MasterDNS** → Reliable upstream transport over DNS
- **Spoof‑Tunnel (Downstream Mode)** → High‑speed one‑way downstream delivery

The result is an asymmetric, high‑throughput, multiplexed tunnel capable of carrying real-world VPN traffic and acting as a transport layer for platforms like:

- X‑UI
- V2Ray / Xray
- Trojan
- Shadowsocks
- Other TCP‑based proxy systems

This project is designed as a **production‑grade transport infrastructure**, not just a proof of concept.

---

# 🎯 Design Goals

- ✅ Reliable upstream communication  
- ✅ High‑bandwidth downstream data transfer  
- ✅ Full session multiplexing  
- ✅ Multi‑user support  
- ✅ Long‑lived connection stability  
- ✅ Suitable for VPN workloads  
- ✅ Clean modular Go architecture  
- ✅ Horizontally scalable  

---

# 🏗 High‑Level Architecture

```
                 ┌──────────────────────┐
                 │      End Client      │
                 └──────────┬───────────┘
                            │
                            ▼
                  ┌──────────────────┐
                  │   Iran Relay     │
                  │ (Hybrid Node)    │
                  └───────┬──────────┘
         Upstream (DNS)   │     Downstream (High-Speed)
         via MasterDNS    │
                          ▼
                  ┌──────────────────┐
                  │  Foreign Server  │
                  │ (Core Gateway)   │
                  └──────────────────┘
```

### Direction Split

| Direction | Technology Used |
|-----------|-----------------|
| Client → Foreign | MasterDNS (DNS-based upstream) |
| Foreign → Client | High-speed downstream transport |

---

# 🧠 Core Concepts

## 1️⃣ Asymmetric Transport

Instead of using the same protocol for both directions:

- Upstream traffic is lightweight and reliable.
- Downstream traffic is optimized for throughput.

This dramatically improves overall performance while maintaining stability.

---

## 2️⃣ Session Model

The system operates using:

- **Master Session**
- **Multiple logical Streams**
- Stream ID mapping
- Shared encryption context
- Flow-controlled packet windows

Each user connection (e.g., TCP from V2Ray) becomes a logical stream inside a multiplexed session.

---

## 3️⃣ Bridge Layer

The project introduces a new internal module:

```
bridge/
```

Responsibilities:

- Map upstream sessions to downstream streams
- Handle ACK routing
- Manage retransmissions
- Synchronize session IDs
- Buffer control
- Prevent head-of-line blocking
- Manage multi-user lifecycle

---

# 📦 Project Structure (Proposed)

```
/cmd
    /client
    /relay
    /server

/internal
    /masterdns
    /downstream
    /bridge
    /session
    /crypto
    /mux
    /flow
    /config

/pkg
    /protocol
```

---

# 🔄 End-to-End Data Flow

### ✅ Upstream (Control / Requests)

1. Client sends request
2. Iran relay encodes via MasterDNS
3. Foreign server receives
4. Target server contacted
5. Response prepared for downstream

### ✅ Downstream (Bulk Data)

1. Foreign server packages response
2. Data fragmented into transport frames
3. Frames delivered to Iran relay
4. Relay reassembles stream
5. Data forwarded to client

ACK and control signals return via upstream channel.

---

# 🛠 Building

Requirements:

- Go 1.22+
- Linux (recommended for raw networking features)
- Root privileges may be required depending on transport mode

```bash
git clone https://github.com/yourrepo/hybrid-tunnel
cd hybrid-tunnel
go build ./cmd/...
```

---

# ⚙ Configuration

Example configuration structure:

```yaml
node:
  mode: relay # client | relay | server

masterdns:
  domain: example.com
  key: BASE64_KEY

downstream:
  key: BASE64_KEY
  mtu: 1200

session:
  max_streams: 1024
  window_size: 256
  ack_interval_ms: 100
```

---

# 🌐 Running With X‑UI

Once the tunnel is established:

1. Run Hybrid Tunnel between Iran relay and foreign server.
2. Bind X‑UI / V2Ray to localhost on the foreign server.
3. Configure Hybrid Tunnel to forward traffic to X‑UI port.
4. Users connect normally through their VPN client.

The transport layer becomes transparent to end-users.

---

# 📈 Scalability Design

Designed for:

- Dozens to hundreds of concurrent users
- Long-lived TCP streams
- High downstream burst handling
- Controlled memory usage
- Adaptive flow management
- Clean session cleanup

---

# 🔐 Security Model

- Authenticated encryption for all frames
- Shared session key derivation
- Stream isolation
- Replay protection
- Controlled handshake mechanism

---

# 🧩 Roadmap

- [ ] Core bridge implementation
- [ ] Unified session manager
- [ ] Load balancing support
- [ ] Multi-relay architecture
- [ ] Monitoring & metrics
- [ ] Web management API
- [ ] Horizontal scaling cluster mode

---

# 📊 Performance Targets

| Metric | Target |
|--------|--------|
| Downstream Throughput | Near line speed |
| Upstream Latency | Stable & predictable |
| Packet Loss Recovery | Automatic |
| Multi-User Support | 100+ concurrent sessions |

---

# ⚠ Disclaimer

This software is intended for research and educational purposes.  
Users are responsible for complying with applicable laws and regulations in their jurisdiction.

---

# 🤝 Contributing

Pull requests and architectural discussions are welcome.

---

# 📜 License

MIT License

