package canvas

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var (
	// ErrOutOfBounds indicates the requested coordinates fall outside canvas dimensions.
	ErrOutOfBounds = errors.New("pixel coordinates are out of bounds")

	// ErrInvalidColor indicates the color index exceeds the supported palette range.
	ErrInvalidColor = errors.New("color index is out of palette range")

	// ErrInvalidPayloadSize indicates the binary payload does not match expected byte length.
	ErrInvalidPayloadSize = errors.New("invalid binary payload size for pixel update")
)

// UpdatePacketSize is the fixed size in bytes for a single pixel delta.
// [X: uint16 (2 bytes)][Y: uint16 (2 bytes)][Color: uint8 (1 byte)] = 5 bytes.
const UpdatePacketSize = 5

// MaxColorIndex defines the maximum index for the curated 32-color palette (0 to 31).
const MaxColorIndex uint8 = 31

// ColorRGB represents a 24-bit RGB color.
type ColorRGB struct {
	R uint8
	G uint8
	B uint8
}

// Hex returns the 6-character hexadecimal representation (e.g., "#6D001A").
func (c ColorRGB) Hex() string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// Palette is the official 32-color curated palette inspired by Reddit r/place.
// Each color index (0-31) maps directly to an exact RGB value.
var Palette = [32]ColorRGB{
	0:  {R: 0x6D, G: 0x00, B: 0x1A}, // Burgundy
	1:  {R: 0xBE, G: 0x00, B: 0x39}, // Dark Red
	2:  {R: 0xFF, G: 0x45, B: 0x00}, // Red / Orange-Red
	3:  {R: 0xFF, G: 0xA8, B: 0x00}, // Warm Orange
	4:  {R: 0xFF, G: 0xD6, B: 0x35}, // Yellow
	5:  {R: 0xFF, G: 0xF8, B: 0xB8}, // Pale Yellow
	6:  {R: 0x00, G: 0xA3, B: 0x68}, // Dark Green
	7:  {R: 0x00, G: 0xCC, B: 0x78}, // Green
	8:  {R: 0x7E, G: 0xED, B: 0x56}, // Light Green
	9:  {R: 0x00, G: 0x75, B: 0x6F}, // Dark Teal
	10: {R: 0x00, G: 0x9E, B: 0xAA}, // Teal
	11: {R: 0x00, G: 0xCC, B: 0xC0}, // Light Teal
	12: {R: 0x24, G: 0x50, B: 0xA4}, // Dark Blue
	13: {R: 0x36, G: 0x90, B: 0xEA}, // Blue
	14: {R: 0x51, G: 0xE9, B: 0xF4}, // Cyan
	15: {R: 0x49, G: 0x3A, B: 0xC1}, // Indigo
	16: {R: 0x6A, G: 0x5C, B: 0xFF}, // Periwinkle
	17: {R: 0x94, G: 0xB3, B: 0xFF}, // Lavender
	18: {R: 0x81, G: 0x1E, B: 0x9F}, // Dark Purple
	19: {R: 0xB4, G: 0x4A, B: 0xC0}, // Purple
	20: {R: 0xE4, G: 0xAB, B: 0xFF}, // Pale Purple
	21: {R: 0xDE, G: 0x10, B: 0x7F}, // Magenta
	22: {R: 0xFF, G: 0x38, B: 0x81}, // Pink
	23: {R: 0xFF, G: 0x99, B: 0xAA}, // Light Pink
	24: {R: 0x6D, G: 0x48, B: 0x2F}, // Dark Brown
	25: {R: 0x9C, G: 0x69, B: 0x26}, // Brown
	26: {R: 0xFF, G: 0xB4, B: 0x70}, // Light Brown
	27: {R: 0x00, G: 0x00, B: 0x00}, // Black
	28: {R: 0x51, G: 0x52, B: 0x52}, // Dark Gray
	29: {R: 0x89, G: 0x8D, B: 0x90}, // Gray
	30: {R: 0xD4, G: 0xD7, B: 0xD9}, // Light Gray
	31: {R: 0xFF, G: 0xFF, B: 0xFF}, // White
}

// PixelUpdate models a single pixel change event.
type PixelUpdate struct {
	X     uint16 `json:"x"`
	Y     uint16 `json:"y"`
	Color uint8  `json:"color"`
}

// EncodeBinary encodes a PixelUpdate into a zero-heap 5-byte slice.
// Format: [X: uint16 BigEndian (2B)][Y: uint16 BigEndian (2B)][Color: uint8 (1B)]
func (p PixelUpdate) EncodeBinary(buf []byte) []byte {
	if cap(buf) < UpdatePacketSize {
		buf = make([]byte, UpdatePacketSize)
	} else {
		buf = buf[:UpdatePacketSize]
	}
	binary.BigEndian.PutUint16(buf[0:2], p.X)
	binary.BigEndian.PutUint16(buf[2:4], p.Y)
	buf[4] = p.Color
	return buf
}

// DecodeBinary decodes a 5-byte slice into a PixelUpdate.
func DecodeBinary(b []byte) (PixelUpdate, error) {
	if len(b) < UpdatePacketSize {
		return PixelUpdate{}, ErrInvalidPayloadSize
	}
	x := binary.BigEndian.Uint16(b[0:2])
	y := binary.BigEndian.Uint16(b[2:4])
	color := b[4]
	if color > MaxColorIndex {
		return PixelUpdate{}, ErrInvalidColor
	}
	return PixelUpdate{
		X:     x,
		Y:     y,
		Color: color,
	}, nil
}
