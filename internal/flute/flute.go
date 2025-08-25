package flute

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// FLUTESession represents a FLUTE session
type FLUTESession struct {
	SessionID     string
	TransportID   string
	SourceIP      string
	MulticastAddr string
	Port          int
	TTL           int
	conn          *net.UDPConn
	mu            sync.RWMutex
	stats         *FLUTEStats
}

// FLUTEStats tracks FLUTE delivery statistics
type FLUTEStats struct {
	PacketsSent     uint64
	BytesSent       uint64
	PacketsReceived uint64
	BytesReceived   uint64
	Errors          uint64
	mu              sync.RWMutex
}

// FLUTEPacket represents a FLUTE packet
type FLUTEPacket struct {
	Header    FLUTEPacketHeader
	Payload   []byte
	Timestamp time.Time
}

// FLUTEPacketHeader represents FLUTE packet header
type FLUTEPacketHeader struct {
	Version     uint8
	Type        uint8
	SessionID   [16]byte
	SequenceNum uint32
	PayloadLen  uint16
	Checksum    uint16
}

// NewFLUTESession creates a new FLUTE session
func NewFLUTESession(sessionID, transportID, sourceIP, multicastAddr string, port, ttl int) *FLUTESession {
	return &FLUTESession{
		SessionID:     sessionID,
		TransportID:   transportID,
		SourceIP:      sourceIP,
		MulticastAddr: multicastAddr,
		Port:          port,
		TTL:           ttl,
		stats:         &FLUTEStats{},
	}
}

// Start starts the FLUTE session
func (fs *FLUTESession) Start() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Resolve multicast address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", fs.MulticastAddr, fs.Port))
	if err != nil {
		return fmt.Errorf("failed to resolve multicast address: %v", err)
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %v", err)
	}

	// Note: SetTTL is not available on all platforms, so we'll skip it for now
	// In a real implementation, you would use platform-specific methods

	fs.conn = conn
	log.Printf("FLUTE session started: %s -> %s:%d (TTL: %d)", fs.SessionID, fs.MulticastAddr, fs.Port, fs.TTL)
	return nil
}

// Stop stops the FLUTE session
func (fs *FLUTESession) Stop() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.conn != nil {
		err := fs.conn.Close()
		fs.conn = nil
		log.Printf("FLUTE session stopped: %s", fs.SessionID)
		return err
	}
	return nil
}

// SendPacket sends a FLUTE packet
func (fs *FLUTESession) SendPacket(payload []byte, sequenceNum uint32) error {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if fs.conn == nil {
		return fmt.Errorf("FLUTE session not started")
	}

	// Create packet header
	header := FLUTEPacketHeader{
		Version:     1,
		Type:        0, // Data packet
		SequenceNum: sequenceNum,
		PayloadLen:  uint16(len(payload)),
	}

	// Copy session ID
	copy(header.SessionID[:], fs.SessionID)

	// Calculate checksum
	header.Checksum = fs.calculateChecksum(payload)

	// Serialize header
	headerBytes := make([]byte, 32)
	headerBytes[0] = header.Version
	headerBytes[1] = header.Type
	copy(headerBytes[2:18], header.SessionID[:])
	binary.BigEndian.PutUint32(headerBytes[18:22], header.SequenceNum)
	binary.BigEndian.PutUint16(headerBytes[22:24], header.PayloadLen)
	binary.BigEndian.PutUint16(headerBytes[24:26], header.Checksum)

	// Combine header and payload
	packet := append(headerBytes, payload...)

	// Send packet
	_, err := fs.conn.Write(packet)
	if err != nil {
		fs.stats.mu.Lock()
		fs.stats.Errors++
		fs.stats.mu.Unlock()
		return fmt.Errorf("failed to send FLUTE packet: %v", err)
	}

	// Update statistics
	fs.stats.mu.Lock()
	fs.stats.PacketsSent++
	fs.stats.BytesSent += uint64(len(packet))
	fs.stats.mu.Unlock()

	log.Printf("FLUTE packet sent: session=%s, seq=%d, size=%d bytes", fs.SessionID, sequenceNum, len(packet))
	return nil
}

