package metrics

import (
	"sync"
	"time"
)

// MetricsTracker tracks real-time performance metrics
type MetricsTracker struct {
	mu sync.RWMutex

	// Latency tracking
	latencySamples    []time.Duration
	maxLatencySamples int

	// Bandwidth tracking
	bytesTransferred uint64
	startTime        time.Time
	lastUpdateTime   time.Time

	// Packet loss tracking
	totalPackets     uint64
	lostPackets      uint64
	recoveredPackets uint64

	// Chunk tracking
	totalChunks     uint64
	chunkSizes      []int
	maxChunkSamples int
}

// NewMetricsTracker creates a new metrics tracker
func NewMetricsTracker(maxSamples int) *MetricsTracker {
	return &MetricsTracker{
		maxLatencySamples: maxSamples,
		maxChunkSamples:   maxSamples,
		startTime:         time.Now(),
		lastUpdateTime:    time.Now(),
	}
}

// RecordChunkDelivery records a chunk delivery for metrics calculation
func (mt *MetricsTracker) RecordChunkDelivery(chunkSize int, deliveryTime time.Duration) {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	// Record latency
	mt.latencySamples = append(mt.latencySamples, deliveryTime)
	if len(mt.latencySamples) > mt.maxLatencySamples {
		mt.latencySamples = mt.latencySamples[1:]
	}

	// Record bandwidth
	mt.bytesTransferred += uint64(chunkSize)
	mt.lastUpdateTime = time.Now()

	// Record chunk size
	mt.chunkSizes = append(mt.chunkSizes, chunkSize)
	if len(mt.chunkSizes) > mt.maxChunkSamples {
		mt.chunkSizes = mt.chunkSizes[1:]
	}

	mt.totalChunks++
}

// RecordPacketLoss records packet loss events
func (mt *MetricsTracker) RecordPacketLoss() {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.totalPackets++
	mt.lostPackets++
}

// RecordPacketRecovery records successful packet recovery
func (mt *MetricsTracker) RecordPacketRecovery() {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.totalPackets++
	mt.recoveredPackets++
}

// RecordSuccessfulPacket records successful packet delivery
func (mt *MetricsTracker) RecordSuccessfulPacket() {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.totalPackets++
}

// GetLatency returns the average latency in milliseconds
func (mt *MetricsTracker) GetLatency() float64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	if len(mt.latencySamples) == 0 {
		return 0.0
	}

	var total time.Duration
	for _, latency := range mt.latencySamples {
		total += latency
	}

	avgLatency := total / time.Duration(len(mt.latencySamples))
	result := float64(avgLatency.Milliseconds())

	// Debug logging (commented out for production)
	// fmt.Printf("DEBUG: Latency samples: %d, total: %v, avg: %v, result: %.2fms\n",
	// 	len(mt.latencySamples), total, avgLatency, result)

	return result
}

// GetBandwidth returns the current bandwidth in Mbps
func (mt *MetricsTracker) GetBandwidth() float64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	if mt.lastUpdateTime.Equal(mt.startTime) {
		return 0.0
	}

	duration := mt.lastUpdateTime.Sub(mt.startTime).Seconds()
	if duration == 0 {
		return 0.0
	}

	// Convert bytes to Mbps
	bytesPerSecond := float64(mt.bytesTransferred) / duration
	mbps := (bytesPerSecond * 8) / (1024 * 1024) // Convert to Mbps

	// Debug logging (commented out for production)
	// fmt.Printf("DEBUG: Bandwidth - bytes: %d, duration: %.2fs, bytes/sec: %.2f, mbps: %.2f\n",
	// 	mt.bytesTransferred, duration, bytesPerSecond, mbps)

	return mbps
}

// GetPacketLoss returns the packet loss percentage
func (mt *MetricsTracker) GetPacketLoss() float64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	if mt.totalPackets == 0 {
		return 0.0
	}

	return (float64(mt.lostPackets) / float64(mt.totalPackets)) * 100.0
}

// GetRecoveryRate returns the packet recovery rate percentage
func (mt *MetricsTracker) GetRecoveryRate() float64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	if mt.lostPackets == 0 {
		return 100.0
	}

	return (float64(mt.recoveredPackets) / float64(mt.lostPackets)) * 100.0
}

// GetAverageChunkSize returns the average chunk size in bytes
func (mt *MetricsTracker) GetAverageChunkSize() int {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	if len(mt.chunkSizes) == 0 {
		return 0
	}

	total := 0
	for _, size := range mt.chunkSizes {
		total += size
	}

	return total / len(mt.chunkSizes)
}

// GetTotalChunks returns the total number of chunks delivered
func (mt *MetricsTracker) GetTotalChunks() uint64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.totalChunks
}

// GetTotalBytes returns the total bytes transferred
func (mt *MetricsTracker) GetTotalBytes() uint64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.bytesTransferred
}

// Reset resets all metrics
func (mt *MetricsTracker) Reset() {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	mt.latencySamples = nil
	mt.chunkSizes = nil
	mt.bytesTransferred = 0
	mt.totalPackets = 0
	mt.lostPackets = 0
	mt.recoveredPackets = 0
	mt.totalChunks = 0
	mt.startTime = time.Now()
	mt.lastUpdateTime = time.Now()
}
