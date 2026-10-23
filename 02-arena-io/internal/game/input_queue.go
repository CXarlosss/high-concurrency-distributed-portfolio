package game

import "github.com/cv-projects/arena-io/internal/netproto"

const (
	InputBufferSize = 32
	InputBufferMask = InputBufferSize - 1
)

type InputRingBuffer struct {
	inputs        [InputBufferSize]netproto.ClientInput
	valid         [InputBufferSize]bool
	lastAckSeq    uint32
	highestRcvSeq uint32
}

func NewInputRingBuffer() *InputRingBuffer {
	return &InputRingBuffer{}
}

func (rb *InputRingBuffer) Push(input netproto.ClientInput) bool {
	if input.Sequence <= rb.lastAckSeq {
		return false
	}

	slot := input.Sequence & InputBufferMask
	if rb.valid[slot] && rb.inputs[slot].Sequence == input.Sequence {
		return false
	}

	rb.inputs[slot] = input
	rb.valid[slot] = true

	if input.Sequence > rb.highestRcvSeq {
		rb.highestRcvSeq = input.Sequence
	}
	return true
}

func (rb *InputRingBuffer) PollNext() (netproto.ClientInput, bool) {
	nextSeq := rb.lastAckSeq + 1
	slot := nextSeq & InputBufferMask

	if rb.valid[slot] && rb.inputs[slot].Sequence == nextSeq {
		input := rb.inputs[slot]
		rb.valid[slot] = false
		rb.lastAckSeq = nextSeq
		return input, true
	}

	return netproto.ClientInput{}, false
}

func (rb *InputRingBuffer) LastAckSeq() uint32 {
	return rb.lastAckSeq
}
