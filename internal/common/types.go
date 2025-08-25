package common

import (
	"time"
)

// Client represents a client connected to either CDN or Proxy
type Client struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"nodeId"`
	Type      string    `json:"type"` // "CDN" or "PROXY"
	StartTime time.Time `json:"startTime"`
	Status    string    `json:"status"`
}

// LogEntry represents a log entry from any service
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Service   string    `json:"service"`
	Message   string    `json:"message"`
}

// ServiceStats represents statistics for a service
type ServiceStats struct {
	Type        string  `json:"type"`
	NodeCount   int     `json:"nodeCount"`
	ClientCount int     `json:"clientCount"`
	Latency     int     `json:"latency"`    // in milliseconds
	PacketLoss  float64 `json:"packetLoss"` // percentage
	Bandwidth   int     `json:"bandwidth"`  // in Mbps
}

// ProxyStats represents detailed statistics for Proxy service with unicast/broadcast split
type ProxyStats struct {
	ServiceStats
	UnicastBandwidth   float64                `json:"unicastBandwidth"`   // in Mbps
	BroadcastBandwidth float64                `json:"broadcastBandwidth"` // in Mbps
	UnicastPercent     float64                `json:"unicastPercent"`     // percentage
	BroadcastPercent   float64                `json:"broadcastPercent"`   // percentage
	FLUTEStats         map[string]interface{} `json:"fluteStats"`
}

// RaptorFEC represents Raptor FEC implementation
type RaptorFEC struct {
	SourceSymbols int
	RepairSymbols int
	SymbolSize    int
}

// FLUTEDelivery represents FLUTE delivery system
type FLUTEDelivery struct {
	SessionID     string
	TransportID   string
	SourceIP      string
	MulticastAddr string
	Port          int
}

// ProxyNode represents a proxy node with FLUTE delivery
type ProxyNode struct {
	ID      string
	Clients map[string]*Client
	FEC     *RaptorFEC
	FLUTE   *FLUTEDelivery
	mu      interface{} // sync.RWMutex placeholder
}

// VideoChunk represents a video data chunk
type VideoChunk struct {
	ID        string
	Data      []byte
	Sequence  int
	Timestamp time.Time
}
