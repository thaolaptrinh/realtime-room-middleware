package player

import (
	"fmt"
	"math"
)

// ValidatePlayerID checks that a PlayerID is non-empty and within reasonable length.
func ValidatePlayerID(id PlayerID) error {
	if id == "" {
		return fmt.Errorf("player ID cannot be empty")
	}
	if len(id) > 256 {
		return fmt.Errorf("player ID too long: %d > 256", len(id))
	}
	return nil
}

// ValidateUserID checks that a UserID is non-empty and within reasonable length.
func ValidateUserID(id UserID) error {
	if id == "" {
		return fmt.Errorf("user ID cannot be empty")
	}
	if len(id) > 256 {
		return fmt.Errorf("user ID too long: %d > 256", len(id))
	}
	return nil
}

// ValidateVector3 checks that a Vector3 contains finite values (no NaN or Inf).
func ValidateVector3(v Vector3) error {
	if math.IsNaN(float64(v.X)) || math.IsInf(float64(v.X), 0) {
		return fmt.Errorf("X coordinate is NaN or Inf")
	}
	if math.IsNaN(float64(v.Y)) || math.IsInf(float64(v.Y), 0) {
		return fmt.Errorf("Y coordinate is NaN or Inf")
	}
	if math.IsNaN(float64(v.Z)) || math.IsInf(float64(v.Z), 0) {
		return fmt.Errorf("Z coordinate is NaN or Inf")
	}
	return nil
}

// ValidateQuaternion checks that a Quaternion is a valid unit quaternion
// (all components finite, non-zero magnitude). Does not enforce strict unit length
// because Unity may send slightly denormalized quaternions.
func ValidateQuaternion(q Quaternion) error {
	if math.IsNaN(float64(q.X)) || math.IsInf(float64(q.X), 0) {
		return fmt.Errorf("quaternion X is NaN or Inf")
	}
	if math.IsNaN(float64(q.Y)) || math.IsInf(float64(q.Y), 0) {
		return fmt.Errorf("quaternion Y is NaN or Inf")
	}
	if math.IsNaN(float64(q.Z)) || math.IsInf(float64(q.Z), 0) {
		return fmt.Errorf("quaternion Z is NaN or Inf")
	}
	if math.IsNaN(float64(q.W)) || math.IsInf(float64(q.W), 0) {
		return fmt.Errorf("quaternion W is NaN or Inf")
	}

	// Reject zero quaternion (non-rotatable).
	if q.X == 0 && q.Y == 0 && q.Z == 0 && q.W == 0 {
		return fmt.Errorf("zero quaternion is invalid")
	}

	return nil
}

// ValidatePlayerTransform checks that position and rotation are both valid.
func ValidatePlayerTransform(t PlayerTransform) error {
	if err := ValidateVector3(t.Position); err != nil {
		return fmt.Errorf("position invalid: %w", err)
	}
	if err := ValidateQuaternion(t.Rotation); err != nil {
		return fmt.Errorf("rotation invalid: %w", err)
	}
	return nil
}

// ValidatePlayerInput checks the sequence number, transform validity, and timestamp.
func ValidatePlayerInput(input PlayerInput) error {
	if err := ValidatePlayerTransform(input.Transform); err != nil {
		return err
	}
	if input.Timestamp <= 0 {
		return fmt.Errorf("timestamp must be positive: %d", input.Timestamp)
	}
	return nil
}
