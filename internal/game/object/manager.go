package object

import "fmt"

// ObjectManager manages room objects within a single room instance.
// Not goroutine-safe — must be accessed only from the room loop goroutine.
type ObjectManager struct {
	objects map[ObjectID]*ObjectState
}

// NewObjectManager creates an empty ObjectManager.
func NewObjectManager() *ObjectManager {
	return &ObjectManager{objects: make(map[ObjectID]*ObjectState)}
}

// Create registers a new active object. Returns an error if the ID already exists.
func (m *ObjectManager) Create(id ObjectID, kind ObjectKind, transform ObjectTransform) (*ObjectState, error) {
	if _, exists := m.objects[id]; exists {
		return nil, fmt.Errorf("object %q already exists", id)
	}
	obj := &ObjectState{
		ID:        id,
		Kind:      kind,
		Transform: transform,
		Status:    ObjectStatusActive,
	}
	m.objects[id] = obj
	return obj, nil
}

// Get returns the object with the given ID. Returns false if not found.
func (m *ObjectManager) Get(id ObjectID) (*ObjectState, bool) {
	obj, ok := m.objects[id]
	return obj, ok
}

// List returns all tracked objects regardless of status.
// The slice contains live pointers — callers must not modify them outside the room loop.
func (m *ObjectManager) List() []*ObjectState {
	result := make([]*ObjectState, 0, len(m.objects))
	for _, obj := range m.objects {
		result = append(result, obj)
	}
	return result
}

// ActiveObjects returns all objects with ObjectStatusActive.
func (m *ObjectManager) ActiveObjects() []*ObjectState {
	var result []*ObjectState
	for _, obj := range m.objects {
		if obj.Status == ObjectStatusActive {
			result = append(result, obj)
		}
	}
	return result
}

// UpdateTransform sets the object's transform and increments its version.
func (m *ObjectManager) UpdateTransform(id ObjectID, transform ObjectTransform) error {
	obj, ok := m.objects[id]
	if !ok {
		return fmt.Errorf("object %q not found", id)
	}
	obj.Transform = transform
	obj.Version++
	return nil
}

// UpdateCustomState replaces the object's custom state bytes and increments version.
func (m *ObjectManager) UpdateCustomState(id ObjectID, state []byte) error {
	obj, ok := m.objects[id]
	if !ok {
		return fmt.Errorf("object %q not found", id)
	}
	obj.CustomState = state
	obj.Version++
	return nil
}

// MarkInactive marks the object as inactive and increments its version.
// Inactive objects are not synchronized to clients in normal operation.
func (m *ObjectManager) MarkInactive(id ObjectID) error {
	obj, ok := m.objects[id]
	if !ok {
		return fmt.Errorf("object %q not found", id)
	}
	if obj.Status == ObjectStatusInactive {
		return nil
	}
	obj.Status = ObjectStatusInactive
	obj.Version++
	return nil
}

// Remove deletes the object entry entirely.
func (m *ObjectManager) Remove(id ObjectID) {
	delete(m.objects, id)
}

// Count returns the total number of tracked objects (active and inactive).
func (m *ObjectManager) Count() int {
	return len(m.objects)
}
