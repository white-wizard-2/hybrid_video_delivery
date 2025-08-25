package cdn

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"mockProxy/internal/cmaf"
	"mockProxy/internal/common"
	"mockProxy/internal/metrics"
)

type CDNNode struct {
	ID       string
	Clients  map[string]*common.Client
	mu       sync.RWMutex
	videoURL string
	Stream   *cmaf.CMAFStream
}

type CDNService struct {
	nodes   map[string]*CDNNode
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	logChan chan common.LogEntry
	metrics *metrics.MetricsTracker
	config  common.GlobalConfig
}

func NewService() *CDNService {
	ctx, cancel := context.WithCancel(context.Background())
	return &CDNService{
		nodes:   make(map[string]*CDNNode),
		ctx:     ctx,
		cancel:  cancel,
		logChan: make(chan common.LogEntry, 1000),
		metrics: metrics.NewMetricsTracker(100), // Track last 100 samples
	}
}

func (s *CDNService) Start() {
	log.Println("CDN Service started")
	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "CDN",
		Message:   "CDN Service started",
	}
}

func (s *CDNService) Stop() {
	s.cancel()
	log.Println("CDN Service stopped")
}

func (s *CDNService) AddNode() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodeID := fmt.Sprintf("cdn-node-%d", len(s.nodes)+1)

	// Use CMAF-CTE chunking for video streaming
	stream := cmaf.NewCMAFStream(nodeID, "./sample.mp4", 1*time.Second, "avc1.640028", 200, true) // Match Proxy: 200 Mbps
	err := stream.Initialize()
	if err != nil {
		log.Printf("Failed to initialize CMAF stream for %s: %v", nodeID, err)
	}

	node := &CDNNode{
		ID:       nodeID,
		Clients:  make(map[string]*common.Client),
		videoURL: "/sample.mp4",
		Stream:   stream,
	}

	s.nodes[nodeID] = node

	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "CDN",
		Message:   fmt.Sprintf("Added CDN node: %s", nodeID),
	}

	return nodeID
}

func (s *CDNService) AddClient(nodeID string) string {
	s.mu.RLock()
	node, exists := s.nodes[nodeID]
	s.mu.RUnlock()

	if !exists {
		return ""
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	clientID := fmt.Sprintf("cdn-client-%d", len(node.Clients)+1)
	client := &common.Client{
		ID:        clientID,
		NodeID:    nodeID,
		Type:      "CDN",
		StartTime: time.Now(),
		Status:    "connected",
	}

	node.Clients[clientID] = client

	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "CDN",
		Message:   fmt.Sprintf("Client %s connected to CDN node %s", clientID, nodeID),
	}

	// Start video streaming simulation in a non-blocking way
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Video streaming panic recovered for client %s: %v", clientID, r)
			}
		}()
		s.simulateVideoStreaming(client)
	}()

	return clientID
}

func (s *CDNService) simulateVideoStreaming(client *common.Client) {
	// Find the node this client belongs to
	s.mu.RLock()
	var node *CDNNode
	for _, n := range s.nodes {
		if _, exists := n.Clients[client.ID]; exists {
			node = n
			break
		}
	}
	s.mu.RUnlock()

	if node == nil || node.Stream == nil {
		log.Printf("No video stream found for client %s", client.ID)
		return
	}

	// Use chunk duration for ticker
	ticker := time.NewTicker(300 * time.Millisecond) // Match Proxy: 300ms chunks
	defer ticker.Stop()

	chunkCount := 0
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			chunkCount++
			chunkStartTime := time.Now()

			// Get next CMAF chunk
			chunk, err := node.Stream.GetNextChunk()
			if err != nil {
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "ERROR",
					Service:   "CDN",
					Message:   fmt.Sprintf("Failed to get CMAF chunk for client %s: %v", client.ID, err),
				}
				continue
			}

			// Simulate realistic network latency (CDN: 20-50ms)
			networkLatency := time.Duration(20+rand.Intn(30)) * time.Millisecond
			time.Sleep(networkLatency) // Simulate network delay
			deliveryTime := time.Since(chunkStartTime)

			// Check for packet drop simulation
			s.mu.RLock()
			packetDropEnabled := s.config.PacketDrop.Enabled
			dropRate := s.config.PacketDrop.DropRate
			s.mu.RUnlock()

			if packetDropEnabled && rand.Float64()*100 < dropRate {
				s.metrics.RecordPacketLoss()
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "WARN",
					Service:   "CDN",
					Message:   fmt.Sprintf("CDN packet loss detected for client %s (chunk %d)", client.ID, chunkCount),
				}
				continue // Skip this chunk delivery
			}

			// Record metrics for real calculations
			s.metrics.RecordChunkDelivery(chunk.Size, deliveryTime)
			s.metrics.RecordSuccessfulPacket()

			// Write chunk to CDN output folder
			s.writeChunkToOutput(chunk, "cdn")

			// Log successful delivery
			s.logChan <- common.LogEntry{
				Timestamp: time.Now(),
				Level:     "DEBUG",
				Service:   "CDN",
				Message: fmt.Sprintf("CDN delivered CMAF chunk %d (ID: %s, size: %d bytes, latency: %.2fms) to client %s",
					chunkCount, chunk.ID[:8], chunk.Size, deliveryTime.Milliseconds(), client.ID),
			}
		}
	}
}

