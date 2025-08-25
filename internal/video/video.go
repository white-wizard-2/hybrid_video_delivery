package video

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// VideoFrame represents a video frame with metadata
type VideoFrame struct {
	ID        uint64
	Timestamp time.Time
	Data      []byte
	Size      int
	Sequence  uint32
}

// VideoStream represents a video stream source
type VideoStream struct {
	filePath    string
	frameRate   int
	frameSize   int
	loop        bool
	mu          sync.RWMutex
	currentPos  int64
	totalFrames uint64
}

// NewVideoStream creates a new video stream from a file
func NewVideoStream(filePath string, frameRate int, loop bool) *VideoStream {
	return &VideoStream{
		filePath:  filePath,
		frameRate: frameRate,
		loop:      loop,
	}
}

// Initialize sets up the video stream
func (vs *VideoStream) Initialize() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(vs.filePath); os.IsNotExist(err) {
		return fmt.Errorf("video file not found: %s", vs.filePath)
	}

	// Get file size for frame calculation
	file, err := os.Open(vs.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// For MP4 files, we'll simulate frames based on file size
	// Assume each frame is roughly 1KB for simulation purposes
	vs.frameSize = 1024
	vs.totalFrames = uint64(stat.Size()) / uint64(vs.frameSize)
	if vs.totalFrames == 0 {
		vs.totalFrames = 1000 // Fallback to 1000 frames
	}

	log.Printf("Video stream initialized: %s, %d frames, %d fps (file size: %d bytes)",
		vs.filePath, vs.totalFrames, vs.frameRate, stat.Size())
	return nil
}

// GetNextFrame simulates reading the next frame from the video file
func (vs *VideoStream) GetNextFrame() (*VideoFrame, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Simulate frame reading from MP4 file
	// In a real implementation, you would use an MP4 parser library
	frameID := uint64(vs.currentPos / int64(vs.frameSize))

	// Check if we've reached the end
	if frameID >= vs.totalFrames {
		if vs.loop {
			// Loop back to beginning
			vs.currentPos = 0
			frameID = 0
		} else {
			return nil, fmt.Errorf("end of video stream")
		}
	}

	// Simulate frame data (in reality, this would be actual video frame data)
	frameData := make([]byte, vs.frameSize)
	for i := range frameData {
		frameData[i] = byte((int(frameID) + i) % 256)
	}

	// Create frame with simulated metadata
	frame := &VideoFrame{
		ID:        frameID,
		Timestamp: time.Now(),
		Data:      frameData,
		Size:      len(frameData),
		Sequence:  uint32(frameID),
	}

	// Move to next frame
	vs.currentPos += int64(vs.frameSize)

	return frame, nil
}

// GetFrameRate returns the frame rate
func (vs *VideoStream) GetFrameRate() int {
	return vs.frameRate
}

// GetTotalFrames returns the total number of frames
func (vs *VideoStream) GetTotalFrames() uint64 {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.totalFrames
}
