package spatial

import (
	"fmt"
	"math"
)

type cellEntry struct {
	coord CellCoord
	pos   EntityPosition
}

// GridSpatialHash is a grid-based spatial index for proximity queries.
//
// It is NOT goroutine-safe. The caller must synchronize access.
// In the room architecture, the room loop is the only mutator and reader;
// external callers hold the room's sessionMu.RLock for reads.
type GridSpatialHash struct {
	cellSize      float32
	maxWorldCoord float32
	cells         map[CellCoord]map[EntityID]EntityPosition
	entities      map[EntityID]cellEntry
}

// NewGridSpatialHash creates a spatial hash with the given configuration.
func NewGridSpatialHash(config SpatialConfig) *GridSpatialHash {
	maxCoord := config.MaxWorldCoord
	if maxCoord <= 0 {
		maxCoord = DefaultSpatialConfig().MaxWorldCoord
	}
	return &GridSpatialHash{
		cellSize:      config.CellSizeM,
		maxWorldCoord: maxCoord,
		cells:         make(map[CellCoord]map[EntityID]EntityPosition),
		entities:      make(map[EntityID]cellEntry),
	}
}

// validatePosition checks that a position is finite and within world bounds.
func (h *GridSpatialHash) validatePosition(pos EntityPosition) error {
	if math.IsNaN(float64(pos.X)) || math.IsInf(float64(pos.X), 0) {
		return fmt.Errorf("X coordinate is NaN or Inf")
	}
	if math.IsNaN(float64(pos.Z)) || math.IsInf(float64(pos.Z), 0) {
		return fmt.Errorf("Z coordinate is NaN or Inf")
	}
	if math.Abs(float64(pos.X)) > float64(h.maxWorldCoord) {
		return fmt.Errorf("X coordinate %.0f exceeds max world bound %.0f", pos.X, h.maxWorldCoord)
	}
	if math.Abs(float64(pos.Z)) > float64(h.maxWorldCoord) {
		return fmt.Errorf("Z coordinate %.0f exceeds max world bound %.0f", pos.Z, h.maxWorldCoord)
	}
	return nil
}

// Update inserts or moves an entity to the given position.
// Returns an error if the position is NaN, Inf, or exceeds MaxWorldCoord.
func (h *GridSpatialHash) Update(id EntityID, pos EntityPosition) error {
	if err := h.validatePosition(pos); err != nil {
		return fmt.Errorf("spatial update %q: %w", id, err)
	}

	newCoord := toCellCoord(pos, h.cellSize)

	if entry, ok := h.entities[id]; ok {
		if entry.coord == newCoord {
			h.cells[newCoord][id] = pos
			h.entities[id] = cellEntry{coord: newCoord, pos: pos}
			return nil
		}
		h.removeFromCell(id, entry.coord)
	}

	cell, ok := h.cells[newCoord]
	if !ok {
		cell = make(map[EntityID]EntityPosition)
		h.cells[newCoord] = cell
	}
	cell[id] = pos
	h.entities[id] = cellEntry{coord: newCoord, pos: pos}
	return nil
}

// Remove removes an entity from the spatial index. No-op if not present.
func (h *GridSpatialHash) Remove(id EntityID) {
	entry, ok := h.entities[id]
	if !ok {
		return
	}
	h.removeFromCell(id, entry.coord)
	delete(h.entities, id)
}

// Get returns the current position of an entity.
func (h *GridSpatialHash) Get(id EntityID) (EntityPosition, bool) {
	entry, ok := h.entities[id]
	if !ok {
		return EntityPosition{}, false
	}
	return entry.pos, true
}

// QueryRadius returns all entity IDs within the given radius of the position.
// Returns nil for non-positive radius or invalid query positions.
// The returned slice has no guaranteed order.
func (h *GridSpatialHash) QueryRadius(pos EntityPosition, radius float32) []EntityID {
	if radius <= 0 {
		return nil
	}
	if err := h.validatePosition(pos); err != nil {
		return nil
	}

	viewerCoord := toCellCoord(pos, h.cellSize)
	cellRadius := int(math.Ceil(float64(radius / h.cellSize)))
	radiusSq := float64(radius) * float64(radius)

	var result []EntityID

	for cx := viewerCoord.X - cellRadius; cx <= viewerCoord.X + cellRadius; cx++ {
		for cz := viewerCoord.Z - cellRadius; cz <= viewerCoord.Z + cellRadius; cz++ {
			cell, ok := h.cells[CellCoord{X: cx, Z: cz}]
			if !ok {
				continue
			}
			for id, entityPos := range cell {
				dx := float64(pos.X - entityPos.X)
				dz := float64(pos.Z - entityPos.Z)
				if dx*dx+dz*dz <= radiusSq {
					result = append(result, id)
				}
			}
		}
	}

	return result
}

// Len returns the number of tracked entities.
func (h *GridSpatialHash) Len() int {
	return len(h.entities)
}

// Clear removes all entities from the index.
func (h *GridSpatialHash) Clear() {
	for coord := range h.cells {
		delete(h.cells, coord)
	}
	for id := range h.entities {
		delete(h.entities, id)
	}
}

// CellSize returns the configured cell size.
func (h *GridSpatialHash) CellSize() float32 {
	return h.cellSize
}

func (h *GridSpatialHash) removeFromCell(id EntityID, coord CellCoord) {
	cell, ok := h.cells[coord]
	if !ok {
		return
	}
	delete(cell, id)
	if len(cell) == 0 {
		delete(h.cells, coord)
	}
}

func toCellCoord(pos EntityPosition, cellSize float32) CellCoord {
	return CellCoord{
		X: int(math.Floor(float64(pos.X / cellSize))),
		Z: int(math.Floor(float64(pos.Z / cellSize))),
	}
}
