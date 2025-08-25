package cdn

import (
	"context"
	"fmt"
	"log"
	"math/rand"
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
	stream := cmaf.NewCMAFStream(nodeID, "./sample.mp4", 2*time.Second, "avc1.640028", 100, true)
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
	ticker := time.NewTicker(500 * time.Millisecond) // 500ms chunks for better metrics
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

			// Simulate realistic network latency (CDN: 20-80ms)
			networkLatency := time.Duration(20+rand.Intn(60)) * time.Millisecond
			time.Sleep(networkLatency) // Simulate network delay
			deliveryTime := time.Since(chunkStartTime)

			// Record metrics for real calculations
			s.metrics.RecordChunkDelivery(chunk.Size, deliveryTime)
			s.metrics.RecordSuccessfulPacket()

			// Real CDN streaming with CMAF-CTE chunks
			s.logChan <- common.LogEntry{
				Timestamp: time.Now(),
				Level:     "DEBUG",
				Service:   "CDN",
				Message: fmt.Sprintf("CDN streaming CMAF chunk %d (ID: %s, size: %d bytes, duration: %s, latency: %.2fms) to client %s (CMAF-CTE)",
					chunkCount, chunk.ID[:8], chunk.Size, chunk.Duration, deliveryTime.Milliseconds(), client.ID),
			}

			// Simulate occasional packet loss (lower than proxy)
			if chunkCount%5 == 0 { // More frequent packet loss for testing
				s.metrics.RecordPacketLoss()
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "WARN",
					Service:   "CDN",
					Message:   fmt.Sprintf("CDN packet loss detected for client %s (chunk %d)", client.ID, chunkCount),
				}
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
