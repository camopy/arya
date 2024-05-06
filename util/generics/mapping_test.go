package ge

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Nil(t, Map([]string{}, func(s string) int { return len(s) }))
	})
	t.Run("values", func(t *testing.T) {
		res := Map(
			[]string{"a", "b", "cc", "ddd", "ee", "ffff"},
			func(s string) int { return len(s) },
		)
		assert.Exactly(t, []int{1, 1, 2, 3, 2, 4}, res)
	})
}
