package game

type Player struct {
	ID        uint32
	Position  Vec2
	Velocity  Vec2
	TargetDir Vec2
	Mass      float64
	Radius    float64
	Speed     float64
	Alive     bool
}

func NewPlayer(id uint32, startPos Vec2) Player {
	return Player{
		ID:        id,
		Position:  startPos,
		Mass:      10.0,
		Radius:    15.0,
		Speed:     150.0, // World units per second
		Alive:     true,
	}
}
