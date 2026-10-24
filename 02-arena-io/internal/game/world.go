package game

import (
	"math"
	"math/rand"
	"sync"

	"github.com/cv-projects/arena-io/internal/netproto"
)

const (
	TargetTPS       = 30
	FixedDeltaTime  = 1.0 / float64(TargetTPS)
	WorldWidth      = 3000.0
	WorldHeight     = 3000.0
	MaxPlayersLimit = 500
	SpeedTolerance  = 1.15
	TotalFoodCount  = 250
	BotIDStart      = 100000
	BotCountDefault = 20
)

type FoodPellet struct {
	X        uint16
	Y        uint16
	ColorIdx uint8
}

type World struct {
	mu           sync.RWMutex
	players      []Player
	playerIndex  map[uint32]int
	inputBuffers map[uint32]*InputRingBuffer
	grid         *SpatialGrid
	tickCount    uint32

	food        []FoodPellet
	recentEaten []netproto.FoodEaten
	recentKills []netproto.KillEvent
	botIDs      []uint32
}

func NewWorld() *World {
	w := &World{
		players:      make([]Player, 0, MaxPlayersLimit),
		playerIndex:  make(map[uint32]int, MaxPlayersLimit),
		inputBuffers: make(map[uint32]*InputRingBuffer, MaxPlayersLimit),
		grid:         NewSpatialGrid(),
		food:         make([]FoodPellet, TotalFoodCount),
		recentEaten:  make([]netproto.FoodEaten, 0, 32),
		recentKills:  make([]netproto.KillEvent, 0, 8),
		botIDs:       make([]uint32, 0, BotCountDefault),
	}

	// Initialize food pellets across the 3000x3000 canvas
	for i := 0; i < TotalFoodCount; i++ {
		w.food[i] = FoodPellet{
			X:        uint16(60 + rand.Intn(int(WorldWidth-120))),
			Y:        uint16(60 + rand.Intn(int(WorldHeight-120))),
			ColorIdx: uint8(i % 5),
		}
	}

	return w
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

// ToggleBots spawns or stops autonomous server-side bots
func (w *World) ToggleBots() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.botIDs) > 0 {
		// Stop all bots
		for _, bID := range w.botIDs {
			if idx, ok := w.playerIndex[bID]; ok {
				lastIdx := len(w.players) - 1
				if idx != lastIdx {
					lastPlayer := w.players[lastIdx]
					w.players[idx] = lastPlayer
					w.playerIndex[lastPlayer.ID] = idx
				}
				w.players = w.players[:lastIdx]
				delete(w.playerIndex, bID)
				delete(w.inputBuffers, bID)
			}
		}
		w.botIDs = w.botIDs[:0]
		return 0
	}

	// Spawn 20 autonomous bots
	for i := 0; i < BotCountDefault; i++ {
		botID := uint32(BotIDStart + i)
		spawnPos := Vec2{
			X: 250.0 + rand.Float64()*(WorldWidth-500.0),
			Y: 250.0 + rand.Float64()*(WorldHeight-500.0),
		}
		bot := NewBot(botID, spawnPos)
		idx := len(w.players)
		w.players = append(w.players, bot)
		w.playerIndex[botID] = idx
		w.inputBuffers[botID] = NewInputRingBuffer()
		w.botIDs = append(w.botIDs, botID)
	}

	return len(w.botIDs)
}

func (w *World) GetFoodPelletsNet() []netproto.FoodPelletNet {
	w.mu.RLock()
	defer w.mu.RUnlock()

	out := make([]netproto.FoodPelletNet, len(w.food))
	for i, f := range w.food {
		out[i] = netproto.FoodPelletNet{
			X:        f.X,
			Y:        f.Y,
			ColorIdx: f.ColorIdx,
		}
	}
	return out
}

func (w *World) FlushEvents() ([]netproto.FoodEaten, []netproto.KillEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()

	eaten := w.recentEaten
	kills := w.recentKills

	w.recentEaten = make([]netproto.FoodEaten, 0, 32)
	w.recentKills = make([]netproto.KillEvent, 0, 8)

	return eaten, kills
}

func (w *World) updateBotAI(b *Player) {
	b.BotTimer--
	if b.BotTimer > 0 {
		return
	}
	// Decide new steering target every 10-18 ticks (~0.4s)
	b.BotTimer = 10 + rand.Intn(8)

	// 1. Check for nearby larger players to FLEE
	var fleeDir Vec2
	shouldFlee := false

	for i := range w.players {
		other := &w.players[i]
		if other.ID == b.ID || !other.Alive {
			continue
		}
		if other.Mass > b.Mass*1.12 {
			diff := b.Position.Sub(other.Position)
			dist := diff.Length()
			if dist < b.Radius+140.0 {
				fleeDir = fleeDir.Add(diff.Normalize())
				shouldFlee = true
			}
		}
	}

	if shouldFlee && fleeDir.LengthSq() > 0.01 {
		fleeTarget := b.Position.Add(fleeDir.Normalize().Mul(400.0))
		b.TargetPos = fleeTarget
		return
	}

	// 2. Check for nearby smaller players to HUNT
	var closestPrey *Player
	minPreyDist := 320.0

	for i := range w.players {
		other := &w.players[i]
		if other.ID == b.ID || !other.Alive {
			continue
		}
		if b.Mass > other.Mass*1.12 {
			dist := b.Position.Sub(other.Position).Length()
			if dist < minPreyDist {
				minPreyDist = dist
				closestPrey = other
			}
		}
	}

	if closestPrey != nil {
		b.TargetPos = closestPrey.Position
		return
	}

	// 3. Otherwise: Seek closest food pellet
	var closestFood Vec2
	minFoodDist := 1000000.0
	bx := b.Position.X
	by := b.Position.Y

	// Sample 25 food pellets for ultra-fast zero-alloc lookup
	sampleCount := 25
	offset := rand.Intn(len(w.food))
	for i := 0; i < sampleCount; i++ {
		idx := (offset + i) % len(w.food)
		fx := float64(w.food[idx].X)
		fy := float64(w.food[idx].Y)
		d2 := (bx-fx)*(bx-fx) + (by-fy)*(by-fy)
		if d2 < minFoodDist {
			minFoodDist = d2
			closestFood = Vec2{X: fx, Y: fy}
		}
	}

	if minFoodDist < 900000.0 {
		b.TargetPos = closestFood
	} else {
		b.TargetPos = Vec2{
			X: 200.0 + rand.Float64()*(WorldWidth-400.0),
			Y: 200.0 + rand.Float64()*(WorldHeight-400.0),
		}
	}
}

