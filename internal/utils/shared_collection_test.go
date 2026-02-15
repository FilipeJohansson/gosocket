package utils

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSharedCollection_Uint64(t *testing.T) {
	c := NewSharedCollection[string, uint64]()

	id1 := c.Add("first")
	id2 := c.Add("second")
	assert.Equal(t, uint64(1), id1)
	assert.Equal(t, uint64(2), id2)
	assert.True(t, c.Has(id1))
	assert.Equal(t, 2, c.Len())

	got, ok := c.Get(id1)
	assert.True(t, ok)
	assert.Equal(t, "first", got)

	all := c.GetAll()
	all[id1] = "changed"
	got, _ = c.Get(id1)
	assert.Equal(t, "first", got)

	ids := c.GetIds()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	assert.Equal(t, []uint64{1, 2}, ids)

	seen := map[uint64]string{}
	c.ForEach(func(id uint64, obj string) {
		seen[id] = obj
	})
	assert.Equal(t, 2, len(seen))

	assert.True(t, c.Remove(id1))
	assert.False(t, c.Remove(id1))
}

func TestSharedCollection_String(t *testing.T) {
	c := NewSharedCollection[int, string](4)

	id1 := c.Add(10)
	id2 := c.Add(20)
	overrideID := c.Add(30, "custom")

	assert.Equal(t, "000001", id1)
	assert.Equal(t, "000002", id2)
	assert.Equal(t, "custom", overrideID)

	v, ok := c.Get("custom")
	assert.True(t, ok)
	assert.Equal(t, 30, v)
}
