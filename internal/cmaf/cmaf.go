package cmaf

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// CMAFChunk represents a video chunk in CMAF format
type CMAFChunk struct {
	ID          string
	Index       int
	StartTime   time.Time
	Duration    time.Duration
	Data        []byte
	Size        int
	SegmentType string // "init", "media", "fragment"
	Codec       string
	Bandwidth   int
}

// CMAFStream represents a CMAF video stream
type CMAFStream struct {
	ID            string
	SourceFile    string
	ChunkDuration time.Duration
	ChunkSize     int
	Codec         string
	Bandwidth     int
	mu            sync.RWMutex
	chunks        []*CMAFChunk
	currentChunk  int
	loop          bool
}

// NewCMAFStream creates a new CMAF stream
func NewCMAFStream(id, sourceFile string, chunkDuration time.Duration, codec string, bandwidth int, loop bool) *CMAFStream {
	return &CMAFStream{
		ID:            id,
		SourceFile:    sourceFile,
		ChunkDuration: chunkDuration,
		Codec:         codec,
		Bandwidth:     bandwidth,
		loop:          loop,
	}
}

// Initialize sets up the CMAF stream and creates chunks
func (cs *CMAFStream) Initialize() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Check if source file exists
	if _, err := os.Stat(cs.SourceFile); os.IsNotExist(err) {
		return fmt.Errorf("source file not found: %s", cs.SourceFile)
	}

	// Read source file
	file, err := os.Open(cs.SourceFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return err
	}

	fileSize := stat.Size()
	log.Printf("CMAF stream initializing: %s, file size: %d bytes", cs.ID, fileSize)

	// Create chunks from the source file
	err = cs.createChunks(file, fileSize)
	if err != nil {
		return err
	}

	log.Printf("CMAF stream initialized: %s, %d chunks created", cs.ID, len(cs.chunks))
	return nil
}

// createChunks splits the source file into CMAF chunks
func (cs *CMAFStream) createChunks(file *os.File, fileSize int64) error {
	// Calculate chunk size based on bandwidth and duration
	// For simulation: assume 1MB per second at given bandwidth
	bytesPerSecond := cs.Bandwidth * 1024 * 1024 / 8 // Convert Mbps to bytes/s
	chunkSize := int(float64(bytesPerSecond) * cs.ChunkDuration.Seconds())

	if chunkSize == 0 {
		chunkSize = 1024 * 1024 // Default 1MB chunks
	}

	cs.ChunkSize = chunkSize

	// Create chunks
	chunkIndex := 0
	startTime := time.Now()

	for {
		chunk := make([]byte, chunkSize)
		n, err := file.Read(chunk)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading chunk %d: %v", chunkIndex, err)
		}

		// Trim chunk to actual size read
		chunk = chunk[:n]

		// Create chunk hash for ID
		hash := md5.Sum(chunk)
		chunkID := hex.EncodeToString(hash[:])

		// Determine segment type
		segmentType := "media"
		if chunkIndex == 0 {
			segmentType = "init"
		}

		cmafChunk := &CMAFChunk{
			ID:          chunkID,
			Index:       chunkIndex,
			StartTime:   startTime.Add(time.Duration(chunkIndex) * cs.ChunkDuration),
			Duration:    cs.ChunkDuration,
			Data:        chunk,
			Size:        n,
			SegmentType: segmentType,
			Codec:       cs.Codec,
			Bandwidth:   cs.Bandwidth,
		}

		cs.chunks = append(cs.chunks, cmafChunk)
		chunkIndex++

		// If we've read the entire file, break
		if int64(chunkIndex*chunkSize) >= fileSize {
			break
		}
	}

	return nil
}