func (s *CDNService) GetNodes() map[string]*CDNNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodes
}

func (s *CDNService) GetLogs() <-chan common.LogEntry {
	return s.logChan
}

func (s *CDNService) GetStats() common.ServiceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalClients := 0
	for _, node := range s.nodes {
		node.mu.RLock()
		totalClients += len(node.Clients)
		node.mu.RUnlock()
	}

	// Use real metrics calculations
	latency := s.metrics.GetLatency()
	if latency == 0 {
		latency = 0 // Fallback to simulated value if no data yet
	}

	bandwidth := s.metrics.GetBandwidth()
	if bandwidth == 0 {
		bandwidth = 0 // Fallback to simulated value if no data yet
	}

	packetLoss := s.metrics.GetPacketLoss()
	if packetLoss == 0 {
		packetLoss = 0 // Fallback to simulated value if no data yet
	}

	// Get additional metrics
	unicastBytes := s.metrics.GetTotalBytes()
	totalChunks := s.metrics.GetTotalChunks()
	avgChunkSize := s.metrics.GetAverageChunkSize()
	recoveryRate := s.metrics.GetRecoveryRate()

	return common.ServiceStats{
		Type:         "CDN",
		NodeCount:    len(s.nodes),
		ClientCount:  totalClients,
		Latency:      int(latency),
		PacketLoss:   packetLoss,
		Bandwidth:    int(bandwidth),
		UnicastBytes: unicastBytes,
		TotalChunks:  totalChunks,
		AvgChunkSize: avgChunkSize,
		RecoveryRate: recoveryRate,
	}
}

// UpdateConfig updates the service configuration
func (s *CDNService) UpdateConfig(config common.GlobalConfig) {
	s.mu.Lock()
	s.config = config
	s.mu.Unlock()

	log.Printf("CDN Service: Configuration updated - Packet Drop Enabled: %v, Drop Rate: %.2f%%",
		config.PacketDrop.Enabled, config.PacketDrop.DropRate)
}

// writeChunkToOutput writes a chunk to the CDN output folder
func (s *CDNService) writeChunkToOutput(chunk *cmaf.CMAFChunk, outputType string) {
	outputDir := fmt.Sprintf("%s_output", outputType)

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("Failed to create output directory %s: %v", outputDir, err)
		return
	}

	// Create filename with timestamp and chunk info
	timestamp := time.Now().Format("20060102_150405_000")
	filename := fmt.Sprintf("%s/chunk_%s_%s_%d.m4s", outputDir, timestamp, chunk.ID[:8], chunk.Size)

	// Write chunk data to file
	if err := os.WriteFile(filename, chunk.Data, 0644); err != nil {
		log.Printf("Failed to write chunk to %s: %v", filename, err)
	} else {
		log.Printf("CDN wrote chunk %s to %s (size: %d bytes)", chunk.ID[:8], filename, chunk.Size)
	}
}
