package interest

import "github.com/thaonguyen/realtime-room-middleware/internal/game/spatial"

// InterestConfig holds radii for interest management queries.
type InterestConfig struct {
	VisualRadiusM     float32
	ObjectRadiusM     float32
	VoiceRadiusM      float32
	FullAvatarRadiusM float32
	LowLodRadiusM     float32
}

// DefaultInterestConfig returns production-default interest settings (all 30m).
func DefaultInterestConfig() InterestConfig {
	return InterestConfig{
		VisualRadiusM:     30,
		ObjectRadiusM:     30,
		VoiceRadiusM:      30,
		FullAvatarRadiusM: 30,
		LowLodRadiusM:     30,
	}
}

// InterestSet represents the set of entities visible to a viewer.
type InterestSet struct {
	VisiblePlayers []spatial.EntityID
	// Future milestones:
	// VisibleObjects  []spatial.EntityID
	// VoiceCandidates []spatial.EntityID
}

// InterestManager computes per-client interest sets using spatial proximity.
type InterestManager struct {
	config InterestConfig
}

// NewInterestManager creates a manager with the given config.
func NewInterestManager(config InterestConfig) *InterestManager {
	return &InterestManager{config: config}
}

// QueryVisiblePlayers returns players within the visual radius of the viewer,
// excluding the viewer's own entity.
func (m *InterestManager) QueryVisiblePlayers(
	idx *spatial.GridSpatialHash,
	viewerPos spatial.EntityPosition,
	viewerID spatial.EntityID,
) InterestSet {
	ids := idx.QueryRadius(viewerPos, m.config.VisualRadiusM)
	filtered := make([]spatial.EntityID, 0, len(ids))
	for _, id := range ids {
		if id != viewerID {
			filtered = append(filtered, id)
		}
	}
	return InterestSet{VisiblePlayers: filtered}
}
