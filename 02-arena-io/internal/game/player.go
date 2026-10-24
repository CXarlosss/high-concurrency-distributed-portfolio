package game

type Player struct {
	ID        uint32
	Position  Vec2
	Velocity  Vec2
	TargetPos Vec2
	Mass      float64
	Radius    float64
	Speed     float64
	Alive     bool
	IsBot     bool
	BotTimer  int
}

func NewPlayer(id uint32, startPos Vec2) Player {
	return Player{
		ID:        id,
		Position:  startPos,
		TargetPos: startPos,
		Mass:      10.0,
		Radius:    15.0,
		Speed:     160.0, // World units per second
		Alive:     true,
		IsBot:     false,
	}
}

func NewBot(id uint32, startPos Vec2) Player {
	p := NewPlayer(id, startPos)
	p.IsBot = true
	return p
}
