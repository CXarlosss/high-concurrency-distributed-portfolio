package game

import (
	"sync"

	"github.com/cv-projects/arena-io/internal/netproto"
)

const (
	TargetTPS       = 30
	FixedDeltaTime  = 1.0 / float64(TargetTPS)
	WorldWidth      = 3000.0
	WorldHeight     = 3000.0
	MaxPlayersLimit = 500
	SpeedTolerance  = 1.05
)

type World struct {
	mu           sync.RWMutex
	players      []Player
	playerIndex  map[uint32]int
	inputBuffers map[uint32]*InputRingBuffer
	grid         *SpatialGrid
	tickCount    uint32
}

func NewWorld() *World {
	return &World{
		players:      make([]Player, 0, MaxPlayersLimit),
		playerIndex:  make(map[uint32]int, MaxPlayersLimit),
		inputBuffers: make(map[uint32]*InputRingBuffer, MaxPlayersLimit),
		grid:         NewSpatialGrid(),
	}
}

func (w *World) TickCount() uint32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.tickCount
}

func (w *World) AddPlayer(id uint32, startPos Vec2) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.playerIndex[id]; exists {
		return
	}

	p := NewPlayer(id, startPos)
	idx := len(w.players)
	w.players = append(w.players, p)
	w.playerIndex[id] = idx
	w.inputBuffers[id] = NewInputRingBuffer()
}

func (w *World) RemovePlayer(id uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()

	idx, exists := w.playerIndex[id]
	if !exists {
		return
	}

	// Swap and pop to keep slice contiguous with 0 allocs
	lastIdx := len(w.players) - 1
	if idx != lastIdx {
		lastPlayer := w.players[lastIdx]
		w.players[idx] = lastPlayer
		w.playerIndex[lastPlayer.ID] = idx
	}

	w.players = w.players[:lastIdx]
	delete(w.playerIndex, id)
	delete(w.inputBuffers, id)
}

func (w *World) QueueInput(id uint32, in netproto.ClientInput) bool {
	w.mu.RLock()
	rb, exists := w.inputBuffers[id]
	w.mu.RUnlock()

	if !exists {
		return false
	}
	return rb.Push(in)
}

func (w *World) LastAckSeq(id uint32) uint32 {
	w.mu.RLock()
	rb, exists := w.inputBuffers[id]
	w.mu.RUnlock()

	if !exists {
		return 0
	}
	return rb.LastAckSeq()
}

func (w *World) clampToBounds(p *Player) {
	if p.Position.X < p.Radius {
		p.Position.X = p.Radius
	} else if p.Position.X > WorldWidth-p.Radius {
		p.Position.X = WorldWidth - p.Radius
	}

	if p.Position.Y < p.Radius {
		p.Position.Y = p.Radius
	} else if p.Position.Y > WorldHeight-p.Radius {
		p.Position.Y = WorldHeight - p.Radius
	}
}

func (w *World) ProcessPlayerInput(p *Player, input netproto.ClientInput, dt float64) {
	target := Vec2{X: float64(input.TargetX), Y: float64(input.TargetY)}
	dir := target.Sub(p.Position)

	if dir.LengthSq() < 1.0 {
		p.Velocity = Vec2{}
		return
	}

	normDir := dir.Normalize()
	p.Velocity = normDir.Mul(p.Speed)

	displacement := p.Velocity.Mul(dt)
	maxDist := p.Speed * dt * SpeedTolerance

	if displacement.Length() > maxDist {
		displacement = displacement.Normalize().Mul(p.Speed * dt)
	}

	p.Position = p.Position.Add(displacement)
	w.clampToBounds(p)
}

func (w *World) Update(dt float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.tickCount++

	// 1. Process inputs for each player (max 3 inputs per tick to prevent flood)
	for i := range w.players {
		p := &w.players[i]
		if !p.Alive {
			continue
		}

		if rb, ok := w.inputBuffers[p.ID]; ok {
			processed := 0
			for processed < 3 {
				in, hasInput := rb.PollNext()
				if !hasInput {
					break
				}
				w.ProcessPlayerInput(p, in, dt)
				processed++
			}
		}
	}

	// 2. Clear and rebuild spatial grid
	w.grid.Clear()
	for i := range w.players {
		p := &w.players[i]
		if p.Alive {
			w.grid.Insert(p.ID, p.Position)
		}
	}
}

// GetVisibleEntities populates entities slice with visible entities for a given player using AoI
func (w *World) GetVisibleEntities(playerID uint32, neighborBuf []uint32, out []netproto.EntitySnapshot) []netproto.EntitySnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()

	idx, exists := w.playerIndex[playerID]
	if !exists {
		return out[:0]
	}

	p := w.players[idx]
	neighbors := w.grid.QueryNeighbors(p.Position, neighborBuf)

	out = out[:0]
	for _, nID := range neighbors {
		if nIdx, ok := w.playerIndex[nID]; ok {
			np := w.players[nIdx]
			out = append(out, netproto.EntitySnapshot{
				ID:     np.ID,
				X:      float32(np.Position.X),
				Y:      float32(np.Position.Y),
				Radius: float32(np.Radius),
				Mass:   uint16(np.Mass * 10),
			})
		}
	}
	return out
}
