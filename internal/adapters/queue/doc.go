// Package queue defines the adapter boundary for pending-room queue operations.
//
// Future implementations:
//
//   - MemoryQueue: in-process channel-based queue for dev and single-vps modes.
//   - RedisQueue: Redis LIST-backed queue for distributed-k3s mode, consumed
//     by KEDA ScaledObject to trigger game-node scaling.
//
// The distributed resolver and registry use this adapter to push pending-room
// requests when no healthy game node has capacity. Single-vps mode does not
// use this adapter and must not depend on it at runtime. No Redis client
// dependency should be added until the distributed mode implementation begins.
package queue
