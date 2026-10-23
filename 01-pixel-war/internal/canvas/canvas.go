package canvas

import (
	"fmt"
	"sync"
)

const (
	// DefaultWidth is the default canvas width in pixels.
	DefaultWidth uint16 = 1000

	// DefaultHeight is the default canvas height in pixels.
	DefaultHeight uint16 = 1000

	// DefaultNumStripes is the number of striped RWMutexes for sharded concurrency.
	// 64 stripes drastically reduces lock contention under massive parallel writes.
	DefaultNumStripes = 64
)

// Canvas defines the public interface for thread-safe canvas operations.
type Canvas interface {
	Width() uint16
	Height() uint16
	TotalPixels() int
	SetPixel(x, y uint16, color uint8) (oldColor uint8, changed bool, err error)
	GetPixel(x, y uint16) (uint8, error)
	Snapshot() []byte
	LoadSnapshot(data []byte) error
}

// MemoryCanvas is a high-performance, sharded, zero-allocation in-memory canvas.
// Total RAM consumption for 1000x1000 is ~1MB.
type MemoryCanvas struct {
	width      uint16
	height     uint16
	total      int
	numStripes uint32
	stripes    []sync.RWMutex
	buffer     []byte
}

// New creates a new in-memory canvas initialized to white (color index 31).
func New(width, height uint16) *MemoryCanvas {
	if width == 0 {
		width = DefaultWidth
	}
	if height == 0 {
		height = DefaultHeight
	}

	total := int(width) * int(height)
	buf := make([]byte, total)
	// Initialize default color to white (palette index 31)
	for i := range buf {
		buf[i] = MaxColorIndex
	}

	return &MemoryCanvas{
		width:      width,
		height:     height,
		total:      total,
		numStripes: DefaultNumStripes,
		stripes:    make([]sync.RWMutex, DefaultNumStripes),
		buffer:     buf,
	}
}

// Width returns canvas width.
func (c *MemoryCanvas) Width() uint16 {
	return c.width
}

// Height returns canvas height.
func (c *MemoryCanvas) Height() uint16 {
	return c.height
}

// TotalPixels returns the total number of pixels.
func (c *MemoryCanvas) TotalPixels() int {
	return c.total
}

// getStripeIndex maps an offset to its corresponding stripe mutex.
func (c *MemoryCanvas) getStripeIndex(offset int) uint32 {
	return uint32(offset) % c.numStripes
}

// SetPixel atomically updates the color of the pixel at (x, y).
// It returns the previous color, whether the pixel changed value, and any validation errors.
// Designed with 0 heap allocations on the hot path.
func (c *MemoryCanvas) SetPixel(x, y uint16, color uint8) (oldColor uint8, changed bool, err error) {
	if x >= c.width || y >= c.height {
		return 0, false, ErrOutOfBounds
	}
	if color > MaxColorIndex {
		return 0, false, ErrInvalidColor
	}

	offset := int(y)*int(c.width) + int(x)
	stripe := c.getStripeIndex(offset)

	c.stripes[stripe].Lock()
	oldColor = c.buffer[offset]
	if oldColor == color {
		c.stripes[stripe].Unlock()
		return oldColor, false, nil
	}

	c.buffer[offset] = color
	c.stripes[stripe].Unlock()
	return oldColor, true, nil
}

// GetPixel reads the color index of the pixel at (x, y).
func (c *MemoryCanvas) GetPixel(x, y uint16) (uint8, error) {
	if x >= c.width || y >= c.height {
		return 0, ErrOutOfBounds
	}

	offset := int(y)*int(c.width) + int(x)
	stripe := c.getStripeIndex(offset)

	c.stripes[stripe].RLock()
	color := c.buffer[offset]
	c.stripes[stripe].RUnlock()
	return color, nil
}

// Snapshot creates a thread-safe, consistent byte copy of the entire canvas.
// For 1000x1000, this copies ~1MB of raw bytes.
func (c *MemoryCanvas) Snapshot() []byte {
	// Acquire read lock across all stripes to guarantee an atomic snapshot
	for i := uint32(0); i < c.numStripes; i++ {
		c.stripes[i].RLock()
	}

	snap := make([]byte, c.total)
	copy(snap, c.buffer)

	for i := uint32(0); i < c.numStripes; i++ {
		c.stripes[i].RUnlock()
	}

	return snap
}

// LoadSnapshot restores the canvas state from a raw byte snapshot.
func (c *MemoryCanvas) LoadSnapshot(data []byte) error {
	if len(data) != c.total {
		return fmt.Errorf("snapshot size mismatch: expected %d bytes, got %d", c.total, len(data))
	}

	// Acquire write lock across all stripes
	for i := uint32(0); i < c.numStripes; i++ {
		c.stripes[i].Lock()
	}

	copy(c.buffer, data)

	for i := uint32(0); i < c.numStripes; i++ {
		c.stripes[i].Unlock()
	}

	return nil
}
