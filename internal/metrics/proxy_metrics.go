package metrics

import (
	"sync"
	"time"
)

// ProxyMetricsTracker extends MetricsTracker with unicast/broadcast bandwidth tracking
type ProxyMetricsTracker struct {
	*MetricsTracker // Embed base metrics

	// Unicast vs Broadcast bandwidth tracking
	unicastBytes   uint64
	broadcastBytes uint64
	unicastMutex   sync.RWMutex

	// FLUTE multicast tracking
	multicastSessions uint64
	flutePackets      uint64

	// FEC and fallback tracking
	fecFailures      uint64
	unicastFallbacks uint64
}

// NewProxyMetricsTracker creates a new proxy metrics tracker
func NewProxyMetricsTracker(maxSamples int) *ProxyMetricsTracker {
	return &ProxyMetricsTracker{
		MetricsTracker: NewMetricsTracker(maxSamples),
	}
}

// RecordUnicastDelivery records unicast chunk delivery (individual client)
func (pmt *ProxyMetricsTracker) RecordUnicastDelivery(chunkSize int, deliveryTime time.Duration) {
	pmt.MetricsTracker.RecordChunkDelivery(chunkSize, deliveryTime)

	pmt.unicastMutex.Lock()
	pmt.unicastBytes += uint64(chunkSize)
	pmt.unicastMutex.Unlock()
}

// RecordBroadcastDelivery records broadcast chunk delivery (multicast to all clients)
func (pmt *ProxyMetricsTracker) RecordBroadcastDelivery(chunkSize int, deliveryTime time.Duration, clientCount int) {
	pmt.MetricsTracker.RecordChunkDelivery(chunkSize, deliveryTime)

	pmt.unicastMutex.Lock()
	// Broadcast is sent once but received by all clients
	pmt.broadcastBytes += uint64(chunkSize)
	pmt.flutePackets++
	pmt.unicastMutex.Unlock()
}

// RecordFLUTESession records a new FLUTE multicast session
func (pmt *ProxyMetricsTracker) RecordFLUTESession() {
	pmt.unicastMutex.Lock()
	pmt.multicastSessions++
	pmt.unicastMutex.Unlock()
}

// RecordFECFailure records a FEC failure that triggered unicast fallback
func (pmt *ProxyMetricsTracker) RecordFECFailure() {
	pmt.unicastMutex.Lock()
	pmt.fecFailures++
	pmt.unicastMutex.Unlock()
}

// RecordUnicastFallback records a unicast fallback delivery
func (pmt *ProxyMetricsTracker) RecordUnicastFallback() {
	pmt.unicastMutex.Lock()
	pmt.unicastFallbacks++
	pmt.unicastMutex.Unlock()
}

// GetUnicastBandwidth returns unicast bandwidth in Mbps
func (pmt *ProxyMetricsTracker) GetUnicastBandwidth() float64 {
	pmt.unicastMutex.RLock()
	defer pmt.unicastMutex.RUnlock()

	if pmt.lastUpdateTime.Equal(pmt.startTime) {
		return 0.0
	}

	duration := pmt.lastUpdateTime.Sub(pmt.startTime).Seconds()
	if duration == 0 {
		return 0.0
	}

	bytesPerSecond := float64(pmt.unicastBytes) / duration
	mbps := (bytesPerSecond * 8) / (1024 * 1024) // Convert to Mbps

	return mbps
}

// GetBroadcastBandwidth returns broadcast bandwidth in Mbps
func (pmt *ProxyMetricsTracker) GetBroadcastBandwidth() float64 {
	pmt.unicastMutex.RLock()
	defer pmt.unicastMutex.RUnlock()

	if pmt.lastUpdateTime.Equal(pmt.startTime) {
		return 0.0
	}

	duration := pmt.lastUpdateTime.Sub(pmt.startTime).Seconds()
	if duration == 0 {
		return 0.0
	}

	bytesPerSecond := float64(pmt.broadcastBytes) / duration
	mbps := (bytesPerSecond * 8) / (1024 * 1024) // Convert to Mbps

	return mbps
}

// GetTotalBandwidth returns total bandwidth (unicast + broadcast) in Mbps
func (pmt *ProxyMetricsTracker) GetTotalBandwidth() float64 {
	return pmt.GetUnicastBandwidth() + pmt.GetBroadcastBandwidth()
}

// GetBandwidthSplit returns the percentage split between unicast and broadcast
func (pmt *ProxyMetricsTracker) GetBandwidthSplit() (unicastPercent, broadcastPercent float64) {
	total := pmt.GetTotalBandwidth()
	if total == 0 {
		return 0.0, 0.0
	}

	unicast := pmt.GetUnicastBandwidth()
	broadcast := pmt.GetBroadcastBandwidth()

	unicastPercent = (unicast / total) * 100.0
	broadcastPercent = (broadcast / total) * 100.0

	return unicastPercent, broadcastPercent
}

// GetFLUTEStats returns FLUTE-specific statistics
func (pmt *ProxyMetricsTracker) GetFLUTEStats() map[string]interface{} {
	pmt.unicastMutex.RLock()
	defer pmt.unicastMutex.RUnlock()

	return map[string]interface{}{
		"multicastSessions": pmt.multicastSessions,
		"flutePackets":      pmt.flutePackets,
		"unicastBytes":      pmt.unicastBytes,
		"broadcastBytes":    pmt.broadcastBytes,
		"fecFailures":       pmt.fecFailures,
		"unicastFallbacks":  pmt.unicastFallbacks,
	}
}

// Reset resets all proxy metrics
func (pmt *ProxyMetricsTracker) Reset() {
	pmt.MetricsTracker.Reset()

	pmt.unicastMutex.Lock()
	pmt.unicastBytes = 0
	pmt.broadcastBytes = 0
	pmt.multicastSessions = 0
	pmt.flutePackets = 0
	pmt.fecFailures = 0
	pmt.unicastFallbacks = 0
	pmt.unicastMutex.Unlock()
}
