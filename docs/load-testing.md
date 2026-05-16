# Load Testing

> Policy written for Milestone 7. Results to be filled in after measurement.

## Hard Rules

- Do not claim capacity without measured results.
- 200 CCU must be measured, not assumed.
- The 15–20 KB/s/user estimate is a planning assumption only.
- Docker Compose is for dev/integration only — not production load testing evidence.
- Debug endpoints and web visualizers are not load test evidence.

---

## Transport Coverage Requirements

The system has two realtime transport adapters. Both must be validated independently and together:

| Transport       | Client type      | Port   |
|-----------------|------------------|--------|
| KCP/UDP         | Unity native     | :9000  |
| WSS/WebSocket   | Unity WebGL      | :9001  |

Load clients must match the transport under test. A KCP-only load test does not validate WebGL behavior. A WebSocket-only test does not validate native KCP behavior.

---

## 1. Native KCP Load Validation

### Load Client

Use a Go KCP load client:

```
loadtest/shared/kcp_client.go
```

The client must:

- Connect via HTTP /join with `client_platform: native`
- Open KCP/UDP session to game-server :9000
- Send Hello/JoinRoom via MessagePack
- Receive FullSnapshot
- Send PlayerInput/PlayerTransform at tick rate
- Send ObjectCommand (lock/release)
- Collect packet-level stats

### Metrics to Capture

```
KCP RTT p50/p95/p99
KCP retransmit count
KCP send queue depth
Server CPU %
Server memory MB
Bandwidth in/out Mbps
Bytes per second per client
Packets per second
Room tick duration ms
Delta build duration ms
Spatial query duration ms
Object lock success/reject rate
Snapshot size bytes
Delta size bytes
GC pause duration
Goroutine count
```

### Scenarios

```
scenario_join:             clients join gradually
scenario_join_storm:       many clients join simultaneously
scenario_200ccu_movement:  200 native clients move randomly
scenario_object_lock:      clients compete for same objects
scenario_packet_loss:      simulate packet loss if supported
scenario_reconnect:        disconnect/reconnect cycle
scenario_idle_cleanup:     clients leave, validate room cleanup
```

### Acceptance Targets (single VPS)

```
50 CCU:
- must pass comfortably

100 CCU:
- CPU < 60%
- no goroutine leak
- p95 update latency acceptable

200 CCU:
- CPU < 75%
- memory stable
- bandwidth < 100 Mbps
- no goroutine leak
- p95 visible update latency acceptable for Unity UX
```

If 200 CCU fails: document the bottleneck. Identify whether the limit is CPU, bandwidth, KCP, serialization, spatial, delta, or Unity-side. Do not guess.

---

## 2. WebGL WebSocket Load Validation

### Load Client

Use a Go WebSocket load client:

```
loadtest/shared/ws_client.go
```

The client must:

- Connect via HTTP /join with `client_platform: webgl`
- Open WSS/WebSocket session to game-server :9001
- Send Hello/JoinRoom via MessagePack (same envelope as KCP)
- Receive FullSnapshot
- Send PlayerInput/PlayerTransform via MessagePack
- Collect connection-level stats

Browser/WebGL performance must be validated separately from native KCP. WebGL load testing is not covered by KCP test results.

### Metrics to Capture

```
WebSocket RTT p50/p95/p99
WebSocket jitter
Send queue depth
Bandwidth in/out Mbps
Bytes per second per client
Disconnect count / reconnect count
Backpressure events
Server CPU % under WebSocket load
Room tick duration ms under WebSocket load
```

### Scenarios

```
scenario_ws_join:           WebSocket clients join gradually
scenario_ws_200ccu:         200 WebSocket clients move randomly
scenario_ws_reconnect:      WebSocket disconnect/reconnect cycle
```

### Acceptance Targets

Same targets as native KCP per CCU tier. WebSocket latency and jitter may be higher due to TCP head-of-line blocking — document measured values; do not assume equivalence with KCP.

---

## 3. Mixed Transport Room Validation

Mixed transport scenarios are required before production readiness. These validate the shared room logic and delta broadcast across session types.

### Required Scenarios

```
scenario_mixed_movement_kcp_sender:
- A KCP (native) user moves.
- WebSocket (WebGL) users in the same room receive the correct PlayerDelta.
- Validate delta semantics are identical.

scenario_mixed_movement_ws_sender:
- A WebSocket (WebGL) user moves.
- KCP (native) users in the same room receive the correct PlayerDelta.
- Validate delta semantics are identical.

scenario_mixed_object_lock:
- KCP and WebSocket users compete for the same object.
- Lock grant/reject is consistent across both transports.
- ObjectDelta is received correctly by both KCP and WebSocket clients.

scenario_mixed_join_storm:
- KCP and WebSocket clients join the same room simultaneously.
- No state corruption.
- FullSnapshot delivered correctly to both transport types.
```

### Pass Criteria

```
- Delta content is identical regardless of sender transport.
- Delta content is identical regardless of receiver transport.
- Object lock result is transport-agnostic.
- No room state corruption when both transport types are active.
- No full-room broadcast regression introduced by mixed transport.
```

---

## 4. Clarifications

### Web Visualizer

A web visualizer (browser debug page) is not load test evidence unless it explicitly uses WSS + MessagePack matching the production WebGL transport. A visualizer that connects via REST polling or HTTP debug endpoints does not validate the WebSocket transport path.

### Docker Compose

Docker Compose is dev and integration only. Load tests for single-vps production mode must target the actual single-vps runtime, not the Docker Compose dev stack.

### Benchmark vs Load Test

Benchmarks (`make bench-spatial`, `make bench-delta`) measure component throughput in isolation. They do not substitute for end-to-end CCU load tests. Run both.

---

## 5. Findings Log

> To be filled in after Milestone 7 runs.

| Date | CCU | Transport | CPU | Bandwidth | p95 RTT | Result | Notes |
|------|-----|-----------|-----|-----------|---------|--------|-------|
|      |     |           |     |           |         |        |       |
