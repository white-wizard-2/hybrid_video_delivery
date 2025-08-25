package proxy

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
	"mockProxy/internal/fec"
	"mockProxy/internal/flute"
	"mockProxy/internal/metrics"
)

type ProxyNode struct {
	ID      string
	Clients map[string]*common.Client
	mu      sync.RWMutex
	FEC     *fec.RaptorFEC
	FLUTE   *flute.FLUTESession
	Stream  *cmaf.CMAFStream
}

type ProxyService struct {
	nodes   map[string]*ProxyNode
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	logChan chan common.LogEntry
	metrics *metrics.ProxyMetricsTracker

	// Shared multicast infrastructure - only one broadcast stream for all nodes
	sharedStream     *cmaf.CMAFStream
	sharedFLUTE      *flute.FLUTESession
	sharedFEC        *fec.RaptorFEC
	streamStarted    bool
	globalChunkCount int
	chunkMutex       sync.Mutex

	// Global broadcast coordinator
	broadcastTicker  *time.Ticker
	broadcastStarted bool

	// Configuration for packet drop simulation
	config common.GlobalConfig
}

func NewService() *ProxyService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProxyService{
		nodes:   make(map[string]*ProxyNode),
		ctx:     ctx,
		cancel:  cancel,
		logChan: make(chan common.LogEntry, 1000),
		metrics: metrics.NewProxyMetricsTracker(100), // Track last 100 samples
		config:  common.GlobalConfig{},               // Initialize with default config
	}
}

func (s *ProxyService) Start() {
	log.Println("Proxy Service started")
	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "PROXY",
		Message:   "Proxy Service started with Raptor FEC and FLUTE delivery",
	}
}

func (s *ProxyService) Stop() {
	s.cancel()
	log.Println("Proxy Service stopped")
}

func (s *ProxyService) AddNode() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodeID := fmt.Sprintf("proxy-node-%d", len(s.nodes)+1)

	// Initialize shared multicast infrastructure only once
	if !s.streamStarted {
		// Initialize shared Raptor FEC with fixed repair symbols (always recover as much as possible)
		sharedFEC, err := fec.NewRaptorFEC(100, 20, 1024) // 20 repair symbols for maximum recovery
		if err != nil {
			log.Printf("Failed to create shared Raptor FEC: %v", err)
		} else {
			s.sharedFEC = sharedFEC
		}

		// Initialize shared FLUTE session (one session for all nodes)
		sharedFLUTE := flute.NewFLUTESession(
			"shared-multicast-session",
			"shared-transport",
			"192.168.1.100",
			"239.255.255.250",
			8080, // Fixed port for shared multicast
			32,
		)

		// Start shared FLUTE session
		err = sharedFLUTE.Start()
		if err != nil {
			log.Printf("Failed to start shared FLUTE session: %v", err)
		} else {
			s.sharedFLUTE = sharedFLUTE
		}

		// Initialize shared CMAF stream (one stream for all nodes)
		sharedStream := cmaf.NewCMAFStream("shared-multicast", "./sample.mp4", 1*time.Second, "avc1.640028", 200, true)
		err = sharedStream.Initialize()
		if err != nil {
			log.Printf("Failed to initialize shared CMAF stream: %v", err)
		} else {
			s.sharedStream = sharedStream
		}

		s.streamStarted = true
		s.globalChunkCount = 0

		// Start global broadcast coordinator
		s.startGlobalBroadcast()

		s.logChan <- common.LogEntry{
			Timestamp: time.Now(),
			Level:     "INFO",
			Service:   "PROXY",
			Message:   "Shared multicast infrastructure initialized: single broadcast stream for all nodes",
		}
	}

	// Create node that uses shared infrastructure
	node := &ProxyNode{
		ID:      nodeID,
		Clients: make(map[string]*common.Client),
		FEC:     s.sharedFEC,    // Use shared FEC
		FLUTE:   s.sharedFLUTE,  // Use shared FLUTE
		Stream:  s.sharedStream, // Use shared stream
	}

	s.nodes[nodeID] = node

	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "PROXY",
		Message: fmt.Sprintf("Added Proxy node: %s using shared multicast infrastructure (bandwidth doesn't scale with nodes)",
			nodeID),
	}

	return nodeID
}