func (w *World) Update(dt float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.tickCount++

	// 1. Process client inputs to update TargetPos
	for i := range w.players {
		p := &w.players[i]
		if !p.Alive {
			continue
		}

		if p.IsBot {
			w.updateBotAI(p)
			continue
		}

		if rb, ok := w.inputBuffers[p.ID]; ok {
			for {
				in, hasInput := rb.PollNext()
				if !hasInput {
					break
				}
				p.TargetPos = Vec2{X: float64(in.TargetX), Y: float64(in.TargetY)}
			}
		}
	}

	// 2. Continuous smooth physical glide towards TargetPos (Eliminating all tirones/stutters)
	for i := range w.players {
		p := &w.players[i]
		if !p.Alive {
			continue
		}

		dir := p.TargetPos.Sub(p.Position)
		dist := dir.Length()

		if dist > 3.0 {
			norm := dir.Normalize()
			moveDist := math.Min(dist, p.Speed*dt)
			p.Velocity = norm.Mul(p.Speed)
			p.Position = p.Position.Add(norm.Mul(moveDist))
			w.clampToBounds(p)
		} else {
			p.Velocity = Vec2{}
		}
	}

	// 3. Clear and rebuild spatial grid
	w.grid.Clear()
	for i := range w.players {
		p := &w.players[i]
		if p.Alive {
			w.grid.Insert(p.ID, p.Position)
		}
	}

	// 4. Authoritative Food Pellet Eating
	for i := range w.players {
		p := &w.players[i]
		if !p.Alive {
			continue
		}

		rSq := p.Radius * p.Radius
		px := p.Position.X
		py := p.Position.Y

		for fIdx := range w.food {
			fx := float64(w.food[fIdx].X)
			fy := float64(w.food[fIdx].Y)
			dx := px - fx
			dy := py - fy
			if dx*dx+dy*dy < rSq {
				// Grow mass & radius
				p.Mass += 0.8
				p.Radius = math.Min(120.0, 15.0*math.Sqrt(p.Mass/10.0))
				p.Speed = math.Max(60.0, 160.0/math.Pow(p.Mass/10.0, 0.16))

				// Respawn food pellet elsewhere
				newX := uint16(60 + rand.Intn(int(WorldWidth-120)))
				newY := uint16(60 + rand.Intn(int(WorldHeight-120)))
				w.food[fIdx].X = newX
				w.food[fIdx].Y = newY

				w.recentEaten = append(w.recentEaten, netproto.FoodEaten{
					Index: uint16(fIdx),
					NewX:  newX,
					NewY:  newY,
				})
			}
		}
	}

	// 5. Authoritative Player-on-Player Combat (Absorption / Devouring)
	neighborBuf := make([]uint32, 0, 64)
	for i := range w.players {
		p := &w.players[i]
		if !p.Alive {
			continue
		}

		neighbors := w.grid.QueryNeighbors(p.Position, neighborBuf)
		for _, nID := range neighbors {
			if nID == p.ID {
				continue
			}
			nIdx, ok := w.playerIndex[nID]
			if !ok {
				continue
			}
			other := &w.players[nIdx]
			if !other.Alive {
				continue
			}

			// Must have at least 12% mass advantage to absorb
			if p.Mass > other.Mass*1.12 {
				dist := p.Position.Sub(other.Position).Length()
				if dist < p.Radius {
					// p devours other!
					massGain := other.Mass * 0.55
					p.Mass += massGain
					p.Radius = math.Min(150.0, 15.0*math.Sqrt(p.Mass/10.0))
					p.Speed = math.Max(60.0, 160.0/math.Pow(p.Mass/10.0, 0.16))

					w.recentKills = append(w.recentKills, netproto.KillEvent{
						WinnerID: p.ID,
						VictimID: other.ID,
						Points:   uint16(massGain * 10),
					})

					// Respawn victim
					other.Mass = 10.0
					other.Radius = 15.0
					other.Speed = 160.0
					other.Position = Vec2{
						X: 200.0 + rand.Float64()*(WorldWidth-400.0),
						Y: 200.0 + rand.Float64()*(WorldHeight-400.0),
					}
					other.TargetPos = other.Position
					other.Velocity = Vec2{}
				}
			}
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
