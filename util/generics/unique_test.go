package ge

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestUnique(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		res := Unique[int](nil)
		assert.Nil(t, res)
	})

	t.Run("empty", func(t *testing.T) {
		res := Unique[int]([]int{})
		assert.Empty(t, res)
	})

	t.Run("single item", func(t *testing.T) {
		slice := []int{}
		res := Unique[int](slice)
		assert.Exactly(t, slice, res)
	})

	t.Run("already unique", func(t *testing.T) {
		res := Unique[int]([]int{1, 4, 5, 2, 3, 10})
		assert.Exactly(t, []int{1, 4, 5, 2, 3, 10}, res)
	})

	t.Run("non-unique unsorted", func(t *testing.T) {
		res := Unique[int]([]int{1, 4, 1, 5, 4, 2, 5, 3, 10})
		assert.Exactly(t, []int{1, 4, 5, 2, 3, 10}, res)
	})

	t.Run("non-unique sorted", func(t *testing.T) {
		res := Unique[int]([]int{1, 1, 2, 3, 4, 4, 5, 5, 10})
		assert.Exactly(t, []int{1, 2, 3, 4, 5, 10}, res)
	})
}