// startGlobalBroadcast starts a single global broadcast stream that serves all nodes
func (s *ProxyService) startGlobalBroadcast() {
	if s.broadcastStarted {
		return
	}

	s.broadcastTicker = time.NewTicker(300 * time.Millisecond)
	s.broadcastStarted = true

	go func() {
		for {
			select {
			case <-s.ctx.Done():
				if s.broadcastTicker != nil {
					s.broadcastTicker.Stop()
				}
				return
			case <-s.broadcastTicker.C:
				s.broadcastNextChunk()
			}
		}
	}()

	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "PROXY",
		Message:   "Global broadcast coordinator started: single broadcast stream for all nodes",
	}
}

// broadcastNextChunk broadcasts the next chunk to all nodes globally
func (s *ProxyService) broadcastNextChunk() {
	if s.sharedStream == nil {
		return
	}

	chunkStartTime := time.Now()

	// Get next CMAF chunk from shared stream
	chunk, err := s.sharedStream.GetNextChunk()
	if err != nil {
		s.logChan <- common.LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Service:   "PROXY",
			Message:   fmt.Sprintf("Failed to get CMAF chunk for global broadcast: %v", err),
		}
		return
	}

	// Simulate realistic network latency (5G: 5-30ms)
	networkLatency := time.Duration(5+rand.Intn(25)) * time.Millisecond
	time.Sleep(networkLatency) // Simulate network delay
	deliveryTime := time.Since(chunkStartTime)

	// Increment global chunk counter
	s.chunkMutex.Lock()
	s.globalChunkCount++
	globalChunkNum := s.globalChunkCount
	s.chunkMutex.Unlock()

	// Record broadcast delivery once globally (simulating true multicast)
	// This ensures bandwidth doesn't scale with the number of nodes
	totalClients := 0
	s.mu.RLock()
	for _, node := range s.nodes {
		totalClients += len(node.Clients)
	}
	s.mu.RUnlock()

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
			Service:   "PROXY",
			Message:   fmt.Sprintf("Global broadcast packet loss detected (chunk %d)", globalChunkNum),
		}

		// Always try FEC recovery (FEC is always enabled)
		if s.sharedFEC != nil {
			if rand.Float64() < 0.8 { // 80% FEC recovery rate
				s.metrics.RecordFECSuccess()
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "INFO",
					Service:   "PROXY",
					Message:   fmt.Sprintf("Raptor FEC successfully recovered lost packet (chunk %d)", globalChunkNum),
				}
			} else {
				s.metrics.RecordFECFailure()
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "ERROR",
					Service:   "PROXY",
					Message:   fmt.Sprintf("Raptor FEC failed to recover packet (chunk %d), falling back to unicast", globalChunkNum),
				}
				// Simulate unicast fallback
				s.metrics.RecordUnicastFallback()
			}
		}
		return
	}

	s.metrics.RecordBroadcastDelivery(chunk.Size, deliveryTime, totalClients)
	s.metrics.RecordSuccessfulPacket()

	// Write chunk to Proxy output folder
	s.writeChunkToOutput(chunk, "proxy")

	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Service:   "PROXY",
		Message: fmt.Sprintf("Global broadcast: CMAF chunk %d (ID: %s, size: %d bytes, latency: %.2fms) via shared FLUTE multicast %s:%d (serving %d nodes, %d total clients)",
			globalChunkNum, chunk.ID[:8], chunk.Size, deliveryTime.Milliseconds(), s.sharedFLUTE.MulticastAddr, s.sharedFLUTE.Port, len(s.nodes), totalClients),
	}
}

func (s *ProxyService) AddClient(nodeID string) string {
	s.mu.RLock()
	node, exists := s.nodes[nodeID]
	s.mu.RUnlock()

	if !exists {
		return ""
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	clientID := fmt.Sprintf("proxy-client-%d", len(node.Clients)+1)
	client := &common.Client{
		ID:        clientID,
		NodeID:    nodeID,
		Type:      "PROXY",
		StartTime: time.Now(),
		Status:    "connected",
	}

	node.Clients[clientID] = client

	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "PROXY",
		Message:   fmt.Sprintf("Client %s connected to Proxy node %s via FLUTE multicast", clientID, nodeID),
	}

	// Start 5G broadcast simulation with FLUTE delivery
	go s.simulate5GBroadcast(client, node)

	return clientID
}

