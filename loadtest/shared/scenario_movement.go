package shared

// ScenarioMovement simulates N clients moving randomly in a room,
// validating that PlayerDelta messages are received correctly and
// that no full-room broadcast occurs during normal ticks.
//
// Measures:
//   - Delta broadcast correctness
//   - Packet size per client
//   - Server tick duration
//   - Latency p50/p95/p99
//
// Not yet implemented.
func ScenarioMovement(config ScenarioConfig) (*ScenarioResult, error) {
	// TODO: implement
	return nil, nil
}
