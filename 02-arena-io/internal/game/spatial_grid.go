package game

const (
	CellSize           = 150.0
	GridCols           = int(WorldWidth / CellSize)  // 20
	GridRows           = int(WorldHeight / CellSize) // 20
	TotalCells         = GridCols * GridRows         // 400
	MaxEntitiesPerCell = 64
)

type Cell struct {
	EntityIDs []uint32
}

type SpatialGrid struct {
	cells [TotalCells]Cell
}

func NewSpatialGrid() *SpatialGrid {
	g := &SpatialGrid{}
	for i := 0; i < TotalCells; i++ {
		g.cells[i].EntityIDs = make([]uint32, 0, MaxEntitiesPerCell)
	}
	return g
}

func (g *SpatialGrid) Clear() {
	for i := 0; i < TotalCells; i++ {
		g.cells[i].EntityIDs = g.cells[i].EntityIDs[:0]
	}
}

func (g *SpatialGrid) getCellIndex(pos Vec2) int {
	col := int(pos.X / CellSize)
	row := int(pos.Y / CellSize)

	if col < 0 {
		col = 0
	} else if col >= GridCols {
		col = GridCols - 1
	}

	if row < 0 {
		row = 0
	} else if row >= GridRows {
		row = GridRows - 1
	}

	return row*GridCols + col
}

func (g *SpatialGrid) Insert(id uint32, pos Vec2) {
	idx := g.getCellIndex(pos)
	g.cells[idx].EntityIDs = append(g.cells[idx].EntityIDs, id)
}

func (g *SpatialGrid) QueryNeighbors(pos Vec2, result []uint32) []uint32 {
	centerCol := int(pos.X / CellSize)
	centerRow := int(pos.Y / CellSize)

	result = result[:0]

	for r := centerRow - 1; r <= centerRow+1; r++ {
		if r < 0 || r >= GridRows {
			continue
		}
		for c := centerCol - 1; c <= centerCol+1; c++ {
			if c < 0 || c >= GridCols {
				continue
			}
			cellIdx := r*GridCols + c
			result = append(result, g.cells[cellIdx].EntityIDs...)
		}
	}
	return result
}
