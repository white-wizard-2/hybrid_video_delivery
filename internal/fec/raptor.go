package fec

import (
	"fmt"
	"log"
	"math/rand"
	"sync"

	"github.com/klauspost/reedsolomon"
)

// RaptorFEC implements Raptor Forward Error Correction
type RaptorFEC struct {
	dataShards   int
	parityShards int
	shardSize    int
	encoder      reedsolomon.Encoder
	mu           sync.RWMutex
	stats        *FECStats
}

// FECStats tracks FEC performance statistics
type FECStats struct {
	TotalPackets     uint64
	LostPackets      uint64
	RecoveredPackets uint64
	FailedRecoveries uint64
	mu               sync.RWMutex
}

// NewRaptorFEC creates a new Raptor FEC encoder/decoder
func NewRaptorFEC(dataShards, parityShards, shardSize int) (*RaptorFEC, error) {
	encoder, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("failed to create Reed-Solomon encoder: %v", err)
	}

	return &RaptorFEC{
		dataShards:   dataShards,
		parityShards: parityShards,
		shardSize:    shardSize,
		encoder:      encoder,
		stats:        &FECStats{},
	}, nil
}

// Encode encodes data with FEC
func (rf *RaptorFEC) Encode(data []byte) ([][]byte, error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// Pad data to shard size
	paddedData := rf.padData(data)

	// Split data into shards
	shards := make([][]byte, rf.dataShards+rf.parityShards)
	for i := 0; i < rf.dataShards; i++ {
		start := i * rf.shardSize
		end := start + rf.shardSize
		if end > len(paddedData) {
			end = len(paddedData)
		}
		shards[i] = make([]byte, rf.shardSize)
		copy(shards[i], paddedData[start:end])
	}

	// Initialize parity shards
	for i := rf.dataShards; i < rf.dataShards+rf.parityShards; i++ {
		shards[i] = make([]byte, rf.shardSize)
	}

	// Encode parity shards
	err := rf.encoder.Encode(shards)
	if err != nil {
		return nil, fmt.Errorf("failed to encode data: %v", err)
	}

	rf.stats.mu.Lock()
	rf.stats.TotalPackets++
	rf.stats.mu.Unlock()

	log.Printf("FEC encoded %d bytes into %d data + %d parity shards", len(data), rf.dataShards, rf.parityShards)
	return shards, nil
}

// Decode decodes data with FEC recovery
func (rf *RaptorFEC) Decode(shards [][]byte) ([]byte, error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// Check if we have enough shards
	availableShards := 0
	for _, shard := range shards {
		if shard != nil {
			availableShards++
		}
	}

	if availableShards < rf.dataShards {
		rf.stats.mu.Lock()
		rf.stats.LostPackets++
		rf.stats.FailedRecoveries++
		rf.stats.mu.Unlock()
		return nil, fmt.Errorf("insufficient shards for recovery: %d/%d", availableShards, rf.dataShards)
	}

	// Try to reconstruct missing shards
	err := rf.encoder.Reconstruct(shards)
	if err != nil {
		rf.stats.mu.Lock()
		rf.stats.LostPackets++
		rf.stats.FailedRecoveries++
		rf.stats.mu.Unlock()
		return nil, fmt.Errorf("failed to reconstruct data: %v", err)
	}

	// Extract original data
	originalData := make([]byte, 0, rf.dataShards*rf.shardSize)
	for i := 0; i < rf.dataShards; i++ {
		originalData = append(originalData, shards[i]...)
	}

	// Remove padding
	originalData = rf.unpadData(originalData)

	rf.stats.mu.Lock()
	rf.stats.RecoveredPackets++
	rf.stats.mu.Unlock()

	log.Printf("FEC decoded %d bytes successfully", len(originalData))
	return originalData, nil
}

// SimulatePacketLoss simulates packet loss for testing
func (rf *RaptorFEC) SimulatePacketLoss(shards [][]byte, lossRate float64) [][]byte {
	rf.mu.RLock()
	defer rf.mu.RUnlock()

	// Create a copy of shards
	result := make([][]byte, len(shards))
	copy(result, shards)

	// Randomly drop shards based on loss rate
	for i := range result {
		if rand.Float64() < lossRate {
			result[i] = nil // Mark as lost
		}
	}

	return result
}

// padData pads data to shard size
func (rf *RaptorFEC) padData(data []byte) []byte {
	totalSize := rf.dataShards * rf.shardSize
	if len(data) >= totalSize {
		return data[:totalSize]
	}

	padded := make([]byte, totalSize)
	copy(padded, data)
	return padded
}

// unpadData removes padding from data
func (rf *RaptorFEC) unpadData(data []byte) []byte {
	// Find the last non-zero byte
	lastNonZero := len(data) - 1
	for lastNonZero >= 0 && data[lastNonZero] == 0 {
		lastNonZero--
	}
	return data[:lastNonZero+1]
}

// GetStats returns FEC statistics
func (rf *RaptorFEC) GetStats() FECStats {
	rf.stats.mu.RLock()
	defer rf.stats.mu.RUnlock()
	return *rf.stats
}

// GetRecoveryRate returns the packet recovery rate
func (rf *RaptorFEC) GetRecoveryRate() float64 {
	rf.stats.mu.RLock()
	defer rf.stats.mu.RUnlock()

	if rf.stats.TotalPackets == 0 {
		return 0.0
	}

	return float64(rf.stats.RecoveredPackets) / float64(rf.stats.TotalPackets) * 100.0
}

// GetLossRate returns the packet loss rate
func (rf *RaptorFEC) GetLossRate() float64 {
	rf.stats.mu.RLock()
	defer rf.stats.mu.RUnlock()

	if rf.stats.TotalPackets == 0 {
		return 0.0
	}

	return float64(rf.stats.LostPackets) / float64(rf.stats.TotalPackets) * 100.0
}
