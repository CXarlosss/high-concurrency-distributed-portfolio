package netproto

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
)

const (
	MsgWelcome      uint8 = 0x00
	MsgClientInput  uint8 = 0x01
	MsgServerUpdate uint8 = 0x02
	MsgFoodInit     uint8 = 0x03
	MsgFoodEaten    uint8 = 0x04
	MsgKillEvent    uint8 = 0x05
	MsgClientAction uint8 = 0x06
	HeaderSize            = 11
	EntityRecordLen       = 22
	MaxPacketSize         = 4096
)

const (
	ActionSplit uint8 = 0x01
	ActionEject uint8 = 0x02
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
	ID      uint32
	OwnerID uint32
	X       float32
	Y       float32
	Radius  float32
	Mass    uint16
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
		binary.LittleEndian.PutUint32(dst[offset+4:offset+8], e.OwnerID)
		binary.LittleEndian.PutUint32(dst[offset+8:offset+12], math.Float32bits(e.X))
		binary.LittleEndian.PutUint32(dst[offset+12:offset+16], math.Float32bits(e.Y))
		binary.LittleEndian.PutUint32(dst[offset+16:offset+20], math.Float32bits(e.Radius))
		binary.LittleEndian.PutUint16(dst[offset+20:offset+22], e.Mass)
		offset += EntityRecordLen
	}

	return dst[:requiredLen], nil
}

type FoodPelletNet struct {
	X        uint16
	Y        uint16
	ColorIdx uint8
}

func EncodeFoodInit(dst []byte, pellets []FoodPelletNet) []byte {
	needed := 3 + len(pellets)*5
	if len(dst) < needed {
		dst = make([]byte, needed)
	}
	dst[0] = MsgFoodInit
	binary.LittleEndian.PutUint16(dst[1:3], uint16(len(pellets)))
	offset := 3
	for _, p := range pellets {
		binary.LittleEndian.PutUint16(dst[offset:offset+2], p.X)
		binary.LittleEndian.PutUint16(dst[offset+2:offset+4], p.Y)
		dst[offset+4] = p.ColorIdx
		offset += 5
	}
	return dst[:needed]
}

type FoodEaten struct {
	Index uint16
	NewX  uint16
	NewY  uint16
}

func EncodeFoodEaten(dst []byte, eaten []FoodEaten) []byte {
	needed := 2 + len(eaten)*6
	if len(dst) < needed {
		dst = make([]byte, needed)
	}
	dst[0] = MsgFoodEaten
	dst[1] = uint8(len(eaten))
	offset := 2
	for _, e := range eaten {
		binary.LittleEndian.PutUint16(dst[offset:offset+2], e.Index)
		binary.LittleEndian.PutUint16(dst[offset+2:offset+4], e.NewX)
		binary.LittleEndian.PutUint16(dst[offset+4:offset+6], e.NewY)
		offset += 6
	}
	return dst[:needed]
}

type KillEvent struct {
	WinnerID uint32
	VictimID uint32
	Points   uint16
}

func EncodeKillEvent(dst []byte, winnerID, victimID uint32, points uint16) []byte {
	if len(dst) < 11 {
		dst = make([]byte, 11)
	}
	dst[0] = MsgKillEvent
	binary.LittleEndian.PutUint32(dst[1:5], winnerID)
	binary.LittleEndian.PutUint32(dst[5:9], victimID)
	binary.LittleEndian.PutUint16(dst[9:11], points)
	return dst[:11]
}
