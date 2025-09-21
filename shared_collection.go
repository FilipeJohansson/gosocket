package gosocket

import (
	"hash/fnv"
	"sync"
)

// A generic, thread-safe map of objectis with auto-incrementing IDs
type SharedCollection[T any] struct {
	objectMap   map[uint64]T
	stringIdMap map[string]uint64
	nextId      uint64
	sync.Mutex
}

func NewSharedCollection[T any](capacity ...int) *SharedCollection[T] {
	var newObjMap map[uint64]T
	var newStringIdMap map[string]uint64

	if len(capacity) > 0 {
		newObjMap = make(map[uint64]T, capacity[0])
		newStringIdMap = make(map[string]uint64, capacity[0])
	} else {
		newObjMap = make(map[uint64]T)
		newStringIdMap = make(map[string]uint64)
	}

	return &SharedCollection[T]{
		objectMap:   newObjMap,
		stringIdMap: newStringIdMap,
		nextId:      1,
	}
}

// Add an object to the map with the given ID (if provided) or the next available ID
// Returns the ID of the object added
func (s *SharedCollection[T]) Add(obj T, id ...uint64) uint64 {
	s.Lock()
	defer s.Unlock()

	thisId := s.nextId
	if len(id) > 0 {
		thisId = id[0]
	}

	s.objectMap[thisId] = obj
	s.nextId++

	return thisId
}

func (s *SharedCollection[T]) AddWithStringId(obj T, stringId string) uint64 {
	s.Lock()
	defer s.Unlock()

	if existingId, exists := s.stringIdMap[stringId]; exists {
		s.objectMap[existingId] = obj
		return existingId
	}

	internalId := s.hashStringToUint64(stringId)

	for {
		if _, exists := s.objectMap[internalId]; !exists {
			break
		}
		internalId++
	}

	s.objectMap[internalId] = obj
	s.stringIdMap[stringId] = internalId

	return internalId
}

// Removes an object from the map by ID, if it exists
// Returns true if the object was removed
func (s *SharedCollection[T]) Remove(id uint64) bool {
	s.Lock()
	defer s.Unlock()

	delete(s.objectMap, id)
	for stringId, mappedId := range s.stringIdMap {
		if mappedId == id {
			delete(s.stringIdMap, stringId)
			return true
		}
	}

	return false
}

// Removes an object from the map by string ID, if it exists
// Returns true if the object was removed
func (s *SharedCollection[T]) RemoveByStringId(stringId string) bool {
	s.Lock()
	defer s.Unlock()

	if internalId, exists := s.stringIdMap[stringId]; exists {
		delete(s.objectMap, internalId)
		delete(s.stringIdMap, stringId)
		return true
	}

	return false
}

// Call the callback function for each object in the map
func (s *SharedCollection[T]) ForEach(callback func(id uint64, obj T)) {
	// Create a local copy while holding the lock
	s.Lock()
	localCopy := make(map[uint64]T, len(s.objectMap))
	for id, obj := range s.objectMap {
		localCopy[id] = obj
	}
	s.Unlock()

	// Iterate over the local copy without holding the lock
	for id, obj := range localCopy {
		callback(id, obj)
	}
}

func (s *SharedCollection[T]) ForEachWithStringId(callback func(id uint64, stringId string, obj T)) {
	s.Lock()
	localCopy := make(map[uint64]T, len(s.objectMap))
	localStringMap := make(map[uint64]string, len(s.stringIdMap))

	for id, obj := range s.objectMap {
		localCopy[id] = obj
	}

	for stringId, internalId := range s.stringIdMap {
		localStringMap[internalId] = stringId
	}
	s.Unlock()

	for id, obj := range localCopy {
		stringId := localStringMap[id]
		callback(id, stringId, obj)
	}
}

// Get and object with the given ID, if it exists, otherwise nil
// Also returns a boolean indication wheter the object was found
func (s *SharedCollection[T]) Get(id uint64) (T, bool) {
	s.Lock()
	defer s.Unlock()

	obj, found := s.objectMap[id]
	return obj, found
}

func (s *SharedCollection[T]) GetAll() []T {
	s.Lock()
	defer s.Unlock()

	objs := make([]T, 0, len(s.objectMap))
	for _, obj := range s.objectMap {
		objs = append(objs, obj)
	}
	return objs
}

// Get and object with the given string ID, if it exists, otherwise nil
// Also returns a boolean indication wheter the object was found
func (s *SharedCollection[T]) GetByStringId(stringId string) (T, bool) {
	s.Lock()
	defer s.Unlock()

	if internalId, exists := s.stringIdMap[stringId]; exists {
		obj, found := s.objectMap[internalId]
		return obj, found
	}

	var zero T
	return zero, false
}

func (s *SharedCollection[T]) HasStringId(stringId string) bool {
	s.Lock()
	defer s.Unlock()

	_, exists := s.stringIdMap[stringId]
	return exists
}

func (s *SharedCollection[T]) GetStringId(id uint64) (string, bool) {
	s.Lock()
	defer s.Unlock()

	for stringId, internalId := range s.stringIdMap {
		if internalId == id {
			return stringId, true
		}
	}
	return "", false
}

// Get the approximate number of objects in the map
// The reason this is approximate is because the map is read without holding the lock
func (s *SharedCollection[T]) Len() int {
	s.Lock()
	defer s.Unlock()
	return len(s.objectMap)
}

func (s *SharedCollection[T]) hashStringToUint64(str string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(str))
	hash := h.Sum64()

	if hash == 0 {
		hash = 1
	}

	return hash
}
