package game

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"

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

	food           []FoodPellet
	recentEaten    []netproto.FoodEaten
	recentKills    []netproto.KillEvent
	botIDs         []uint32
	nextSubCellNum uint32
}

func NewWorld() *World {
	w := &World{
		players:        make([]Player, 0, MaxPlayersLimit),
		playerIndex:    make(map[uint32]int, MaxPlayersLimit),
		inputBuffers:   make(map[uint32]*InputRingBuffer, MaxPlayersLimit),
		grid:           NewSpatialGrid(),
		food:           make([]FoodPellet, TotalFoodCount),
		recentEaten:    make([]netproto.FoodEaten, 0, 32),
		recentKills:    make([]netproto.KillEvent, 0, 8),
		botIDs:         make([]uint32, 0, BotCountDefault),
		nextSubCellNum: 600000,
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

	// Remove all subcells belonging to this player
	for i := len(w.players) - 1; i >= 0; i-- {
		if w.players[i].OwnerID == id {
			cellID := w.players[i].ID
			lastIdx := len(w.players) - 1
			if i != lastIdx {
				lastPlayer := w.players[lastIdx]
				w.players[i] = lastPlayer
				w.playerIndex[lastPlayer.ID] = i
			}
			w.players = w.players[:lastIdx]
			delete(w.playerIndex, cellID)
		}
	}
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

// ProcessAction handles Spacebar (Split) and W (Eject mass)
func (w *World) ProcessAction(ownerID uint32, actionType uint8, targetX, targetY float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if actionType == netproto.ActionSplit {
		// Count current subcells for this owner
		subCellCount := 0
		for i := range w.players {
			if w.players[i].OwnerID == ownerID && w.players[i].Alive {
				subCellCount++
			}
		}

		if subCellCount >= 8 {
			return
		}

		// Split all qualifying cells (mass >= 20.0)
		initialLen := len(w.players)
		for i := 0; i < initialLen; i++ {
			c := &w.players[i]
			if c.OwnerID != ownerID || !c.Alive || c.Mass < 20.0 {
				continue
			}

			halfMass := c.Mass / 2.0
			c.Mass = halfMass
			c.Radius = math.Max(15.0, 15.0*math.Sqrt(c.Mass/10.0))
			c.Speed = math.Max(60.0, 160.0/math.Pow(c.Mass/10.0, 0.16))
			c.RecombineTimer = 15.0 // 15 seconds until merge!

			// Direction towards target
			dir := Vec2{X: targetX, Y: targetY}.Sub(c.Position)
			if dir.LengthSq() < 0.01 {
				dir = Vec2{X: 1, Y: 0}
			}
			norm := dir.Normalize()

			subID := atomic.AddUint32(&w.nextSubCellNum, 1)
			subPos := c.Position.Add(norm.Mul(c.Radius + 10.0))
			subCell := NewPlayer(subID, subPos)
			subCell.OwnerID = ownerID
			subCell.Mass = halfMass
			subCell.Radius = c.Radius
			subCell.Speed = c.Speed
			subCell.TargetPos = Vec2{X: targetX, Y: targetY}
			subCell.BoostVel = norm.Mul(480.0) // Instantaneous vector impulse!
			subCell.RecombineTimer = 15.0
			subCell.IsBot = c.IsBot

			idx := len(w.players)
			w.players = append(w.players, subCell)
			w.playerIndex[subID] = idx
		}
	} else if actionType == netproto.ActionEject {
		// Eject small mass pellet (1.5 kg)
		for i := range w.players {
			c := &w.players[i]
			if c.OwnerID != ownerID || !c.Alive || c.Mass < 13.0 {
				continue
			}

			c.Mass -= 1.5
			c.Radius = math.Max(15.0, 15.0*math.Sqrt(c.Mass/10.0))
			c.Speed = math.Max(60.0, 160.0/math.Pow(c.Mass/10.0, 0.16))

			dir := Vec2{X: targetX, Y: targetY}.Sub(c.Position)
			if dir.LengthSq() < 0.01 {
				dir = Vec2{X: 1, Y: 0}
			}
			norm := dir.Normalize()

			spawnX := uint16(math.Max(20, math.Min(WorldWidth-20, c.Position.X+norm.X*(c.Radius+24))))
			spawnY := uint16(math.Max(20, math.Min(WorldHeight-20, c.Position.Y+norm.Y*(c.Radius+24))))

			// Reuse random pellet slot to spawn the ejected mass
			pelletIdx := rand.Intn(len(w.food))
			w.food[pelletIdx].X = spawnX
			w.food[pelletIdx].Y = spawnY
			w.food[pelletIdx].ColorIdx = uint8(rand.Intn(5))

			w.recentEaten = append(w.recentEaten, netproto.FoodEaten{
				Index: uint16(pelletIdx),
				NewX:  spawnX,
				NewY:  spawnY,
			})
		}
	}
}

// ToggleBots spawns or stops autonomous server-side bots
func (w *World) ToggleBots() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.botIDs) > 0 {
		for _, bID := range w.botIDs {
			// Remove bot and any of its subcells
			for i := len(w.players) - 1; i >= 0; i-- {
				if w.players[i].OwnerID == bID {
					cellID := w.players[i].ID
					lastIdx := len(w.players) - 1
					if i != lastIdx {
						lastPlayer := w.players[lastIdx]
						w.players[i] = lastPlayer
						w.playerIndex[lastPlayer.ID] = i
					}
					w.players = w.players[:lastIdx]
					delete(w.playerIndex, cellID)
				}
			}
			delete(w.inputBuffers, bID)
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
	b.BotTimer = 10 + rand.Intn(8)

	// 1. Check for nearby larger players to FLEE
	var fleeDir Vec2
	shouldFlee := false

	for i := range w.players {
		other := &w.players[i]
		if other.OwnerID == b.OwnerID || !other.Alive {
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
		b.TargetPos = b.Position.Add(fleeDir.Normalize().Mul(400.0))
		return
	}

	// 2. Check for nearby smaller players to HUNT (or split attack)
	var closestPrey *Player
	minPreyDist := 320.0

	for i := range w.players {
		other := &w.players[i]
		if other.OwnerID == b.OwnerID || !other.Alive {
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
		// Bot split attack if massive enough and in range
		if b.Mass >= 40.0 && minPreyDist < 180.0 && rand.Float64() < 0.15 {
			w.ProcessAction(b.OwnerID, netproto.ActionSplit, closestPrey.Position.X, closestPrey.Position.Y)
		}
		return
	}

	// 3. Otherwise: Seek closest food pellet
	var closestFood Vec2
	minFoodDist := 1000000.0
	bx := b.Position.X
	by := b.Position.Y

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

	// 1. Process client inputs and update TargetPos for all cells belonging to owner
	for i := range w.players {
		p := &w.players[i]
		if !p.Alive {
			continue
		}

		if p.IsBot {
			w.updateBotAI(p)
			continue
		}

		if rb, ok := w.inputBuffers[p.OwnerID]; ok {
			for {
				in, hasInput := rb.PollNext()
				if !hasInput {
					break
				}
				target := Vec2{X: float64(in.TargetX), Y: float64(in.TargetY)}
				// Update all cells belonging to this owner
				for j := range w.players {
					if w.players[j].OwnerID == p.OwnerID {
						w.players[j].TargetPos = target
					}
				}
			}
		}
	}

	// 2. Physical advancement: continuous glide + decaying vector impulse (BoostVel)
	for i := range w.players {
		p := &w.players[i]
		if !p.Alive {
			continue
		}

		// Decay recombine timer
		if p.RecombineTimer > 0 {
			p.RecombineTimer -= dt
			if p.RecombineTimer < 0 {
				p.RecombineTimer = 0
			}
		}

		// Apply impulse velocity if active
		if p.BoostVel.LengthSq() > 1.0 {
			p.Position = p.Position.Add(p.BoostVel.Mul(dt))
			p.BoostVel = p.BoostVel.Mul(0.91) // Decaying exponential friction
		}

		// Normal steering towards TargetPos
		dir := p.TargetPos.Sub(p.Position)
		dist := dir.Length()

		if dist > 3.0 {
			norm := dir.Normalize()
			moveDist := math.Min(dist, p.Speed*dt)
			p.Velocity = norm.Mul(p.Speed)
			p.Position = p.Position.Add(norm.Mul(moveDist))
		} else {
			p.Velocity = Vec2{}
		}

		w.clampToBounds(p)
	}

	// 3. Clear and rebuild spatial grid
	w.grid.Clear()
	for i := range w.players {
		p := &w.players[i]
		if p.Alive {
			w.grid.Insert(p.ID, p.Position)
		}
	}

	// 4. Recombination of cells with same OwnerID
	for i := 0; i < len(w.players); i++ {
		p1 := &w.players[i]
		if !p1.Alive || p1.RecombineTimer > 0 {
			continue
		}

		for j := i + 1; j < len(w.players); j++ {
			p2 := &w.players[j]
			if !p2.Alive || p2.OwnerID != p1.OwnerID || p2.RecombineTimer > 0 {
				continue
			}

			dist := p1.Position.Sub(p2.Position).Length()
			if dist < p1.Radius+p2.Radius {
				// Recombine into p1!
				p1.Mass += p2.Mass
				p1.Radius = math.Min(160.0, 15.0*math.Sqrt(p1.Mass/10.0))
				p1.Speed = math.Max(60.0, 160.0/math.Pow(p1.Mass/10.0, 0.16))

				// Remove p2
				p2ID := p2.ID
				lastIdx := len(w.players) - 1
				if j != lastIdx {
					lastPlayer := w.players[lastIdx]
					w.players[j] = lastPlayer
					w.playerIndex[lastPlayer.ID] = j
				}
				w.players = w.players[:lastIdx]
				delete(w.playerIndex, p2ID)
				j--
			}
		}
	}

	// 5. Authoritative Food Pellet Eating
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
				p.Mass += 0.8
				p.Radius = math.Min(120.0, 15.0*math.Sqrt(p.Mass/10.0))
				p.Speed = math.Max(60.0, 160.0/math.Pow(p.Mass/10.0, 0.16))

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

	// 6. Authoritative Player-on-Player Combat (Absorption between different Owners)
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
			if !other.Alive || other.OwnerID == p.OwnerID {
				continue // Don't eat own subcells!
			}

			// Require 12% mass advantage to absorb
			if p.Mass > other.Mass*1.12 {
				dist := p.Position.Sub(other.Position).Length()
				if dist < p.Radius {
					massGain := other.Mass * 0.55
					p.Mass += massGain
					p.Radius = math.Min(160.0, 15.0*math.Sqrt(p.Mass/10.0))
					p.Speed = math.Max(60.0, 160.0/math.Pow(p.Mass/10.0, 0.16))

					w.recentKills = append(w.recentKills, netproto.KillEvent{
						WinnerID: p.OwnerID,
						VictimID: other.OwnerID,
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
					other.BoostVel = Vec2{}
					other.RecombineTimer = 0
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
				ID:      np.ID,
				OwnerID: np.OwnerID,
				X:       float32(np.Position.X),
				Y:       float32(np.Position.Y),
				Radius:  float32(np.Radius),
				Mass:    uint16(np.Mass * 10),
			})
		}
	}
	return out
}
