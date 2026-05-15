package shared

import "time"

// ScenarioConfig holds parameters for a load test scenario.
type ScenarioConfig struct {
	GatewayAddr string
	RoomID      string
	CCU         int
	Duration    time.Duration
	Movement    bool
	ObjectLock  bool
}

// ScenarioResult holds the outcome of a load test run.
type ScenarioResult struct {
	TotalClients      int
	Duration          time.Duration
	PacketsSent       int64
	PacketsReceived   int64
	BytesSent         int64
	BytesReceived     int64
	Errors            int64
	LatencyP50        time.Duration
	LatencyP95        time.Duration
	LatencyP99        time.Duration
	ServerCPU         float64
	ServerMemoryMB    float64
	AvgBytesPerClient float64
}

// PacketStats tracks per-packet statistics during a load test.
type PacketStats struct {
	Count     int64
	Bytes     int64
	Errors    int64
	Histogram []time.Duration
}

// Record adds a packet measurement.
func (s *PacketStats) Record(size int, latency time.Duration) {
	s.Count++
	s.Bytes += int64(size)
	s.Histogram = append(s.Histogram, latency)
}
