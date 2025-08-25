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

// GetBandwidth returns bandwidth in Mbps using a rolling window
func (mt *MetricsTracker) GetBandwidth() float64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	// Use a rolling window of the last 5 seconds for bandwidth calculation
	windowDuration := 5 * time.Second
	now := time.Now()
	windowStart := now.Add(-windowDuration)

	// Calculate bytes transferred in the last window
	var windowBytes uint64
	var windowCount int

	// Count recent chunk deliveries within the window
	for i := len(mt.chunkSizes) - 1; i >= 0 && i >= len(mt.chunkSizes)-20; i-- {
		// Estimate time based on position (more recent = higher index)
		estimatedTime := mt.lastUpdateTime.Add(-time.Duration(len(mt.chunkSizes)-1-i) * 300 * time.Millisecond)
		if estimatedTime.After(windowStart) {
			windowBytes += uint64(mt.chunkSizes[i])
			windowCount++
		}
	}

	// If we don't have enough recent data, use a simple rate calculation
	if windowCount == 0 {
		// Calculate rate based on recent activity
		if len(mt.chunkSizes) > 0 {
			// Use the last few chunks to estimate current rate
			recentChunks := 10
			if len(mt.chunkSizes) < 10 {
				recentChunks = len(mt.chunkSizes)
			}
			for i := len(mt.chunkSizes) - int(recentChunks); i < len(mt.chunkSizes); i++ {
				windowBytes += uint64(mt.chunkSizes[i])
			}
			// Estimate time for recent chunks (300ms per chunk)
			estimatedTime := time.Duration(recentChunks) * 300 * time.Millisecond
			bytesPerSecond := float64(windowBytes) / estimatedTime.Seconds()
			mbps := (bytesPerSecond * 8) / (1024 * 1024)
			return min(mbps, 1000) // Cap at 1 Gbps
		}
		return 0.0
	}

	// Calculate bandwidth over the window
	bytesPerSecond := float64(windowBytes) / windowDuration.Seconds()
	mbps := (bytesPerSecond * 8) / (1024 * 1024) // Convert to Mbps

	// Cap the bandwidth to prevent unrealistic values
	return min(mbps, 1000) // Cap at 1 Gbps
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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
