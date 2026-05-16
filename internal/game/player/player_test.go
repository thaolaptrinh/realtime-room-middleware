package player

import (
	"math"
	"testing"
	"time"
)

// ---- Validation tests ------------------------------------------------------

func TestValidatePlayerID(t *testing.T) {
	tests := []struct {
		name    string
		id      PlayerID
		wantErr bool
	}{
		{"valid", "player-123", false},
		{"empty", "", true},
		{"too long", PlayerID(string(make([]byte, 257))), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlayerID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlayerID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUserID(t *testing.T) {
	tests := []struct {
		name    string
		id      UserID
		wantErr bool
	}{
		{"valid", "user-abc", false},
		{"empty", "", true},
		{"too long", UserID(string(make([]byte, 257))), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUserID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVector3(t *testing.T) {
	tests := []struct {
		name    string
		v       Vector3
		wantErr bool
	}{
		{"valid", Vector3{X: 1, Y: 2, Z: 3}, false},
		{"zero", Vector3{}, false},
		{"negative", Vector3{X: -1, Y: -2, Z: -3}, false},
		{"NaN X", Vector3{X: float32(math.NaN()), Y: 0, Z: 0}, true},
		{"NaN Y", Vector3{X: 0, Y: float32(math.NaN()), Z: 0}, true},
		{"NaN Z", Vector3{X: 0, Y: 0, Z: float32(math.NaN())}, true},
		{"Inf X", Vector3{X: float32(math.Inf(1)), Y: 0, Z: 0}, true},
		{"Inf Y", Vector3{X: 0, Y: float32(math.Inf(-1)), Z: 0}, true},
		{"Inf Z", Vector3{X: 0, Y: 0, Z: float32(math.Inf(1))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVector3(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVector3() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateQuaternion(t *testing.T) {
	tests := []struct {
		name    string
		q       Quaternion
		wantErr bool
	}{
		{"identity", IdentityQuaternion, false},
		{"valid rotation", Quaternion{X: 0.707, Y: 0, Z: 0, W: 0.707}, false},
		{"zero", Quaternion{}, true},
		{"NaN X", Quaternion{X: float32(math.NaN()), Y: 0, Z: 0, W: 1}, true},
		{"NaN W", Quaternion{X: 0, Y: 0, Z: 0, W: float32(math.NaN())}, true},
		{"Inf Y", Quaternion{X: 0, Y: float32(math.Inf(1)), Z: 0, W: 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuaternion(tt.q)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuaternion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePlayerTransform(t *testing.T) {
	tests := []struct {
		name    string
		t       PlayerTransform
		wantErr bool
	}{
		{
			name: "valid",
			t: PlayerTransform{
				Position:  Vector3{X: 1, Y: 2, Z: 3},
				Rotation:  IdentityQuaternion,
				Tick:      123,
			},
			wantErr: false,
		},
		{
			name: "invalid position",
			t: PlayerTransform{
				Position:  Vector3{X: float32(math.NaN()), Y: 0, Z: 0},
				Rotation:  IdentityQuaternion,
				Tick:      0,
			},
			wantErr: true,
		},
		{
			name: "invalid rotation",
			t: PlayerTransform{
				Position:  Vector3{},
				Rotation:  Quaternion{},
				Tick:      0,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlayerTransform(tt.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlayerTransform() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePlayerInput(t *testing.T) {
	tests := []struct {
		name    string
		input   PlayerInput
		wantErr bool
	}{
		{
			name: "valid",
			input: PlayerInput{
				Seq:       1,
				Transform: PlayerTransform{Position: Vector3{}, Rotation: IdentityQuaternion},
				Timestamp: time.Now().UnixMilli(),
			},
			wantErr: false,
		},
		{
			name: "invalid transform",
			input: PlayerInput{
				Seq:       1,
				Transform: PlayerTransform{Position: Vector3{X: float32(math.Inf(1))}, Rotation: IdentityQuaternion},
				Timestamp: time.Now().UnixMilli(),
			},
			wantErr: true,
		},
		{
			name: "zero timestamp",
			input: PlayerInput{
				Seq:       1,
				Transform: PlayerTransform{Position: Vector3{}, Rotation: IdentityQuaternion},
				Timestamp: 0,
			},
			wantErr: true,
		},
		{
			name: "negative timestamp",
			input: PlayerInput{
				Seq:       1,
				Transform: PlayerTransform{Position: Vector3{}, Rotation: IdentityQuaternion},
				Timestamp: -1,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlayerInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlayerInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ---- PlayerState tests ------------------------------------------------------

func TestNewPlayerState(t *testing.T) {
	pid := PlayerID("p1")
	uid := UserID("u1")
	now := time.Now()

	p := NewPlayerState(pid, uid, now)

	if p.ID != pid {
		t.Errorf("ID = %v, want %v", p.ID, pid)
	}
	if p.UserID != uid {
		t.Errorf("UserID = %v, want %v", p.UserID, uid)
	}
	if p.Status != PlayerStatusJoining {
		t.Errorf("Status = %v, want %v", p.Status, PlayerStatusJoining)
	}
	if p.JoinedAt.IsZero() {
		t.Error("JoinedAt should be set")
	}
	if p.Version != 0 {
		t.Errorf("Version = %d, want 0", p.Version)
	}

	// Check initial transform.
	transform, _ := p.Snapshot()
	if transform.Position != (Vector3{}) {
		t.Errorf("initial Position = %v, want zero", transform.Position)
	}
	if transform.Rotation != IdentityQuaternion {
		t.Errorf("initial Rotation = %v, want identity", transform.Rotation)
	}
	if transform.Tick != 0 {
		t.Errorf("initial Tick = %d, want 0", transform.Tick)
	}
}

func TestPlayerState_UpdateTransform(t *testing.T) {
	p := NewPlayerState("p1", "u1", time.Now())

	// Initial state.
	if p.Version != 0 {
		t.Fatalf("initial Version = %d, want 0", p.Version)
	}

	// Update transform.
	newTransform := PlayerTransform{
		Position: Vector3{X: 10, Y: 20, Z: 30},
		Rotation: Quaternion{X: 0.707, Y: 0, Z: 0, W: 0.707},
		Tick:     0,
	}
	p.UpdateTransform(newTransform, 100)

	// Check version incremented.
	if p.Version != 1 {
		t.Errorf("after update Version = %d, want 1", p.Version)
	}

	// Check snapshot matches.
	transform, version := p.Snapshot()
	if version != 1 {
		t.Errorf("snapshot version = %d, want 1", version)
	}
	if transform.Position != newTransform.Position {
		t.Errorf("snapshot Position = %v, want %v", transform.Position, newTransform.Position)
	}
	if transform.Rotation != newTransform.Rotation {
		t.Errorf("snapshot Rotation = %v, want %v", transform.Rotation, newTransform.Rotation)
	}
	if transform.Tick != 100 {
		t.Errorf("snapshot Tick = %d, want 100", transform.Tick)
	}

	// Second update.
	p.UpdateTransform(newTransform, 101)
	if p.Version != 2 {
		t.Errorf("after second update Version = %d, want 2", p.Version)
	}
	transform, _ = p.Snapshot()
	if transform.Tick != 101 {
		t.Errorf("after second update Tick = %d, want 101", transform.Tick)
	}
}

func TestPlayerState_Status(t *testing.T) {
	p := NewPlayerState("p1", "u1", time.Now())

	if p.GetStatus() != PlayerStatusJoining {
		t.Errorf("initial Status = %v, want Joining", p.GetStatus())
	}

	p.MarkStatus(PlayerStatusActive)
	if p.GetStatus() != PlayerStatusActive {
		t.Errorf("after MarkStatus(StatusActive) = %v, want Active", p.GetStatus())
	}

	p.SetStatus(PlayerStatusLeaving)
	if p.GetStatus() != PlayerStatusLeaving {
		t.Errorf("after SetStatus(StatusLeaving) = %v, want Leaving", p.GetStatus())
	}
}

func TestPlayerState_SnapshotConcurrent(t *testing.T) {
	p := NewPlayerState("p1", "u1", time.Now())

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			p.Snapshot()
		}
		close(done)
	}()

	newTransform := PlayerTransform{
		Position: Vector3{X: 1, Y: 2, Z: 3},
		Rotation: IdentityQuaternion,
	}
	for i := 0; i < 100; i++ {
		p.UpdateTransform(newTransform, uint32(i))
	}

	<-done
	// If we get here without deadlock or data race, test passes.
}