func (s *ProxyService) simulate5GBroadcast(client *common.Client, node *ProxyNode) {
	// Use chunk duration for ticker (300ms chunks for better metrics)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	localChunkCount := 0
	symbolCount := 0

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			localChunkCount++
			symbolCount++

			// Get current global chunk number (broadcast is handled globally)
			s.chunkMutex.Lock()
			globalChunkNum := s.globalChunkCount
			s.chunkMutex.Unlock()

			// Simulate client receiving the broadcast chunk
			networkLatency := time.Duration(5+rand.Intn(25)) * time.Millisecond
			time.Sleep(networkLatency) // Simulate network delay

			// Log client reception of broadcast chunk
			s.logChan <- common.LogEntry{
				Timestamp: time.Now(),
				Level:     "DEBUG",
				Service:   "PROXY",
				Message: fmt.Sprintf("Client %s received broadcast chunk %d via shared multicast (latency: %.2fms)",
					client.ID, globalChunkNum, networkLatency.Milliseconds()),
			}

			// Simulate Raptor FEC encoding/decoding
			if symbolCount%10 == 0 {
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "DEBUG",
					Service:   "PROXY",
					Message: fmt.Sprintf("Raptor FEC encoding: 100 source symbols + 20 repair symbols for client %s",
						client.ID),
				}
			}

			// Simulate FLUTE session management
			if localChunkCount%50 == 0 {
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "INFO",
					Service:   "PROXY",
					Message: fmt.Sprintf("Shared FLUTE session %s heartbeat for client %s",
						node.FLUTE.SessionID, client.ID),
				}
			}
		}
	}
}

// simulateUnicastFallback simulates unicast delivery when FEC fails
func (s *ProxyService) simulateUnicastFallback(client *common.Client, node *ProxyNode, chunk *cmaf.CMAFChunk, chunkCount int) {
	// Simulate unicast network latency (higher than multicast)
	unicastLatency := time.Duration(10+rand.Intn(40)) * time.Millisecond // 10-50ms for unicast
	time.Sleep(unicastLatency)

	// Record unicast delivery metrics
	deliveryTime := unicastLatency
	s.metrics.RecordUnicastDelivery(chunk.Size, deliveryTime)
	s.metrics.RecordUnicastFallback()
	s.metrics.RecordSuccessfulPacket()

	// Log the unicast fallback delivery
	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "PROXY",
		Message: fmt.Sprintf("Unicast fallback delivery: chunk %d (ID: %s, size: %d bytes, latency: %.2fms) to client %s via unicast",
			chunkCount, chunk.ID[:8], chunk.Size, deliveryTime.Milliseconds(), client.ID),
	}
}

func (s *ProxyService) GetNodes() map[string]*ProxyNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodes
}

func (s *ProxyService) GetLogs() <-chan common.LogEntry {
	return s.logChan
}

func (s *ProxyService) GetStats() common.ProxyStats {
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

	totalBandwidth := s.metrics.GetTotalBandwidth()
	if totalBandwidth == 0 {
		totalBandwidth = 0 // Fallback to simulated value if no data yet
	}

	packetLoss := s.metrics.GetPacketLoss()
	if packetLoss == 0 {
		packetLoss = 0 // Fallback to simulated value if no data yet
	}

	// Get unicast and broadcast bandwidth
	unicastBandwidth := s.metrics.GetUnicastBandwidth()
	broadcastBandwidth := s.metrics.GetBroadcastBandwidth()
	unicastPercent, broadcastPercent := s.metrics.GetBandwidthSplit()

	return common.ProxyStats{
		ServiceStats: common.ServiceStats{
			Type:        "PROXY",
			NodeCount:   len(s.nodes),
			ClientCount: totalClients,
			Latency:     int(latency),
			PacketLoss:  packetLoss,
			Bandwidth:   int(totalBandwidth),
		},
		UnicastBandwidth:   unicastBandwidth,
		BroadcastBandwidth: broadcastBandwidth,
		UnicastPercent:     unicastPercent,
		BroadcastPercent:   broadcastPercent,
		FLUTEStats:         s.metrics.GetFLUTEStats(),
	}
}

// UpdateConfig updates the service configuration
func (s *ProxyService) UpdateConfig(config common.GlobalConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// FEC is always enabled with fixed repair symbols (20) for maximum recovery
	// No need to update FEC configuration

	s.config = config // Update the service's config

	log.Printf("Proxy Service: Configuration updated - Packet Drop Enabled: %v, Drop Rate: %.2f%%, FEC: Always enabled with 20 repair symbols",
		config.PacketDrop.Enabled, config.PacketDrop.DropRate)
}

// writeChunkToOutput writes a chunk to the Proxy output folder
func (s *ProxyService) writeChunkToOutput(chunk *cmaf.CMAFChunk, outputType string) {
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
		log.Printf("Proxy wrote chunk %s to %s (size: %d bytes)", chunk.ID[:8], filename, chunk.Size)
	}
}
