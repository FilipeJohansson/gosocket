package gosocket

import (
	"fmt"
	"sync"
)

type ID interface {
	~uint64 | ~string
}

// A generic, thread-safe map of objectis with auto-incrementing IDs
type SharedCollection[T any, K ID] struct {
	objectMap map[K]T
	nextId    uint64
	sync.Mutex
}

func NewSharedCollection[T any, K ID](capacity ...int) *SharedCollection[T, K] {
	var newObjMap map[K]T

	if len(capacity) > 0 {
		newObjMap = make(map[K]T, capacity[0])
	} else {
		newObjMap = make(map[K]T)
	}

	return &SharedCollection[T, K]{
		objectMap: newObjMap,
		nextId:    1,
	}
}

// Add an object to the map with the given ID (if provided) or the next available ID
// Returns the ID of the object added
func (s *SharedCollection[T, K]) Add(obj T, id ...K) K {
	s.Lock()
	defer s.Unlock()

	var thisId K
	if len(id) > 0 {
		thisId = id[0]
	} else {
		thisId = s.generateId()
	}

	s.objectMap[thisId] = obj
	return thisId
}

// Removes an object from the map by ID, if it exists
// Returns true if the object was removed
func (s *SharedCollection[T, K]) Remove(id K) bool {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.objectMap[id]; exists {
		delete(s.objectMap, id)
		return true
	}
	return false
}

// Call the callback function for each object in the map
func (s *SharedCollection[T, K]) ForEach(callback func(id K, obj T)) {
	// Create a local copy while holding the lock
	s.Lock()
	localCopy := make(map[K]T, len(s.objectMap))
	for id, obj := range s.objectMap {
		localCopy[id] = obj
	}
	s.Unlock()

	// Iterate over the local copy without holding the lock
	for id, obj := range localCopy {
		callback(id, obj)
	}
}

// Get and object with the given ID, if it exists, otherwise nil
// Also returns a boolean indication wheter the object was found
func (s *SharedCollection[T, K]) Get(id K) (T, bool) {
	s.Lock()
	defer s.Unlock()

	obj, found := s.objectMap[id]
	return obj, found
}

func (s *SharedCollection[T, K]) GetAll() map[K]T {
	s.Lock()
	defer s.Unlock()

	// Return a copy to avoid external modifications
	result := make(map[K]T, len(s.objectMap))
	for k, v := range s.objectMap {
		result[k] = v
	}
	return result
}

// GetIds returns all IDs in the collection
func (s *SharedCollection[T, K]) GetIds() []K {
	s.Lock()
	defer s.Unlock()

	ids := make([]K, 0, len(s.objectMap))
	for id := range s.objectMap {
		ids = append(ids, id)
	}
	return ids
}

func (s *SharedCollection[T, K]) Has(id K) bool {
	s.Lock()
	defer s.Unlock()

	_, exists := s.objectMap[id]
	return exists
}

// Get the approximate number of objects in the map
// The reason this is approximate is because the map is read without holding the lock
func (s *SharedCollection[T, K]) Len() int {
	s.Lock()
	defer s.Unlock()
	return len(s.objectMap)
}

func (s *SharedCollection[T, K]) generateId() K {
	var zero K

	switch any(zero).(type) {
	case uint64:
		id := s.nextId
		s.nextId++
		return any(id).(K)
	case string:
		id := fmt.Sprintf("%06d", s.nextId)
		s.nextId++
		return any(id).(K)
	default:
		panic("unsupported ID type")
	}
}
