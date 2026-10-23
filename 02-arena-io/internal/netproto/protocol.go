package netproto

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
)

const (
	MsgClientInput  uint8 = 0x01
	MsgServerUpdate uint8 = 0x02
	HeaderSize            = 11
	EntityRecordLen       = 18
	MaxPacketSize         = 4096
)

var (
	ErrPacketTooShort = errors.New("packet length is insufficient")
	ErrUnknownType    = errors.New("unknown packet type")
)

type ClientInput struct {
	Sequence uint32
	TargetX  float32
	TargetY  float32
}

func DecodeClientInput(b []byte) (ClientInput, error) {
	if len(b) < 13 {
		return ClientInput{}, ErrPacketTooShort
	}
	if b[0] != MsgClientInput {
		return ClientInput{}, ErrUnknownType
	}

	seq := binary.LittleEndian.Uint32(b[1:5])
	bitsX := binary.LittleEndian.Uint32(b[5:9])
	bitsY := binary.LittleEndian.Uint32(b[9:13])

	return ClientInput{
		Sequence: seq,
		TargetX:  math.Float32frombits(bitsX),
		TargetY:  math.Float32frombits(bitsY),
	}, nil
}

func EncodeClientInput(dst []byte, seq uint32, targetX, targetY float32) []byte {
	if len(dst) < 13 {
		dst = make([]byte, 13)
	}
	dst[0] = MsgClientInput
	binary.LittleEndian.PutUint32(dst[1:5], seq)
	binary.LittleEndian.PutUint32(dst[5:9], math.Float32bits(targetX))
	binary.LittleEndian.PutUint32(dst[9:13], math.Float32bits(targetY))
	return dst[:13]
}

var BufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, MaxPacketSize)
		return &buf
	},
}

type EntitySnapshot struct {
	ID     uint32
	X      float32
	Y      float32
	Radius float32
	Mass   uint16
}

func EncodeSnapshot(dst []byte, serverTick uint32, ackSeq uint32, entities []EntitySnapshot) ([]byte, error) {
	requiredLen := HeaderSize + len(entities)*EntityRecordLen
	if len(dst) < requiredLen {
		return nil, errors.New("destination buffer too small")
	}

	dst[0] = MsgServerUpdate
	binary.LittleEndian.PutUint32(dst[1:5], serverTick)
	binary.LittleEndian.PutUint32(dst[5:9], ackSeq)
	binary.LittleEndian.PutUint16(dst[9:11], uint16(len(entities)))

	offset := HeaderSize
	for _, e := range entities {
		binary.LittleEndian.PutUint32(dst[offset:offset+4], e.ID)
		binary.LittleEndian.PutUint32(dst[offset+4:offset+8], math.Float32bits(e.X))
		binary.LittleEndian.PutUint32(dst[offset+8:offset+12], math.Float32bits(e.Y))
		binary.LittleEndian.PutUint32(dst[offset+12:offset+16], math.Float32bits(e.Radius))
		binary.LittleEndian.PutUint16(dst[offset+16:offset+18], e.Mass)
		offset += EntityRecordLen
	}

	return dst[:requiredLen], nil
}