// GetNextChunk returns the next chunk in the stream
func (cs *CMAFStream) GetNextChunk() (*CMAFChunk, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(cs.chunks) == 0 {
		return nil, fmt.Errorf("no chunks available")
	}

	// Check if we've reached the end
	if cs.currentChunk >= len(cs.chunks) {
		if cs.loop {
			// Loop back to beginning
			cs.currentChunk = 0
		} else {
			return nil, fmt.Errorf("end of stream")
		}
	}

	chunk := cs.chunks[cs.currentChunk]
	cs.currentChunk++

	// Update chunk start time to current time for live streaming
	chunk.StartTime = time.Now()

	// Write chunk to origin folder for tracking
	cs.writeChunkToFile(chunk, "origin")

	return chunk, nil
}

// writeChunkToFile writes a chunk to the specified output folder
func (cs *CMAFStream) writeChunkToFile(chunk *CMAFChunk, outputType string) {
	outputDir := fmt.Sprintf("%s_output", outputType)
	if outputType == "origin" {
		outputDir = "origin"
	}

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
		log.Printf("Wrote chunk %s to %s (size: %d bytes)", chunk.ID[:8], filename, chunk.Size)
	}
}

// GetChunkByIndex returns a specific chunk by index
func (cs *CMAFStream) GetChunkByIndex(index int) (*CMAFChunk, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if index < 0 || index >= len(cs.chunks) {
		return nil, fmt.Errorf("chunk index out of range: %d", index)
	}

	return cs.chunks[index], nil
}

// GetManifest generates a CMAF manifest (MPD for DASH or M3U8 for HLS)
func (cs *CMAFStream) GetManifest(format string) (string, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	switch format {
	case "dash":
		return cs.generateMPD(), nil
	case "hls":
		return cs.generateM3U8(), nil
	default:
		return "", fmt.Errorf("unsupported manifest format: %s", format)
	}
}

// generateMPD generates a DASH MPD manifest
func (cs *CMAFStream) generateMPD() string {
	mpd := `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" profiles="urn:mpeg:dash:profile:isoff-live:2011" type="dynamic" availabilityStartTime="` + time.Now().Format(time.RFC3339) + `" minBufferTime="PT2S">
  <Period>
    <AdaptationSet mimeType="video/mp4" segmentAlignment="true" startWithSAP="1">
      <SegmentTemplate timescale="1000" media="$RepresentationID$/chunk_$Number$.m4s" initialization="$RepresentationID$/init.m4s" duration="` + fmt.Sprintf("%.0f", cs.ChunkDuration.Milliseconds()) + `" startNumber="1"/>
      <Representation id="video" bandwidth="` + fmt.Sprintf("%d", cs.Bandwidth*1000000) + `" codecs="avc1.640028"/>
    </AdaptationSet>
  </Period>
</MPD>`
	return mpd
}

// generateM3U8 generates an HLS M3U8 manifest
func (cs *CMAFStream) generateM3U8() string {
	m3u8 := "#EXTM3U\n"
	m3u8 += "#EXT-X-VERSION:3\n"
	m3u8 += "#EXT-X-TARGETDURATION:" + fmt.Sprintf("%.0f", cs.ChunkDuration.Seconds()) + "\n"
	m3u8 += "#EXT-X-MEDIA-SEQUENCE:1\n"
	m3u8 += "#EXT-X-PLAYLIST-TYPE:EVENT\n"

	for i := range cs.chunks {
		m3u8 += fmt.Sprintf("#EXTINF:%.3f,\n", cs.ChunkDuration.Seconds())
		m3u8 += fmt.Sprintf("chunk_%d.m4s\n", i+1)
	}

	m3u8 += "#EXT-X-ENDLIST\n"
	return m3u8
}

// GetStats returns CMAF stream statistics
func (cs *CMAFStream) GetStats() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	totalSize := 0
	for _, chunk := range cs.chunks {
		totalSize += chunk.Size
	}

	return map[string]interface{}{
		"id":            cs.ID,
		"chunkCount":    len(cs.chunks),
		"totalSize":     totalSize,
		"chunkSize":     cs.ChunkSize,
		"chunkDuration": cs.ChunkDuration.String(),
		"codec":         cs.Codec,
		"bandwidth":     cs.Bandwidth,
		"currentChunk":  cs.currentChunk,
	}
}
