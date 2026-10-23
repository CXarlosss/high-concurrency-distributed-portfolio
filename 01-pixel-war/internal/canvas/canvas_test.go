package canvas

import (
	"math/rand"
	"sync"
	"testing"
)

func TestCanvas_Initialization(t *testing.T) {
	c := New(100, 100)
	if c.Width() != 100 || c.Height() != 100 {
		t.Fatalf("expected 100x100, got %dx%d", c.Width(), c.Height())
	}
	if c.TotalPixels() != 10000 {
		t.Fatalf("expected 10000 total pixels, got %d", c.TotalPixels())
	}

	// Verify all pixels are initialized to default white (MaxColorIndex = 31)
	for y := uint16(0); y < 100; y++ {
		for x := uint16(0); x < 100; x++ {
			color, err := c.GetPixel(x, y)
			if err != nil {
				t.Fatalf("unexpected error at (%d, %d): %v", x, y, err)
			}
			if color != MaxColorIndex {
				t.Fatalf("expected pixel at (%d, %d) to be %d, got %d", x, y, MaxColorIndex, color)
			}
		}
	}
}

func TestCanvas_BoundsChecking(t *testing.T) {
	c := New(10, 10)

	// Valid bounds
	if _, _, err := c.SetPixel(9, 9, 5); err != nil {
		t.Fatalf("expected valid coordinate, got error: %v", err)
	}

	// Out of bounds X
	if _, _, err := c.SetPixel(10, 5, 5); err != ErrOutOfBounds {
		t.Fatalf("expected ErrOutOfBounds for X=10, got: %v", err)
	}

	// Out of bounds Y
	if _, _, err := c.SetPixel(5, 10, 5); err != ErrOutOfBounds {
		t.Fatalf("expected ErrOutOfBounds for Y=10, got: %v", err)
	}

	// GetPixel out of bounds
	if _, err := c.GetPixel(15, 2); err != ErrOutOfBounds {
		t.Fatalf("expected ErrOutOfBounds on GetPixel, got: %v", err)
	}
}

func TestCanvas_ColorValidation(t *testing.T) {
	c := New(10, 10)

	// Invalid color index
	if _, _, err := c.SetPixel(0, 0, 32); err != ErrInvalidColor {
		t.Fatalf("expected ErrInvalidColor for color 32, got: %v", err)
	}

	// Valid color index boundary
	if _, _, err := c.SetPixel(0, 0, 31); err != nil {
		t.Fatalf("expected valid color 31, got error: %v", err)
	}
}

func TestCanvas_SetAndGetPixel(t *testing.T) {
	c := New(50, 50)

	// Initial set
	old, changed, err := c.SetPixel(12, 34, 7)
	if err != nil {
		t.Fatalf("SetPixel failed: %v", err)
	}
	if old != 31 {
		t.Fatalf("expected old color to be 31, got %d", old)
	}
	if !changed {
		t.Fatalf("expected changed to be true")
	}

	// Read back
	val, err := c.GetPixel(12, 34)
	if err != nil {
		t.Fatalf("GetPixel failed: %v", err)
	}
	if val != 7 {
		t.Fatalf("expected pixel value 7, got %d", val)
	}

	// Set to same color (idempotency check)
	old2, changed2, err := c.SetPixel(12, 34, 7)
	if err != nil {
		t.Fatalf("SetPixel failed: %v", err)
	}
	if old2 != 7 {
		t.Fatalf("expected old color 7, got %d", old2)
	}
	if changed2 {
		t.Fatalf("expected changed to be false for identical color")
	}
}

func TestCanvas_BinaryProtocol(t *testing.T) {
	update := PixelUpdate{
		X:     999,
		Y:     456,
		Color: 15,
	}

	var buf [UpdatePacketSize]byte
	encoded := update.EncodeBinary(buf[:])
	if len(encoded) != UpdatePacketSize {
		t.Fatalf("expected encoded length %d, got %d", UpdatePacketSize, len(encoded))
	}

	decoded, err := DecodeBinary(encoded)
	if err != nil {
		t.Fatalf("failed to decode binary update: %v", err)
	}

	if decoded.X != update.X || decoded.Y != update.Y || decoded.Color != update.Color {
		t.Fatalf("decoded mismatch: expected %+v, got %+v", update, decoded)
	}

	// Test short payload
	_, err = DecodeBinary([]byte{1, 2, 3})
	if err != ErrInvalidPayloadSize {
		t.Fatalf("expected ErrInvalidPayloadSize for 3 bytes, got: %v", err)
	}

	// Test invalid color in binary payload
	badPayload := []byte{0, 1, 0, 1, 50} // color 50 > 31
	_, err = DecodeBinary(badPayload)
	if err != ErrInvalidColor {
		t.Fatalf("expected ErrInvalidColor for color 50, got: %v", err)
	}
}

func TestCanvas_SnapshotAndRestore(t *testing.T) {
	c1 := New(20, 20)
	c1.SetPixel(0, 0, 1)
	c1.SetPixel(5, 5, 2)
	c1.SetPixel(19, 19, 3)

	snap := c1.Snapshot()
	if len(snap) != 400 {
		t.Fatalf("expected snapshot size 400, got %d", len(snap))
	}

	c2 := New(20, 20)
	if err := c2.LoadSnapshot(snap); err != nil {
		t.Fatalf("failed to load snapshot: %v", err)
	}

	p1, _ := c2.GetPixel(0, 0)
	p2, _ := c2.GetPixel(5, 5)
	p3, _ := c2.GetPixel(19, 19)

	if p1 != 1 || p2 != 2 || p3 != 3 {
		t.Fatalf("restored pixel values mismatch: p1=%d, p2=%d, p3=%d", p1, p2, p3)
	}
}

func TestCanvas_ConcurrentAccess(t *testing.T) {
	c := New(100, 100)
	numGoroutines := 100
	opsPerGoroutine := 500

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(id)))
			for i := 0; i < opsPerGoroutine; i++ {
				x := uint16(rng.Intn(100))
				y := uint16(rng.Intn(100))
				color := uint8(rng.Intn(32))

				if i%2 == 0 {
					c.SetPixel(x, y, color)
				} else {
					c.GetPixel(x, y)
				}
			}
		}(g)
	}

	wg.Wait()
}

func BenchmarkSetPixel(b *testing.B) {
	c := New(1000, 1000)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.SetPixel(uint16(i%1000), uint16((i/1000)%1000), uint8(i%32))
	}
}

func BenchmarkGetPixel(b *testing.B) {
	c := New(1000, 1000)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.GetPixel(uint16(i%1000), uint16((i/1000)%1000))
	}
}

func BenchmarkConcurrentSetPixel(b *testing.B) {
	c := New(1000, 1000)
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			c.SetPixel(uint16(i%1000), uint16((i/1000)%1000), uint8(i%32))
			i++
		}
	})
}

func BenchmarkSnapshot(b *testing.B) {
	c := New(1000, 1000)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = c.Snapshot()
	}
}