// calculateChecksum calculates a simple checksum for the payload
func (fs *FLUTESession) calculateChecksum(data []byte) uint16 {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	return uint16(sum & 0xFFFF)
}

// GetStats returns FLUTE statistics
func (fs *FLUTESession) GetStats() FLUTEStats {
	fs.stats.mu.RLock()
	defer fs.stats.mu.RUnlock()
	return *fs.stats
}

// FLUTEReceiver represents a FLUTE receiver
type FLUTEReceiver struct {
	MulticastAddr string
	Port          int
	conn          *net.UDPConn
	mu            sync.RWMutex
	stats         *FLUTEStats
	handlers      map[string]func([]byte, uint32) // sessionID -> handler
}

// NewFLUTEReceiver creates a new FLUTE receiver
func NewFLUTEReceiver(multicastAddr string, port int) *FLUTEReceiver {
	return &FLUTEReceiver{
		MulticastAddr: multicastAddr,
		Port:          port,
		stats:         &FLUTEStats{},
		handlers:      make(map[string]func([]byte, uint32)),
	}
}

// Start starts the FLUTE receiver
func (fr *FLUTEReceiver) Start() error {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Resolve multicast address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", fr.MulticastAddr, fr.Port))
	if err != nil {
		return fmt.Errorf("failed to resolve multicast address: %v", err)
	}

	// Create UDP connection
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to create multicast listener: %v", err)
	}

	fr.conn = conn

	// Start receiving packets
	go fr.receiveLoop()

	log.Printf("FLUTE receiver started: %s:%d", fr.MulticastAddr, fr.Port)
	return nil
}

// Stop stops the FLUTE receiver
func (fr *FLUTEReceiver) Stop() error {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	if fr.conn != nil {
		err := fr.conn.Close()
		fr.conn = nil
		log.Printf("FLUTE receiver stopped: %s:%d", fr.MulticastAddr, fr.Port)
		return err
	}
	return nil
}

// RegisterHandler registers a handler for a session
func (fr *FLUTEReceiver) RegisterHandler(sessionID string, handler func([]byte, uint32)) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.handlers[sessionID] = handler
}

// receiveLoop continuously receives FLUTE packets
func (fr *FLUTEReceiver) receiveLoop() {
	buffer := make([]byte, 65536)
	for {
		fr.mu.RLock()
		conn := fr.conn
		fr.mu.RUnlock()

		if conn == nil {
			break
		}

		// Set read timeout
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			fr.stats.mu.Lock()
			fr.stats.Errors++
			fr.stats.mu.Unlock()
			log.Printf("FLUTE receive error: %v", err)
			continue
		}

		// Parse packet
		if n < 32 {
			continue // Packet too short
		}

		// Parse header
		version := buffer[0]
		packetType := buffer[1]
		sessionID := string(buffer[2:18])
		sequenceNum := binary.BigEndian.Uint32(buffer[18:22])
		payloadLen := binary.BigEndian.Uint16(buffer[22:24])
		_ = binary.BigEndian.Uint16(buffer[24:26]) // checksum - not used for now

		// Validate packet
		if version != 1 || packetType != 0 {
			continue
		}

		if int(payloadLen) > n-32 {
			continue // Payload too short
		}

		// Extract payload
		payload := buffer[32 : 32+payloadLen]

		// Update statistics
		fr.stats.mu.Lock()
		fr.stats.PacketsReceived++
		fr.stats.BytesReceived += uint64(n)
		fr.stats.mu.Unlock()

		// Call handler
		fr.mu.RLock()
		handler, exists := fr.handlers[sessionID]
		fr.mu.RUnlock()

		if exists {
			handler(payload, sequenceNum)
		}

		log.Printf("FLUTE packet received: session=%s, seq=%d, size=%d bytes", sessionID, sequenceNum, len(payload))
	}
}

// GetStats returns FLUTE receiver statistics
func (fr *FLUTEReceiver) GetStats() FLUTEStats {
	fr.stats.mu.RLock()
	defer fr.stats.mu.RUnlock()
	return *fr.stats
}
