package proxy

import (
	"context"
	"fmt"
	"log"
	"math/rand"
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
}

func NewService() *ProxyService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProxyService{
		nodes:   make(map[string]*ProxyNode),
		ctx:     ctx,
		cancel:  cancel,
		logChan: make(chan common.LogEntry, 1000),
		metrics: metrics.NewProxyMetricsTracker(100), // Track last 100 samples
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

	// Initialize real Raptor FEC
	raptorFEC, err := fec.NewRaptorFEC(100, 20, 1024)
	if err != nil {
		log.Printf("Failed to create Raptor FEC for %s: %v", nodeID, err)
	}

	// Initialize real FLUTE session
	fluteSession := flute.NewFLUTESession(
		fmt.Sprintf("session-%s", nodeID),
		fmt.Sprintf("transport-%s", nodeID),
		"192.168.1.100",
		"239.255.255.250",
		8080+len(s.nodes),
		32,
	)

	// Start FLUTE session
	err = fluteSession.Start()
	if err != nil {
		log.Printf("Failed to start FLUTE session for %s: %v", nodeID, err)
	}

	// Use CMAF-CTE chunking for video streaming
	stream := cmaf.NewCMAFStream(nodeID, "./sample.mp4", 1*time.Second, "avc1.640028", 200, true)
	err = stream.Initialize()
	if err != nil {
		log.Printf("Failed to initialize CMAF stream for %s: %v", nodeID, err)
	}

	node := &ProxyNode{
		ID:      nodeID,
		Clients: make(map[string]*common.Client),
		FEC:     raptorFEC,
		FLUTE:   fluteSession,
		Stream:  stream,
	}

	s.nodes[nodeID] = node

	s.logChan <- common.LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "PROXY",
		Message: fmt.Sprintf("Added Proxy node: %s with Raptor FEC (100 source, 20 repair symbols) and FLUTE delivery",
			nodeID),
	}

	return nodeID
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

	chunkCount := 0
	symbolCount := 0

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			chunkCount++
			symbolCount++
			chunkStartTime := time.Now()

			// Get next CMAF chunk
			chunk, err := node.Stream.GetNextChunk()
			if err != nil {
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "ERROR",
					Service:   "PROXY",
					Message:   fmt.Sprintf("Failed to get CMAF chunk for client %s: %v", client.ID, err),
				}
				continue
			}

			// Simulate realistic network latency (5G: 5-30ms)
			networkLatency := time.Duration(5+rand.Intn(25)) * time.Millisecond
			time.Sleep(networkLatency) // Simulate network delay
			deliveryTime := time.Since(chunkStartTime)

			// Record metrics for real calculations
			// Use broadcast delivery for FLUTE multicast (sent once, received by all clients)
			clientCount := len(node.Clients)
			s.metrics.RecordBroadcastDelivery(chunk.Size, deliveryTime, clientCount)
			s.metrics.RecordSuccessfulPacket()

			// Simulate 5G broadcast with FLUTE delivery and CMAF chunks
			s.logChan <- common.LogEntry{
				Timestamp: time.Now(),
				Level:     "DEBUG",
				Service:   "PROXY",
				Message: fmt.Sprintf("5G broadcast CMAF chunk %d (ID: %s, size: %d bytes, latency: %.2fms) to client %s via FLUTE multicast %s:%d",
					chunkCount, chunk.ID[:8], chunk.Size, deliveryTime.Milliseconds(), client.ID, node.FLUTE.MulticastAddr, node.FLUTE.Port),
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

			// Simulate higher packet loss in 5G environment (higher than CDN)
			if chunkCount%5 == 0 {
				s.metrics.RecordPacketLoss()
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "WARN",
					Service:   "PROXY",
					Message: fmt.Sprintf("5G packet loss detected for client %s (chunk %d), applying Raptor FEC recovery",
						client.ID, chunkCount),
				}

				// Simulate FEC recovery
				if rand.Float64() < 0.8 { // 80% recovery rate
					s.metrics.RecordPacketRecovery()
					s.logChan <- common.LogEntry{
						Timestamp: time.Now(),
						Level:     "INFO",
						Service:   "PROXY",
						Message:   fmt.Sprintf("Raptor FEC successfully recovered lost packet for client %s", client.ID),
					}
				} else {
					// FEC failed - fallback to unicast delivery
					s.metrics.RecordFECFailure()
					s.logChan <- common.LogEntry{
						Timestamp: time.Now(),
						Level:     "ERROR",
						Service:   "PROXY",
						Message:   fmt.Sprintf("Raptor FEC failed to recover packet for client %s, falling back to unicast", client.ID),
					}

					// Simulate unicast fallback delivery
					go s.simulateUnicastFallback(client, node, chunk, chunkCount)
				}
			}

			// Simulate FLUTE session management
			if chunkCount%50 == 0 {
				s.logChan <- common.LogEntry{
					Timestamp: time.Now(),
					Level:     "INFO",
					Service:   "PROXY",
					Message: fmt.Sprintf("FLUTE session %s heartbeat for client %s",
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
