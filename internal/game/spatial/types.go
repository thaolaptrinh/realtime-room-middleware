package spatial

// EntityID identifies an entity in the spatial index.
// Compatible with string-based IDs (player.PlayerID, etc.) via conversion.
type EntityID string

// EntityKind distinguishes types of spatial entities.
type EntityKind uint8

const (
	EntityPlayer EntityKind = iota + 1
	EntityObject // future: room objects
)

// EntityPosition is a 2D position on the XZ ground plane.
// Y (vertical) is ignored for spatial indexing — proximity is horizontal.
type EntityPosition struct {
	X float32
	Z float32
}

// Pos returns an EntityPosition from X and Z coordinates.
func Pos(x, z float32) EntityPosition {
	return EntityPosition{X: x, Z: z}
}

// CellCoord identifies a grid cell in the spatial hash.
type CellCoord struct {
	X int
	Z int
}

// SpatialConfig holds configuration for the spatial hash grid.
type SpatialConfig struct {
	CellSizeM     float32
	MaxWorldCoord float32 // Max absolute coordinate value. Default 100000 (100km).
}

// DefaultSpatialConfig returns a SpatialConfig with production defaults.
func DefaultSpatialConfig() SpatialConfig {
	return SpatialConfig{
		CellSizeM:     10.0,
		MaxWorldCoord: 100000.0,
	}
}
